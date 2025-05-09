package humago

import (
	"context"
	"crypto/tls"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
)

// MultipartMaxMemory is the maximum memory to use when parsing multipart
// form data.
var MultipartMaxMemory int64 = 8 * 1024

// Unwrap extracts the underlying HTTP request and response writer from a Huma
// context. If passed a context from a different adapter it will panic.
func Unwrap(ctx huma.Context) (*http.Request, http.ResponseWriter) {
	if c, ok := ctx.(*goContext); ok {
		return c.Unwrap()
	}
	panic("not a humago context")
}

type goContext struct {
	op     *huma.Operation
	r      *http.Request
	w      http.ResponseWriter
	status int
}

// check that goContext implements huma.Context
var _ huma.Context = &goContext{}

func (c *goContext) Unwrap() (*http.Request, http.ResponseWriter) {
	return c.r, c.w
}

func (c *goContext) Operation() *huma.Operation {
	return c.op
}

func (c *goContext) Context() context.Context {
	return c.r.Context()
}

func (c *goContext) Method() string {
	return c.r.Method
}

func (c *goContext) Host() string {
	return c.r.Host
}

func (c *goContext) RemoteAddr() string {
	return c.r.RemoteAddr
}

func (c *goContext) URL() url.URL {
	return *c.r.URL
}

func (c *goContext) Param(name string) string {
	return c.r.PathValue(name)
}

func (c *goContext) Query(name string) string {
	return queryparam.Get(c.r.URL.RawQuery, name)
}

func (c *goContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *goContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *goContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *goContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(MultipartMaxMemory)
	return c.r.MultipartForm, err
}

func (c *goContext) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.w, deadline)
}

func (c *goContext) SetStatus(code int) {
	c.status = code
	c.w.WriteHeader(code)
}

func (c *goContext) Status() int {
	return c.status
}

func (c *goContext) AppendHeader(name string, value string) {
	c.w.Header().Add(name, value)
}

func (c *goContext) SetHeader(name string, value string) {
	c.w.Header().Set(name, value)
}

func (c *goContext) BodyWriter() io.Writer {
	return c.w
}

func (c *goContext) TLS() *tls.ConnectionState {
	return c.r.TLS
}

func (c *goContext) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto:      c.r.Proto,
		ProtoMajor: c.r.ProtoMajor,
		ProtoMinor: c.r.ProtoMinor,
	}
}

// NewContext creates a new Huma context from an HTTP request and response.
func NewContext(op *huma.Operation, r *http.Request, w http.ResponseWriter) huma.Context {
	return &goContext{op: op, r: r, w: w}
}

type Mux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
	ServeHTTP(http.ResponseWriter, *http.Request)
}

type goAdapter struct {
	Mux
	prefix string
}

func (a *goAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.HandleFunc(strings.ToUpper(op.Method)+" "+a.prefix+op.Path, func(w http.ResponseWriter, r *http.Request) {
		handler(&goContext{op: op, r: r, w: w})
	})
}

// NewAdapter creates a new adapter for the given HTTP mux.
func NewAdapter(m Mux, prefix string) huma.Adapter {
	return &goAdapter{m, prefix}
}

// New creates a new Huma API using an HTTP mux.
//
//	mux := http.NewServeMux()
//	api := humago.New(mux, huma.DefaultConfig("My API", "1.0.0"))
func New(m Mux, config huma.Config) huma.API {
	return huma.NewAPI(config, &goAdapter{m, ""})
}

// NewWithPrefix creates a new Huma API using an HTTP mux with a URL prefix.
// This behaves similar to other router's group functionality, adding the prefix
// before each route path (but not in the OpenAPI). The prefix should be used in
// combination with the `OpenAPI().Servers` base path to ensure the correct URLs
// are generated in the OpenAPI spec.
//
//	mux := http.NewServeMux()
//	config := huma.DefaultConfig("My API", "1.0.0")
//	config.Servers = []*huma.Server{{URL: "http://example.com/api"}}
//	api := humago.NewWithPrefix(mux, "/api", config)
func NewWithPrefix(m Mux, prefix string, config huma.Config) huma.API {
	if len(config.Servers) == 0 {
		config.Servers = append(config.Servers, &huma.Server{
			URL: prefix,
		})
	}
	return huma.NewAPI(config, &goAdapter{m, prefix})
}
