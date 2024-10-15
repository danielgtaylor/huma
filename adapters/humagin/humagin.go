package humagin

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
	"github.com/gin-gonic/gin"
)

// MultipartMaxMemory is the maximum memory to use when parsing multipart
// form data.
var MultipartMaxMemory int64 = 8 * 1024

type ginCtx struct {
	op     *huma.Operation
	orig   *gin.Context
	status int
}

// check that ginCtx implements huma.Context
var _ huma.Context = &ginCtx{}

func (c *ginCtx) Operation() *huma.Operation {
	return c.op
}

func (c *ginCtx) Context() context.Context {
	return c.orig.Request.Context()
}

func (c *ginCtx) Method() string {
	return c.orig.Request.Method
}

func (c *ginCtx) Host() string {
	return c.orig.Request.Host
}

func (c *ginCtx) RemoteAddr() string {
	return c.orig.Request.RemoteAddr
}

func (c *ginCtx) URL() url.URL {
	return *c.orig.Request.URL
}

func (c *ginCtx) Param(name string) string {
	return c.orig.Param(name)
}

func (c *ginCtx) Query(name string) string {
	return c.orig.Query(name)
}

func (c *ginCtx) Header(name string) string {
	return c.orig.GetHeader(name)
}

func (c *ginCtx) EachHeader(cb func(name, value string)) {
	for name, values := range c.orig.Request.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *ginCtx) BodyReader() io.Reader {
	return c.orig.Request.Body
}

func (c *ginCtx) GetMultipartForm() (*multipart.Form, error) {
	err := c.orig.Request.ParseMultipartForm(MultipartMaxMemory)
	return c.orig.Request.MultipartForm, err
}

func (c *ginCtx) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.orig.Writer, deadline)
}

func (c *ginCtx) SetStatus(code int) {
	c.status = code
	c.orig.Status(code)
}

func (c *ginCtx) Status() int {
	return c.status
}

func (c *ginCtx) AppendHeader(name string, value string) {
	c.orig.Writer.Header().Add(name, value)
}

func (c *ginCtx) SetHeader(name string, value string) {
	c.orig.Header(name, value)
}

func (c *ginCtx) BodyWriter() io.Writer {
	return c.orig.Writer
}

func (c *ginCtx) TLS() *tls.ConnectionState {
	return c.orig.Request.TLS
}

// Router is an interface that wraps the Gin router's Handle method.
type Router interface {
	Handle(string, string, ...gin.HandlerFunc) gin.IRoutes
}

type ginAdapter struct {
	http.Handler
	router Router
}

func (a *ginAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Handle(op.Method, path, func(c *gin.Context) {
		ctx := &ginCtx{op: op, orig: c}
		handler(ctx)
	})
}

func New(r *gin.Engine, config huma.Config) huma.API {
	return huma.NewAPI(config, &ginAdapter{Handler: r, router: r})
}

// NewWithGroup creates a new Huma API using the provided Gin router and group,
// letting you mount the API at a sub-path. Can be used in combination with
// the `OpenAPI.Servers` field to set the correct base URL for the API / docs
// / schemas / etc.
func NewWithGroup(r *gin.Engine, g *gin.RouterGroup, config huma.Config) huma.API {
	return huma.NewAPI(config, &ginAdapter{Handler: r, router: g})
}
