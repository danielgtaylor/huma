package humachi

import (
	"context"
	"crypto/tls"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/go-chi/chi/v5"
)

// MultipartMaxMemory is the maximum memory to use when parsing multipart
// form data.
var MultipartMaxMemory int64 = 8 * 1024

// Unwrap extracts the underlying HTTP request and response writer from a Huma
// context. If passed a context from a different adapter it will panic.
func Unwrap(ctx huma.Context) (*http.Request, http.ResponseWriter) {
	for {
		if c, ok := ctx.(interface{ Unwrap() huma.Context }); ok {
			ctx = c.Unwrap()
			continue
		}
		break
	}
	if c, ok := ctx.(*chiContext); ok {
		return c.Unwrap()
	}
	panic("not a humachi context")
}

type chiContext struct {
	op     *huma.Operation
	r      *http.Request
	w      http.ResponseWriter
	status int
}

// check that chiContext implements huma.Context
var _ huma.Context = &chiContext{}

func (c *chiContext) Unwrap() (*http.Request, http.ResponseWriter) {
	return c.r, c.w
}

func (c *chiContext) Operation() *huma.Operation {
	return c.op
}

func (c *chiContext) Context() context.Context {
	return c.r.Context()
}

func (c *chiContext) Method() string {
	return c.r.Method
}

func (c *chiContext) Host() string {
	return c.r.Host
}

func (c *chiContext) RemoteAddr() string {
	return c.r.RemoteAddr
}

func (c *chiContext) URL() url.URL {
	return *c.r.URL
}

func (c *chiContext) Param(name string) string {
	v := c.r.PathValue(name)
	if c.r.URL.RawPath == "" {
		return v // RawPath empty means no escaping was done
	}
	u, err := url.PathUnescape(v)
	if err != nil {
		return v // not supposed to happen, but if it does, return the original value
	}
	return u
}

func (c *chiContext) Query(name string) string {
	return queryparam.Get(c.r.URL.RawQuery, name)
}

func (c *chiContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *chiContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *chiContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *chiContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(MultipartMaxMemory)
	return c.r.MultipartForm, err
}

func (c *chiContext) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.w, deadline)
}

func (c *chiContext) SetStatus(code int) {
	c.status = code
	c.w.WriteHeader(code)
}

func (c *chiContext) Status() int {
	return c.status
}

func (c *chiContext) AppendHeader(name string, value string) {
	c.w.Header().Add(name, value)
}

func (c *chiContext) SetHeader(name string, value string) {
	c.w.Header().Set(name, value)
}

func (c *chiContext) BodyWriter() io.Writer {
	return c.w
}

func (c *chiContext) TLS() *tls.ConnectionState {
	return c.r.TLS
}

func (c *chiContext) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto:      c.r.Proto,
		ProtoMajor: c.r.ProtoMajor,
		ProtoMinor: c.r.ProtoMinor,
	}
}

func (c *chiContext) WithContext(ctx context.Context) huma.Context {
	return &chiContext{
		op:     c.op,
		r:      c.r.WithContext(ctx),
		w:      c.w,
		status: c.status,
	}
}

// NewContext creates a new Huma context from an HTTP request and response.
func NewContext(op *huma.Operation, r *http.Request, w http.ResponseWriter) huma.Context {
	return &chiContext{op: op, r: r, w: w}
}

type chiAdapter struct {
	router chi.Router
}

func (a *chiAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.router.MethodFunc(op.Method, op.Path, func(w http.ResponseWriter, r *http.Request) {
		handler(&chiContext{op: op, r: r, w: w})
	})
}

func (a *chiAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// NewAdapter creates a new adapter for the given chi router.
func NewAdapter(r chi.Router) huma.Adapter {
	return &chiAdapter{router: r}
}

// New creates a new Huma API using the latest v5.x.x version of Chi.
func New(r chi.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &chiAdapter{router: r})
}

func middleware(mw func(http.Handler) http.Handler) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, w := Unwrap(ctx)
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx = NewContext(ctx.Operation(), r, w)
			next(ctx)
		})).ServeHTTP(w, r)
	}
}
