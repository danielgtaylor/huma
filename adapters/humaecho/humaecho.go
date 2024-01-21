package humaecho

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/labstack/echo/v4"
)

type echoCtx struct {
	op   *huma.Operation
	orig echo.Context
}

func (c *echoCtx) Operation() *huma.Operation {
	return c.op
}

func (c *echoCtx) Context() context.Context {
	return c.orig.Request().Context()
}

func (c *echoCtx) Method() string {
	return c.orig.Request().Method
}

func (c *echoCtx) Host() string {
	return c.orig.Request().Host
}

func (c *echoCtx) URL() url.URL {
	return *c.orig.Request().URL
}

func (c *echoCtx) Param(name string) string {
	return c.orig.Param(name)
}

func (c *echoCtx) Query(name string) string {
	return c.orig.QueryParam(name)
}

func (c *echoCtx) Header(name string) string {
	return c.orig.Request().Header.Get(name)
}

func (c *echoCtx) EachHeader(cb func(name, value string)) {
	for name, values := range c.orig.Request().Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *echoCtx) BodyReader() io.Reader {
	return c.orig.Request().Body
}

func (c *echoCtx) GetMultipartForm() (*multipart.Form, error) {
	err := c.orig.Request().ParseMultipartForm(8 * 1024)
	return c.orig.Request().MultipartForm, err
}

func (c *echoCtx) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.orig.Response(), deadline)
}

func (c *echoCtx) SetStatus(code int) {
	c.orig.Response().WriteHeader(code)
}

func (c *echoCtx) AppendHeader(name, value string) {
	c.orig.Response().Header().Add(name, value)
}

func (c *echoCtx) SetHeader(name, value string) {
	c.orig.Response().Header().Set(name, value)
}

func (c *echoCtx) BodyWriter() io.Writer {
	return c.orig.Response()
}

type echoAdapter struct {
	router *echo.Echo
}

func (a *echoAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Add(op.Method, path, func(c echo.Context) error {
		ctx := &echoCtx{op: op, orig: c}
		handler(ctx)
		return nil
	})
}

func (a *echoAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

func New(r *echo.Echo, config huma.Config) huma.API {
	return huma.NewAPI(config, &echoAdapter{router: r})
}
