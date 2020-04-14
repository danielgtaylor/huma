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
// panic when using the recovery middleware. Defaults to 10KB.
var MaxLogBodyBytes int64 = 10 * 1024

// Middleware TODO ...
type Middleware = gin.HandlerFunc

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
						log.With(zap.String("request", string(request)), zap.Error(err.(error))).Error("Caught panic")
					} else {
						fmt.Printf("Caught panic: %v\n%s\n\nFrom request:\n%s", err, debug.Stack(), string(request))
					}
				}

				c.AbortWithStatusJSON(http.StatusInternalServerError, &ErrorModel{
					Message: "Internal server error",
				})
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

// LogMiddleware creates a new middleware to set a tagged `*zap.SugarLogger` in the
// Gin context under the `log` key. It debug logs request info. If passed `nil`
// for the logger, then it creates one. If the current terminal is a TTY, it
// will try to use colored output automatically.
func LogMiddleware(l *zap.Logger, tags map[string]string) Middleware {
	var err error
	if l == nil {
		if l, err = NewLogger(); err != nil {
			panic(err)
		}
	}

	// Add any additional tags that were passed.
	for k, v := range tags {
		l = l.With(zap.String(k, v))
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
	dep := NewDependency(DependencyOptions(
		GinContextDependency(),
		OperationDependency(),
	), func(c *gin.Context, op *OpenAPIOperation) (*zap.SugaredLogger, error) {
		l, ok := c.Get("log")
		if !ok {
			return nil, fmt.Errorf("missing logger in context")
		}
		sl := l.(*zap.SugaredLogger).With("operation", op.id)
		return sl, nil
	})

	return &dependencyOption{func(d *OpenAPIDependency) {
		d.dependencies = append(d.dependencies, dep)
	}}
}

// Handler404 will return JSON responses for 404 errors.
func Handler404(c *gin.Context) {
	c.JSON(http.StatusNotFound, &ErrorModel{
		Message: "Not found",
	})
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
