package huma

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logLevel *zap.AtomicLevel

// MaxLogBodyBytes logs at most this many bytes of any request body during a
// panic when using the recovery middleware. Defaults to 10KiB.
var MaxLogBodyBytes int64 = 10 * 1024

// Middleware is a type alias used to group Gin middleware functions.
type Middleware = gin.HandlerFunc

// Handler is a type alias used to group Gin handler functions.
type Handler = gin.HandlerFunc

// bufferedReadCloser will read and buffer up to max bytes into buf. Additional
// reads bypass the buffer.
type bufferedReadCloser struct {
	reader io.ReadCloser
	buf    *bytes.Buffer
	max    int64
}

// newBufferedReadCloser returns a new BufferedReadCloser that wraps reader
// and reads up to max bytes into the buffer.
func newBufferedReadCloser(reader io.ReadCloser, buffer *bytes.Buffer, max int64) *bufferedReadCloser {
	return &bufferedReadCloser{
		reader: reader,
		buf:    buffer,
		max:    max,
	}
}

// Read data into p. Returns number of bytes read and an error, if any.
func (r *bufferedReadCloser) Read(p []byte) (n int, err error) {
	// Read from the underlying reader like normal.
	n, err = r.reader.Read(p)

	// If buffer isn't full, add to it.
	length := int64(r.buf.Len())
	if length < r.max {
		if length+int64(n) < r.max {
			r.buf.Write(p[:n])
		} else {
			r.buf.Write(p[:int64(n)-(r.max-length)])
		}
	}

	return
}

// Close the underlying reader.
func (r *bufferedReadCloser) Close() error {
	return r.reader.Close()
}

// Recovery prints stack traces on panic when used with the logging middleware.
func Recovery() Middleware {
	bufPool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	return func(c *gin.Context) {
		var buf *bytes.Buffer

		// Reset the body so other middleware/handlers can use it.
		if c.Request.Body != nil {
			// Get a buffer that the body will be read into.
			buf = bufPool.Get().(*bytes.Buffer)
			defer bufPool.Put(buf)

			c.Request.Body = newBufferedReadCloser(c.Request.Body, buf, MaxLogBodyBytes)
		}

		// Recovering comes *after* the above so the buffer is not returned to
		// the pool until after we print out its contents.
		defer func() {
			if err := recover(); err != nil {
				// The body might have been read or partially read, so replace it
				// with a clean reader to dump out up to maxBodyBytes with the error.
				if buf != nil && buf.Len() != 0 {
					c.Request.Body = ioutil.NopCloser(buf)
				} else if c.Request.Body != nil {
					defer c.Request.Body.Close()
					c.Request.Body = ioutil.NopCloser(io.LimitReader(c.Request.Body, MaxLogBodyBytes))
				}

				request, _ := httputil.DumpRequest(c.Request, true)

				if l, ok := c.Get("log"); ok {
					if log, ok := l.(*zap.SugaredLogger); ok {
						if e, ok := err.(error); ok {
							log = log.With(zap.Error(e))
						} else {
							log = log.With(zap.Any("error", err))
						}

						log.With(zap.String("request", string(request))).Error("Caught panic")
					} else {
						fmt.Printf("Caught panic: %v\n%s\n\nFrom request:\n%s", err, debug.Stack(), string(request))
					}
				}

				abortWithError(c, http.StatusInternalServerError, "")
			}
		}()

		c.Next()
	}
}

// NewLogger returns a new low-level `*zap.Logger` instance. If the current
// terminal is a TTY, it will try ot use colored output automatically.
func NewLogger() (*zap.Logger, error) {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		config := zap.NewDevelopmentConfig()
		logLevel = &config.Level
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeTime = iso8601UTCTimeEncoder
		return config.Build()
	}

	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = iso8601UTCTimeEncoder
	logLevel = &config.Level
	return config.Build()
}

// A UTC variation of ZapCore.ISO8601TimeEncoder with millisecond precision
func iso8601UTCTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.UTC().Format("2006-01-02T15:04:05.000Z"))
}

// LogOption is used to set optional configuration for logging.
type LogOption func(*zap.Logger) *zap.Logger

// Logger sets the Zap logger to use. If not given, a default one will be
// created instead. Use this as an override.
func Logger(log *zap.Logger) LogOption {
	return func(l *zap.Logger) *zap.Logger {
		return log
	}
}

// LogField adds a key/value pair to the logger's tag fields.
func LogField(name, value string) LogOption {
	return func(l *zap.Logger) *zap.Logger {
		return l.With(zap.String(name, value))
	}
}

// LogMiddleware creates a new middleware to set a tagged `*zap.SugarLogger` in the
// Gin context under the `log` key. It debug logs request info. If passed `nil`
// for the logger, then it creates one. If the current terminal is a TTY, it
// will try to use colored output automatically.
func LogMiddleware(options ...LogOption) Middleware {
	var err error
	var l *zap.Logger
	if l, err = NewLogger(); err != nil {
		panic(err)
	}

	// Add any additional tags that were passed.
	for _, option := range options {
		l = option(l)
	}

	return func(c *gin.Context) {
		start := time.Now()
		contextLog := l.With(
			zap.String("method", c.Request.Method),
			zap.String("template", c.FullPath()),
			zap.String("path", c.Request.URL.RequestURI()),
			zap.String("ip", c.ClientIP()),
		)
		c.Set("log", contextLog.Sugar())

		c.Next()

		contextLog = contextLog.With(
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", time.Since(start)),
		)

		if len(c.Errors) > 0 {
			for _, e := range c.Errors {
				contextLog.Error("Error", zap.Error(e.Err))
			}
		}

		contextLog.Debug("Request")
	}
}

// LogDependency returns a dependency that resolves to a `*zap.SugaredLogger`
// for the current request. This dependency *requires* the use of
// `LogMiddleware` and will error if the logger is not in the request context.
func LogDependency() DependencyOption {
	dep := newDependency(DependencyOptions(
		GinContextDependency(),
		OperationIDDependency(),
	), func(c *gin.Context, opID string) (*zap.SugaredLogger, error) {
		l, ok := c.Get("log")
		if !ok {
			return nil, fmt.Errorf("missing logger in context")
		}
		sl := l.(*zap.SugaredLogger).With("operation", opID)
		return sl, nil
	})

	return &dependencyOption{func(d *openAPIDependency) {
		d.dependencies = append(d.dependencies, dep)
	}}
}

// Handler404 will return JSON responses for 404 errors.
func Handler404() Handler {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/" {
			// Special case: just return an HTTP 204 for the root rather than an error
			// if no custom root handler has been defined. This can be combined with
			// the ServiceLinkMiddleware to provide service description links.
			c.Status(http.StatusNoContent)
			return
		}

		c.Header("content-type", "application/problem+json")
		c.JSON(http.StatusNotFound, &ErrorModel{
			Status: http.StatusNotFound,
			Title:  http.StatusText(http.StatusNotFound),
			Detail: "Requested: " + c.Request.Method + " " + c.Request.URL.RequestURI(),
		})
	}
}

type minimalWriter struct {
	gin.ResponseWriter
	w http.ResponseWriter
}

func (w *minimalWriter) Write(data []byte) (int, error) {
	if w.Status() == http.StatusNoContent {
		return 0, nil
	}

	return w.ResponseWriter.Write(data)
}

func (w *minimalWriter) WriteHeader(statusCode int) {
	if statusCode >= 200 && statusCode < 300 {
		statusCode = http.StatusNoContent
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

// PreferMinimalMiddleware will remove the response body and return 204 No
// Content for any 2xx response where the request had the Prefer: return=minimal
// set on the request.
func PreferMinimalMiddleware() Middleware {
	return func(c *gin.Context) {
		// Wrap the response writer
		if c.GetHeader("prefer") == "return=minimal" {
			c.Writer = &minimalWriter{ResponseWriter: c.Writer}
		}

		c.Next()
	}
}

// AddServiceLinks addds RFC 8631 `service-desc` and `service-doc` link
// relations to the response. Safe to call multiple times and after a link
// header has already been set (it will append to it).
func AddServiceLinks(c *gin.Context) {
	link := c.Writer.Header().Get("link")
	if link != "" {
		link += ", "
	}
	link += `</openapi.json>; rel="service-desc", </docs>; rel="service-doc"`
	c.Header("link", link)
}

// ServiceLinkMiddleware addds RFC 8631 `service-desc` and `service-doc` link
// relations to the root response of the API.
func ServiceLinkMiddleware() Middleware {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/" {
			AddServiceLinks(c)
		}
		c.Next()
	}
}

// selectQValue selects and returns the best value from the allowed set
// given a header with optional quality values, as you would get for an
// Accept or Accept-Encoding header. The *first* item in allowed is preferred
// if there is a tie. If nothing matches, returns an empty string.
func selectQValue(header string, allowed []string) string {
	formats := strings.Split(header, ",")
	best := ""
	bestQ := 0.0
	for _, format := range formats {
		parts := strings.Split(format, ";")
		name := strings.Trim(parts[0], " \t")

		found := false
		for _, n := range allowed {
			if n == name {
				found = true
				break
			}
		}

		if !found {
			// Skip formats we don't support.
			continue
		}

		// Default weight to 1 if no value is passed.
		q := 1.0
		if len(parts) > 1 {
			trimmed := strings.Trim(parts[1], " \t")
			if strings.HasPrefix(trimmed, "q=") {
				q, _ = strconv.ParseFloat(trimmed[2:], 64)
			}
		}

		// Prefer the first one if there is a tie.
		if q > bestQ || (q == bestQ && name == allowed[0]) {
			bestQ = q
			best = name
		}
	}

	return best
}

type contentEncodingWriter struct {
	gin.ResponseWriter
	status      int
	encoding    string
	buf         *bytes.Buffer
	writer      io.Writer
	minSize     int
	gzPool      *sync.Pool
	brPool      *sync.Pool
	wroteHeader bool
}

func (w *contentEncodingWriter) Write(data []byte) (int, error) {
	if w.writer != nil {
		// We are writing compressed data.
		return w.writer.Write(data)
	}

	// Buffer the data until we can decide whether to compress it or not.
	w.buf.Write(data)

	cl, _ := strconv.Atoi(w.Header().Get("Content-Length"))
	if cl >= w.minSize || w.buf.Len() >= w.minSize {
		// We reached out minimum compression size. Set the writer, write the buffer
		// and make sure to set the correct headers.
		switch w.encoding {
		case "gzip":
			gz := w.gzPool.Get().(*gzip.Writer)
			gz.Reset(w.ResponseWriter)
			w.writer = gz
		case "br":
			br := w.brPool.Get().(*brotli.Writer)
			br.Reset(w.ResponseWriter)
			w.writer = br
		}
		w.Header().Set("Content-Encoding", w.encoding)
		w.Header().Set("Vary", "Accept-Encoding")
		w.ResponseWriter.WriteHeader(w.status)
		w.wroteHeader = true
		bufData := w.buf.Bytes()
		w.buf.Reset()
		return w.writer.Write(bufData)
	}

	// Not sure yet whether this should be compressed.
	return len(data), nil
}

func (w *contentEncodingWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.status = code
}

func (w *contentEncodingWriter) Close() {
	if !w.wroteHeader {
		w.ResponseWriter.WriteHeader(w.status)
	}

	if w.buf.Len() > 0 {
		w.ResponseWriter.Write(w.buf.Bytes())
	}

	if w.writer != nil {
		if wc, ok := w.writer.(io.WriteCloser); ok {
			wc.Close()
		}
	}
}

// ContentEncodingMiddleware uses content negotiation with the client to pick
// an appropriate encoding (compression) method and transparently encodes
// the response. Supports GZip and Brotli.
func ContentEncodingMiddleware() Middleware {
	// Use pools to reduce allocations. We use a byte buffer to temporarily store
	// some of each response in order to determine whether compression should
	// be applied. The others are just re-using the GZip and Brotli compressors.
	bufPool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	gzPool := sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(ioutil.Discard)
		},
	}

	brPool := sync.Pool{
		New: func() interface{} {
			return brotli.NewWriter(ioutil.Discard)
		},
	}

	return func(c *gin.Context) {
		if ext := path.Ext(c.Request.URL.Path); ext == ".gif" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".zip" {
			c.Next()
			return
		}

		if ac := c.Request.Header.Get("Accept-Encoding"); ac != "" {
			best := selectQValue(ac, []string{"br", "gzip"})

			if best != "" {
				buf := bufPool.Get().(*bytes.Buffer)
				buf.Reset()
				defer bufPool.Put(buf)

				cew := &contentEncodingWriter{
					ResponseWriter: c.Writer,
					encoding:       best,
					buf:            buf,
					gzPool:         &gzPool,
					brPool:         &brPool,

					// minSize of the body at which compression is enabled. Internet MTU
					// size is 1500 bytes, so anything smaller will still require sending
					// at least that size. 1400 seems to be a sane default.
					minSize: 1400,
				}
				// Status/headers are cached before any data is sent. Calling
				// ensureHeaders makes sure we always send the headers, even for 204
				// responses with no content. It's safe to call even if data was
				// written, in which case this is a no-op.
				defer cew.Close()
				c.Writer = cew
			}
		}

		c.Next()
	}
}
