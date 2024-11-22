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

// avoid race condition inside fasthttp need to cache Context().Done() and UserContext().Value
type contextAdapter struct {
	*fiber.Ctx
	done <-chan struct{}
	user func(any) any
}

var _ context.Context = &contextAdapter{}

func (ca *contextAdapter) Deadline() (deadline time.Time, ok bool) {
	return ca.Ctx.Context().Deadline()
}

func (ca *contextAdapter) Done() <-chan struct{} {
	return ca.done
}

func (ca *contextAdapter) Err() error {
	return ca.Ctx.Context().Err()
}

func (ca *contextAdapter) Value(key any) any {
	var value = ca.user(key)
	if value != nil {
		return value
	}
	return ca.Ctx.Context().Value(key)
}

type fiberCtx struct {
	op     *huma.Operation
	orig   *fiber.Ctx
	status int
}

// check that fiberCtx implements huma.Context
var _ huma.Context = &fiberCtx{}

func (c *fiberCtx) Operation() *huma.Operation {
	return c.op
}

func (c *fiberCtx) Matched() string {
	return c.orig.Route().Path
}

func (c *fiberCtx) Context() context.Context {
	return &contextAdapter{
		Ctx:  c.orig,
		done: c.orig.Context().Done(),
		user: c.orig.UserContext().Value,
	}
}

func (c *fiberCtx) Method() string {
	return c.orig.Method()
}

func (c *fiberCtx) Host() string {
	return c.orig.Hostname()
}

func (c *fiberCtx) RemoteAddr() string {
	return c.orig.Context().RemoteAddr().String()
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
	c.status = code
	c.orig.Status(code)
}

func (c *fiberCtx) Status() int {
	return c.status
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

func (c *fiberCtx) TLS() *tls.ConnectionState {
	return c.orig.Context().TLSConnectionState()
}

func (c *fiberCtx) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto: c.orig.Protocol(),
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
