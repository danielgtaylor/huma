package humagin

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

type ginCtx struct {
	op   *huma.Operation
	orig *gin.Context
}

func (ctx *ginCtx) GetOperation() *huma.Operation {
	return ctx.op
}

func (c *ginCtx) GetContext() context.Context {
	return c.orig.Request.Context()
}

func (c *ginCtx) GetMethod() string {
	return c.orig.Request.Method
}

func (c *ginCtx) GetURL() url.URL {
	return *c.orig.Request.URL
}

func (c *ginCtx) GetParam(name string) string {
	return c.orig.Param(name)
}

func (c *ginCtx) GetQuery(name string) string {
	return c.orig.Query(name)
}

func (c *ginCtx) GetHeader(name string) string {
	return c.orig.GetHeader(name)
}

func (c *ginCtx) EachHeader(cb func(name, value string)) {
	for name, values := range c.orig.Request.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *ginCtx) GetBodyReader() io.Reader {
	return c.orig.Request.Body
}

func (ctx *ginCtx) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(ctx.orig.Writer, deadline)
}

func (c *ginCtx) WriteStatus(code int) {
	c.orig.Status(code)
}

func (c *ginCtx) AppendHeader(name string, value string) {
	c.orig.Writer.Header().Add(name, value)
}

func (c *ginCtx) WriteHeader(name string, value string) {
	c.orig.Header(name, value)
}

func (c *ginCtx) BodyWriter() io.Writer {
	return c.orig.Writer
}

type ginAdapter struct {
	router *gin.Engine
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

func (a *ginAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

func New(r *gin.Engine, config huma.Config) huma.API {
	return huma.NewAPI(config, &ginAdapter{router: r})
}
