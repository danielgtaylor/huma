package huma

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"sync"
	"time"

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
		return config.Build()
	}

	config := zap.NewProductionConfig()
	logLevel = &config.Level
	return config.Build()
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
