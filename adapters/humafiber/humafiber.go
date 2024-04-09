package humafiber

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
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

func (c *fiberCtx) Operation() *huma.Operation {
	return c.op
}

func (c *fiberCtx) Matched() string {
	return c.orig.Route().Path
}

func (c *fiberCtx) Context() context.Context {
	return c.orig.Context()
}

func (c *fiberCtx) Method() string {
	return c.orig.Method()
}

func (c *fiberCtx) Host() string {
	return c.orig.Hostname()
}

func (c *fiberCtx) URL() url.URL {
	u, _ := url.Parse(string(c.orig.Request().RequestURI()))
	return *u
}

func (c *fiberCtx) Param(name string) string {
	return c.orig.Params(name)
}

func (c *fiberCtx) Query(name string) string {
	return c.orig.Query(name)
}

func (c *fiberCtx) Header(name string) string {
	return c.orig.Get(name)
}

func (c *fiberCtx) EachHeader(cb func(name, value string)) {
	c.orig.Request().Header.VisitAll(func(k, v []byte) {
		cb(string(k), string(v))
	})
}

func (c *fiberCtx) BodyReader() io.Reader {
	if c.orig.App().Server().StreamRequestBody {
		// Streaming is enabled, so send the reader.
		return c.orig.Request().BodyStream()
	}
	return bytes.NewReader(c.orig.BodyRaw())
}

func (c *fiberCtx) GetMultipartForm() (*multipart.Form, error) {
	return c.orig.MultipartForm()
}

func (c *fiberCtx) SetReadDeadline(deadline time.Time) error {
	// Note: for this to work properly you need to do two things:
	// 1. Set the Fiber app's `StreamRequestBody` to `true`
	// 2. Set the Fiber app's `BodyLimit` to some small value like `1`
	// Fiber will only call the request handler for streaming once the limit is
	// reached. This is annoying but currently how things work.
	return c.orig.Context().Conn().SetReadDeadline(deadline)
}

func (c *fiberCtx) SetStatus(code int) {
	c.orig.Status(code)
}

func (c *fiberCtx) AppendHeader(name string, value string) {
	c.orig.Append(name, value)
}

func (c *fiberCtx) SetHeader(name string, value string) {
	c.orig.Set(name, value)
}

func (c *fiberCtx) BodyWriter() io.Writer {
	return c.orig
}

type router interface {
	Add(method, path string, handlers ...fiber.Handler) fiber.Router
}

type requestTester interface {
	Test(*http.Request, ...int) (*http.Response, error)
}

type fiberAdapter struct {
	tester requestTester
	router router
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
	resp, err := a.tester.Test(r)
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
	return huma.NewAPI(config, &fiberAdapter{tester: r, router: r})
}

func NewWithGroup(r *fiber.App, g fiber.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &fiberAdapter{tester: r, router: g})
}
