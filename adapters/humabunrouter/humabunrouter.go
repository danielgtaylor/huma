package humabunrouter

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/uptrace/bunrouter"
)

// MultipartMaxMemory is the maximum memory to use when parsing multipart
// form data.
var MultipartMaxMemory int64 = 8 * 1024

type bunContext struct {
	op     *huma.Operation
	r      bunrouter.Request
	w      http.ResponseWriter
	status int
}

// check that bunContext implements huma.Context
var _ huma.Context = &bunContext{}

func (c *bunContext) Operation() *huma.Operation {
	return c.op
}

func (c *bunContext) Context() context.Context {
	return c.r.Context()
}

func (c *bunContext) Method() string {
	return c.r.Method
}

func (c *bunContext) Host() string {
	return c.r.Host
}

func (c *bunContext) RemoteAddr() string {
	return c.r.RemoteAddr
}

func (c *bunContext) URL() url.URL {
	return *c.r.URL
}

func (c *bunContext) Param(name string) string {
	return c.r.Param(name)
}

func (c *bunContext) Query(name string) string {
	return queryparam.Get(c.r.URL.RawQuery, name)
}

func (c *bunContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *bunContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *bunContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *bunContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(MultipartMaxMemory)
	return c.r.MultipartForm, err
}

func (c *bunContext) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.w, deadline)
}

func (c *bunContext) SetStatus(code int) {
	c.status = code
	c.w.WriteHeader(code)
}

func (c *bunContext) Status() int {
	return c.status
}

func (c *bunContext) AppendHeader(name string, value string) {
	c.w.Header().Add(name, value)
}

func (c *bunContext) SetHeader(name string, value string) {
	c.w.Header().Set(name, value)
}

func (c *bunContext) BodyWriter() io.Writer {
	return c.w
}

// NewContext creates a new Huma context from an HTTP request and response.
func NewContext(op *huma.Operation, r bunrouter.Request, w http.ResponseWriter) huma.Context {
	return &bunContext{op: op, r: r, w: w}
}

type bunCompatContext struct {
	op     *huma.Operation
	r      *http.Request
	w      http.ResponseWriter
	status int
}

func (c *bunCompatContext) Operation() *huma.Operation {
	return c.op
}

func (c *bunCompatContext) Context() context.Context {
	return c.r.Context()
}

func (c *bunCompatContext) Method() string {
	return c.r.Method
}

func (c *bunCompatContext) Host() string {
	return c.r.Host
}

func (c *bunCompatContext) RemoteAddr() string {
	return c.r.RemoteAddr
}

func (c *bunCompatContext) URL() url.URL {
	return *c.r.URL
}

func (c *bunCompatContext) Param(name string) string {
	params := bunrouter.ParamsFromContext(c.r.Context())
	return params.ByName(name)
}

func (c *bunCompatContext) Query(name string) string {
	return queryparam.Get(c.r.URL.RawQuery, name)
}

func (c *bunCompatContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *bunCompatContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *bunCompatContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *bunCompatContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(8 * 1024)
	return c.r.MultipartForm, err
}

func (c *bunCompatContext) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.w, deadline)
}

func (c *bunCompatContext) SetStatus(code int) {
	c.status = code
	c.w.WriteHeader(code)
}

func (c *bunCompatContext) Status() int {
	return c.status
}

func (c *bunCompatContext) AppendHeader(name string, value string) {
	c.w.Header().Add(name, value)
}

func (c *bunCompatContext) SetHeader(name string, value string) {
	c.w.Header().Set(name, value)
}

func (c *bunCompatContext) BodyWriter() io.Writer {
	return c.w
}

// NewCompatContext creates a new Huma context from an HTTP request and response.
func NewCompatContext(op *huma.Operation, r *http.Request, w http.ResponseWriter) huma.Context {
	return &bunCompatContext{op: op, r: r, w: w}
}

type bunCompatAdapter struct {
	router *bunrouter.CompatRouter
}

func (a *bunCompatAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Handle(op.Method, path, func(w http.ResponseWriter, r *http.Request) {
		handler(NewCompatContext(op, r, w))
	})
}

func (a *bunCompatAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// NewCompatAdapter creates a new adapter for the given bunrouter compat router.
func NewCompatAdapter(r *bunrouter.CompatRouter) huma.Adapter {
	return &bunCompatAdapter{router: r}
}

type bunAdapter struct {
	router *bunrouter.Router
}

func (a *bunAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Handle(op.Method, path, func(w http.ResponseWriter, r bunrouter.Request) error {
		var err error
		defer func() {
			if re := recover(); re != nil {
				err = fmt.Errorf("panic: %v", re)
			}
		}()

		handler(NewContext(op, r, w))

		return err
	})
}

func (a *bunAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// NewAdapter creates a new adapter for the given bunrouter router.
func NewAdapter(r *bunrouter.Router) huma.Adapter {
	return &bunAdapter{router: r}
}

// NewCompat creates a new Huma API using *bunrouter.CompatRouter.
func NewCompat(r *bunrouter.CompatRouter, config huma.Config) huma.API {
	return huma.NewAPI(config, NewCompatAdapter(r))
}

// New creates a new Huma API using *bunrouter.Router.
func New(r *bunrouter.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, NewAdapter(r))
}
