package middleware

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"runtime/debug"
	"sync"

	"github.com/danielgtaylor/huma"
	"github.com/go-chi/chi"
	"go.uber.org/zap"
)

// MaxLogBodyBytes logs at most this many bytes of any request body during a
// panic when using the recovery middleware. Defaults to 10KiB.
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

// Recovery prints stack traces on panic when used with the logging middleware.
func Recovery(next http.Handler) http.Handler {
	bufPool := sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf *bytes.Buffer

		// Reset the body so other middleware/handlers can use it.
		if r.Body != nil {
			// Get a buffer that the body will be read into.
			buf = bufPool.Get().(*bytes.Buffer)
			defer bufPool.Put(buf)

			r.Body = newBufferedReadCloser(r.Body, buf, MaxLogBodyBytes)
		}

		// Recovering comes *after* the above so the buffer is not returned to
		// the pool until after we print out its contents.
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

				log := GetLogger(r.Context())
				if log != nil {
					if e, ok := err.(error); ok {
						log = log.With(zap.Error(e))
					} else {
						log = log.With(zap.Any("error", err))
					}
					log.With(
						zap.String("request", string(request)),
						zap.String("template", chi.RouteContext(r.Context()).RoutePattern()),
					).Error("Caught panic")
				} else {
					fmt.Printf("Caught panic: %v\n%s\n\nFrom request:\n%s", err, debug.Stack(), string(request))
				}

				ctx := huma.ContextFromRequest(w, r)
				ctx.WriteError(http.StatusInternalServerError, "Unrecoverable internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}
