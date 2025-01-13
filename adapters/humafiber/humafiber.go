package humafiber

import (
	"bytes"
	"context"
	"crypto/tls"
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
	op     *huma.Operation
	status int

	/*
	 * Web framework "fiber" https://gofiber.io/ uses high-performance zero-allocation "fasthttp" server https://github.com/valyala/fasthttp
	 *
	 * The underlying fasthttp server prohibits to use or refer to `*fasthttp.RequestCtx` outside handler
	 * The quote from documentation to fasthttp https://github.com/valyala/fasthttp/blob/master/README.md
	 *
	 * > VERY IMPORTANT! Fasthttp disallows holding references to RequestCtx or to its' members after returning from RequestHandler. Otherwise data races are inevitable. Carefully inspect all the net/http request handlers converted to fasthttp whether they retain references to RequestCtx or to its' members after returning
	 *
	 * As the result "fiber" prohibits to use or refer to `*fiber.Ctx` outside handler
	 * The quote from documentation to fiber https://docs.gofiber.io/#zero-allocation
	 *
	 * > Because fiber is optimized for high-performance, values returned from fiber.Ctx are not immutable by default and will be re-used across requests. As a rule of thumb, you must only use context values within the handler, and you must not keep any references. As soon as you return from the handler, any values you have obtained from the context will be re-used in future requests and will change below your feet
	 *
	 * To deal with these limitations, the contributor of to this adapter @excavador (Oleg Tsarev, email: oleg@tsarev.id, telegram: @oleg_tsarev) is clear variable explicitly in the end of huma.Adapter methods Handle and ServeHTTP
	 *
	 * You must NOT use member `unsafeFiberCtx` directly in adapter, but instead use `orig()` private method
	 */
	unsafeFiberCtx  *fiber.Ctx
	unsafeGolangCtx context.Context
}

// check that fiberCtx implements huma.Context
var _ huma.Context = &fiberCtx{}
var _ context.Context = &fiberCtx{}

func (c *fiberCtx) orig() *fiber.Ctx {
	var result = c.unsafeFiberCtx
	select {
	case <-c.unsafeGolangCtx.Done():
		panic("handler was done already")
	default:
		return result
	}
}

func (c *fiberCtx) Deadline() (deadline time.Time, ok bool) {
	return c.unsafeGolangCtx.Deadline()
}

func (c *fiberCtx) Done() <-chan struct{} {
	return c.unsafeGolangCtx.Done()
}

func (c *fiberCtx) Err() error {
	return c.unsafeGolangCtx.Err()
}

func (c *fiberCtx) Value(key any) any {
	var orig = c.unsafeFiberCtx
	select {
	case <-c.unsafeGolangCtx.Done():
		return nil
	default:
		var value = orig.UserContext().Value(key)
		if value != nil {
			return value
		}
		return orig.Context().Value(key)
	}
}

func (c *fiberCtx) Operation() *huma.Operation {
	return c.op
}

func (c *fiberCtx) Matched() string {
	return c.orig().Route().Path
}

func (c *fiberCtx) Context() context.Context {
	return c
}

func (c *fiberCtx) Method() string {
	return c.orig().Method()
}

func (c *fiberCtx) Host() string {
	return c.orig().Hostname()
}

func (c *fiberCtx) RemoteAddr() string {
	return c.orig().Context().RemoteAddr().String()
}

func (c *fiberCtx) URL() url.URL {
	u, _ := url.Parse(string(c.orig().Request().RequestURI()))
	return *u
}

func (c *fiberCtx) Param(name string) string {
	return c.orig().Params(name)
}

func (c *fiberCtx) Query(name string) string {
	return c.orig().Query(name)
}

func (c *fiberCtx) Header(name string) string {
	return c.orig().Get(name)
}

func (c *fiberCtx) EachHeader(cb func(name, value string)) {
	c.orig().Request().Header.VisitAll(func(k, v []byte) {
		cb(string(k), string(v))
	})
}

func (c *fiberCtx) BodyReader() io.Reader {
	var orig = c.orig()
	if orig.App().Server().StreamRequestBody {
		// Streaming is enabled, so send the reader.
		return orig.Request().BodyStream()
	}
	return bytes.NewReader(orig.BodyRaw())
}

func (c *fiberCtx) GetMultipartForm() (*multipart.Form, error) {
	return c.orig().MultipartForm()
}

func (c *fiberCtx) SetReadDeadline(deadline time.Time) error {
	// Note: for this to work properly you need to do two things:
	// 1. Set the Fiber app's `StreamRequestBody` to `true`
	// 2. Set the Fiber app's `BodyLimit` to some small value like `1`
	// Fiber will only call the request handler for streaming once the limit is
	// reached. This is annoying but currently how things work.
	return c.orig().Context().Conn().SetReadDeadline(deadline)
}

func (c *fiberCtx) SetStatus(code int) {
	var orig = c.orig()
	c.status = code
	orig.Status(code)
}

func (c *fiberCtx) Status() int {
	return c.status
}
func (c *fiberCtx) AppendHeader(name string, value string) {
	c.orig().Append(name, value)
}

func (c *fiberCtx) SetHeader(name string, value string) {
	c.orig().Set(name, value)
}

func (c *fiberCtx) BodyWriter() io.Writer {
	return c.orig().Context()
}

func (c *fiberCtx) TLS() *tls.ConnectionState {
	return c.orig().Context().TLSConnectionState()
}

func (c *fiberCtx) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto: c.orig().Protocol(),
	}
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
		var ctx, cancel = context.WithCancel(context.Background())
		var fc = &fiberCtx{
			op:              op,
			unsafeFiberCtx:  c,
			unsafeGolangCtx: ctx,
		}
		defer func() {
			cancel()
			fc.unsafeFiberCtx = nil
		}()
		handler(fc)
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
