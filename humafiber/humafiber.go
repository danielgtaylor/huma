package humafiber

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

type fiberCtx struct {
	orig *fiber.Ctx
}

func (c *fiberCtx) GetMatched() string {
	return c.orig.Route().Path
}

func (c *fiberCtx) GetContext() context.Context {
	return c.orig.Context()
}

func (c *fiberCtx) GetURL() url.URL {
	u, _ := url.Parse(string(c.orig.Request().RequestURI()))
	return *u
}

func (c *fiberCtx) GetParam(name string) string {
	return c.orig.Params(name)
}

func (c *fiberCtx) GetQuery(name string) string {
	return c.orig.Query(name)
}

func (c *fiberCtx) GetHeader(name string) string {
	return c.orig.Get(name)
}

func (c *fiberCtx) GetBodyReader() io.Reader {
	// return c.orig.Context().RequestBodyStream()
	return bytes.NewReader(c.orig.Body())
}

func (c *fiberCtx) WriteStatus(code int) {
	c.orig.Status(code)
}

func (c *fiberCtx) AppendHeader(name string, value string) {
	c.orig.Append(name, value)
}

func (c *fiberCtx) WriteHeader(name string, value string) {
	c.orig.Set(name, value)
}

func (c *fiberCtx) BodyWriter() io.Writer {
	return c.orig
}

type fiberAdapter struct {
	router fiber.Router
}

func (a *fiberAdapter) Handle(method, path string, handler func(huma.Context)) {
	// Convert {param} to :param
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Add(method, path, func(c *fiber.Ctx) error {
		ctx := &fiberCtx{orig: c}
		handler(ctx)
		return nil
	})
}

func New(r fiber.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &fiberAdapter{router: r})
}
