package humaecho

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
	echoV4 "github.com/labstack/echo/v4"
)

// MultipartMaxMemory is the maximum memory to use when parsing multipart
// form data.
var MultipartMaxMemoryv4 int64 = 8 * 1024

// Unwrap extracts the underlying Echo v4.x context from a Huma context. If passed a
// context from a different adapter it will panic.
func UnwrapV4(ctx huma.Context) echoV4.Context {
	for {
		if c, ok := ctx.(interface{ Unwrap() huma.Context }); ok {
			ctx = c.Unwrap()
			continue
		}
		break
	}
	if c, ok := ctx.(*echoV4Ctx); ok {
		return c.Unwrap()
	}
	panic("not a humaecho context")
}

type echoV4Ctx struct {
	op     *huma.Operation
	orig   echoV4.Context
	status int
}

// check that echoCtx implements huma.Context
var _ huma.Context = &echoV4Ctx{}

func (c *echoV4Ctx) Unwrap() echoV4.Context {
	return c.orig
}

func (c *echoV4Ctx) Operation() *huma.Operation {
	return c.op
}

func (c *echoV4Ctx) Context() context.Context {
	return c.orig.Request().Context()
}

func (c *echoV4Ctx) Method() string {
	return c.orig.Request().Method
}

func (c *echoV4Ctx) Host() string {
	return c.orig.Request().Host
}

func (c *echoV4Ctx) RemoteAddr() string {
	return c.orig.Request().RemoteAddr
}

func (c *echoV4Ctx) URL() url.URL {
	return *c.orig.Request().URL
}

func (c *echoV4Ctx) Param(name string) string {
	return c.orig.Param(name)
}

func (c *echoV4Ctx) Query(name string) string {
	return c.orig.QueryParam(name)
}

func (c *echoV4Ctx) Header(name string) string {
	return c.orig.Request().Header.Get(name)
}

func (c *echoV4Ctx) EachHeader(cb func(name, value string)) {
	for name, values := range c.orig.Request().Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *echoV4Ctx) BodyReader() io.Reader {
	return c.orig.Request().Body
}

func (c *echoV4Ctx) GetMultipartForm() (*multipart.Form, error) {
	err := c.orig.Request().ParseMultipartForm(MultipartMaxMemory)
	return c.orig.Request().MultipartForm, err
}

func (c *echoV4Ctx) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.orig.Response(), deadline)
}

func (c *echoV4Ctx) SetStatus(code int) {
	c.status = code
	c.orig.Response().WriteHeader(code)
}

func (c *echoV4Ctx) Status() int {
	return c.status
}

func (c *echoV4Ctx) AppendHeader(name, value string) {
	c.orig.Response().Header().Add(name, value)
}

func (c *echoV4Ctx) SetHeader(name, value string) {
	c.orig.Response().Header().Set(name, value)
}

func (c *echoV4Ctx) BodyWriter() io.Writer {
	return c.orig.Response()
}

func (c *echoV4Ctx) TLS() *tls.ConnectionState {
	return c.orig.Request().TLS
}

func (c *echoV4Ctx) Version() huma.ProtoVersion {
	r := c.orig.Request()
	return huma.ProtoVersion{
		Proto:      r.Proto,
		ProtoMajor: r.ProtoMajor,
		ProtoMinor: r.ProtoMinor,
	}
}

type routerV4 interface {
	Add(method, path string, handler echoV4.HandlerFunc, middlewares ...echoV4.MiddlewareFunc) *echoV4.Route
}

type echoV4Adapter struct {
	http.Handler
	router routerV4
}

func (a *echoV4Adapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Add(op.Method, path, func(c echoV4.Context) error {
		ctx := &echoV4Ctx{op: op, orig: c}
		handler(ctx)
		return nil
	})
}

func NewV4(r *echoV4.Echo, config huma.Config) huma.API {
	return huma.NewAPI(config, &echoV4Adapter{Handler: r, router: r})
}

// NewWithGroupV4 creates a new Huma API using the provided echo v4.x router and group,
// letting you mount the API at a sub-path. Can be used in combination with
// the `OpenAPI.Servers` field to set the correct base URL for the API / docs
// / schemas / etc.
func NewWithGroupV4(r *echoV4.Echo, g *echoV4.Group, config huma.Config) huma.API {
	return huma.NewAPI(config, &echoV4Adapter{Handler: r, router: g})
}
