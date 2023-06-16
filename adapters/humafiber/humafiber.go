package humafiber

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
)

type fiberCtx struct {
	op   *huma.Operation
	orig *fiber.Ctx
}

func (ctx *fiberCtx) GetOperation() *huma.Operation {
	return ctx.op
}

func (c *fiberCtx) GetMatched() string {
	return c.orig.Route().Path
}

func (c *fiberCtx) GetContext() context.Context {
	return c.orig.Context()
}

func (c *fiberCtx) GetMethod() string {
	return c.orig.Method()
}

func (ctx *fiberCtx) GetHost() string {
	return ctx.orig.Hostname()
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

func (c *fiberCtx) EachHeader(cb func(name, value string)) {
	c.orig.Request().Header.VisitAll(func(k, v []byte) {
		cb(string(k), string(v))
	})
}

func (c *fiberCtx) GetBodyReader() io.Reader {
	return c.orig.Request().BodyStream()
}

func (c *fiberCtx) SetReadDeadline(deadline time.Time) error {
	// Note: for this to work properly you need to do two things:
	// 1. Set the Fiber app's `StreamRequestBody` to `true`
	// 2. Set the Fiber app's `BodyLimit` to some small value like `1`
	// Fiber will only call the request handler for streaming once the limit is
	// reached. This is annoying but currently how things work.
	return c.orig.Context().Conn().SetReadDeadline(deadline)
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
	router *fiber.App
}

func (a *fiberAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Add(op.Method, path, func(c *fiber.Ctx) error {
		ctx := &fiberCtx{op: op, orig: c}
		handler(ctx)
		return nil
	})
}

func (a *fiberAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// b, _ := httputil.DumpRequest(r, true)
	// fmt.Println(string(b))
	resp, err := a.router.Test(r)
	if err != nil {
		panic(err)
	}
	h := w.Header()
	for k, v := range resp.Header {
		for item := range v {
			h.Add(k, v[item])
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func New(r *fiber.App, config huma.Config) huma.API {
	return huma.NewAPI(config, &fiberAdapter{router: r})
}
