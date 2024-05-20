package humahttprouter

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/julienschmidt/httprouter"
)

// MultipartMaxMemory is the maximum memory to use when parsing multipart
// form data.
var MultipartMaxMemory int64 = 8 * 1024

type httprouterContext struct {
	op     *huma.Operation
	r      *http.Request
	w      http.ResponseWriter
	ps     httprouter.Params
	status int
}

// check that httprouterContext implements huma.Context
var _ huma.Context = &httprouterContext{}

func (c *httprouterContext) Operation() *huma.Operation {
	return c.op
}

func (c *httprouterContext) Context() context.Context {
	return c.r.Context()
}

func (c *httprouterContext) Method() string {
	return c.r.Method
}

func (c *httprouterContext) Host() string {
	return c.r.Host
}

func (c *httprouterContext) RemoteAddr() string {
	return c.r.RemoteAddr
}

func (c *httprouterContext) URL() url.URL {
	return *c.r.URL
}

func (c *httprouterContext) Param(name string) string {
	return c.ps.ByName(name)
}

func (c *httprouterContext) Query(name string) string {
	return queryparam.Get(c.r.URL.RawQuery, name)
}

func (c *httprouterContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *httprouterContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *httprouterContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *httprouterContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(MultipartMaxMemory)
	return c.r.MultipartForm, err
}

func (c *httprouterContext) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.w, deadline)
}

func (c *httprouterContext) SetStatus(code int) {
	c.status = code
	c.w.WriteHeader(code)
}

func (c *httprouterContext) Status() int {
	return c.status
}

func (c *httprouterContext) AppendHeader(name string, value string) {
	c.w.Header().Add(name, value)
}

func (c *httprouterContext) SetHeader(name string, value string) {
	c.w.Header().Set(name, value)
}

func (c *httprouterContext) BodyWriter() io.Writer {
	return c.w
}

type httprouterAdapter struct {
	router *httprouter.Router
}

func (a *httprouterAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Handle(op.Method, path, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		handler(&httprouterContext{op: op, r: r, w: w, ps: ps})
	})
}

func (a *httprouterAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

func New(r *httprouter.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &httprouterAdapter{router: r})
}
