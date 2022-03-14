package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"runtime/debug"
	"sync"

	"github.com/danielgtaylor/huma"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

var bufContextKey contextKey = "huma-middleware-body-buffer"

// GetBufferedBody returns the buffered body from a request when using the
// recovery middleware, up to MaxLogBodyBytes.
func GetBufferedBody(ctx context.Context) []byte {
	if val := ctx.Value(bufContextKey); val != nil {
		buf := val.(*bytes.Buffer)
		data, _ := ioutil.ReadAll(ioutil.NopCloser(buf))
		return data
	}
	return []byte{}
}

// MaxLogBodyBytes logs at most this many bytes of any request body during a
// panic when using the recovery middleware. Defaults to 10KiB. Changing this
// value changes the amount of potential memory used for *each* incoming
// request, so change it carefully and complement the change with load testing
// because larger values can have a detrimental effect on the server.
var MaxLogBodyBytes int64 = 10 * 1024

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

// RemovedHeaders defines a list of HTTP headers that will be redacted from the
// request in the Recovery handler--if any logging or other output occurs, these
// headings will have value '<redacted>'. By default, a huma service removes the
// 'Authorization' header to avoid leaking sensitive information, but clients
// can override this with an empty slice.
var RemovedHeaders = []string{"Authorization"}

const redacted = "<redacted>"

// PanicFunc defines a function to run after a panic, which allows you to set
// up custom logging, metrics, etc.
type PanicFunc func(ctx context.Context, err error, request string)

// Recovery prints stack traces on panic when used with the logging middleware.
func Recovery(onPanic PanicFunc) func(http.Handler) http.Handler {
	bufPool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var buf *bytes.Buffer

			// Reset the body so other middleware/handlers can use it.
			if r.Body != nil {
				// Get a buffer that the body will be read into.
				buf = bufPool.Get().(*bytes.Buffer)
				defer bufPool.Put(buf)

				r.Body = newBufferedReadCloser(r.Body, buf, MaxLogBodyBytes)

				r = r.WithContext(context.WithValue(r.Context(), bufContextKey, buf))
			}

			for _, v := range RemovedHeaders {
				r.Header.Set(v, redacted)
			}

			// Recovering comes *after* the above so the buffer is not returned to
			// the pool until after we print out its contents. This deferred func
			// is used to recover from panics and deliberately left in-line.
			defer func() {
				if err := recover(); err != nil {
					// The body might have been read or partially read, so replace it
					// with a clean reader to dump out up to maxBodyBytes with the error.
					if buf != nil && buf.Len() != 0 {
						r.Body = ioutil.NopCloser(buf)
					} else if r.Body != nil {
						defer r.Body.Close()
						r.Body = ioutil.NopCloser(io.LimitReader(r.Body, MaxLogBodyBytes))
					}

					request, _ := httputil.DumpRequest(r, true)

					if _, ok := err.(error); !ok {
						err = fmt.Errorf("%v", err)
					}

					if onPanic != nil {
						onPanic(r.Context(), err.(error), string(request))
					} else {
						// Fall back to the standard library logger.
						log.Printf("Caught panic: %v\n%s\n\nFrom request:\n%s", err, debug.Stack(), string(request))
					}

					// If OpenTracing is enabled, augment the span with error info
					if span := opentracing.SpanFromContext(r.Context()); span != nil {
						span.SetTag(string(ext.Error), err)
					}

					ctx := huma.ContextFromRequest(w, r)
					ctx.WriteError(http.StatusInternalServerError, "Unrecoverable internal server error")
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
