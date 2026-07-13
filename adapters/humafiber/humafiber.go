package humafiber

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
)

// Unwrap extracts the underlying Fiber context from a Huma context. If passed a
// context from a different adapter it will panic. Keep in mind the limitations
// of the underlying Fiber/fasthttp libraries and how that impacts
// memory-safety: https://docs.gofiber.io/#zero-allocation. Do not keep
// references to the underlying context or its values!
func Unwrap(ctx huma.Context) fiber.Ctx {
	for {
		if c, ok := ctx.(interface{ Unwrap() huma.Context }); ok {
			ctx = c.Unwrap()
			continue
		}
		break
	}
	if c, ok := ctx.(*fiberWrapper); ok {
		return c.Unwrap()
	}
	panic("not a humafiber context")
}

type fiberAdapter struct {
	tester requestTester
	router router
}

type fiberWrapper struct {
	op     *huma.Operation
	status int
	orig   fiber.Ctx
	ctx    context.Context
}

// check that fiberWrapper implements huma.Context
var _ huma.Context = &fiberWrapper{}

func (c *fiberWrapper) Unwrap() fiber.Ctx {
	return c.orig
}

func (c *fiberWrapper) Operation() *huma.Operation {
	return c.op
}

func (c *fiberWrapper) Matched() string {
	return c.orig.Route().Path
}

func (c *fiberWrapper) Context() context.Context {
	return c.ctx
}

func (c *fiberWrapper) Method() string {
	return c.orig.Method()
}

func (c *fiberWrapper) Host() string {
	return c.orig.Hostname()
}

func (c *fiberWrapper) RemoteAddr() string {
	return c.orig.RequestCtx().RemoteAddr().String()
}

func (c *fiberWrapper) URL() url.URL {
	u, _ := url.Parse(string(c.orig.Request().RequestURI()))
	return *u
}

func (c *fiberWrapper) Param(name string) string {
	return c.orig.Params(name)
}

func (c *fiberWrapper) Query(name string) string {
	return c.orig.Query(name)
}

func (c *fiberWrapper) Header(name string) string {
	return c.orig.Get(name)
}

func (c *fiberWrapper) EachHeader(cb func(name, value string)) {
	c.orig.Request().Header.All()(func(k, v []byte) bool {
		cb(string(k), string(v))
		return true // Keep iterating.
	})
}

func (c *fiberWrapper) BodyReader() io.Reader {
	var orig = c.orig
	if orig.App().Server().StreamRequestBody {
		// Streaming is enabled, so send the reader.
		return orig.Request().BodyStream()
	}
	return bytes.NewReader(orig.Body())
}

func (c *fiberWrapper) GetMultipartForm() (*multipart.Form, error) {
	return c.orig.MultipartForm()
}

func (c *fiberWrapper) SetReadDeadline(deadline time.Time) error {
	// Note: for this to work properly you need to do two things:
	// 1. Set the Fiber app's `StreamRequestBody` to `true`
	// 2. Set the Fiber app's `BodyLimit` to some small value like `1`
	// Fiber will only call the request handler for streaming once the limit is
	// reached. This is annoying but currently how things work.
	return c.orig.RequestCtx().Conn().SetReadDeadline(deadline)
}

func (c *fiberWrapper) SetStatus(code int) {
	var orig = c.orig
	c.status = code
	orig.Status(code)
}

func (c *fiberWrapper) Status() int {
	return c.status
}

func (c *fiberWrapper) AppendHeader(name string, value string) {
	c.orig.Append(name, value)
}

func (c *fiberWrapper) SetHeader(name string, value string) {
	c.orig.Set(name, value)
}

func (c *fiberWrapper) BodyWriter() io.Writer {
	return c.orig.RequestCtx()
}

// StreamBody streams the response body via Fiber/fasthttp's stream writer. It
// is the optional streaming hook huma's SSE support uses because fasthttp can't
// flush the response writer synchronously from within the handler.
func (c *fiberWrapper) StreamBody(fn func(io.Writer)) {
	rc := c.orig.RequestCtx()
	rc.SetBodyStreamWriter(func(bw *bufio.Writer) {
		fn(&fiberStreamWriter{bw: bw, conn: rc.Conn()})
	})
}

func (c *fiberWrapper) TLS() *tls.ConnectionState {
	return c.orig.RequestCtx().TLSConnectionState()
}

func (c *fiberWrapper) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto: c.orig.Protocol(),
	}
}

// WithContext replaces the underlying context. Fiber stores a single context
// per request, so this mutates it in place (rather than returning an isolated
// copy) so that native Fiber middleware observe values set via huma.WithValue.
func (c *fiberWrapper) WithContext(ctx context.Context) huma.Context {
	c.orig.SetContext(ctx)
	return &fiberWrapper{
		op:     c.op,
		status: c.status,
		orig:   c.orig,
		ctx:    ctx,
	}
}

// NewContext creates a new Huma context from a fiber context
func NewContext(op *huma.Operation, c fiber.Ctx) huma.Context {
	return &fiberWrapper{op: op, orig: c, ctx: c.Context()}
}

type router interface {
	Add(methods []string, path string, handler any, handlers ...any) fiber.Router
}

type requestTester interface {
	Test(*http.Request, ...fiber.TestConfig) (*http.Response, error)
}

type contextWrapperValue struct {
	Key   any
	Value any
}

type contextWrapper struct {
	values []*contextWrapperValue
	context.Context
}

var (
	_ context.Context = &contextWrapper{}
)

func (c *contextWrapper) Value(key any) any {
	var raw = c.Context.Value(key)
	if raw != nil {
		return raw
	}
	for _, pair := range c.values {
		if pair.Key == key {
			return pair.Value
		}
	}
	return nil
}

func (a *fiberAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Add([]string{op.Method}, path, func(c fiber.Ctx) error {
		var values []*contextWrapperValue
		c.RequestCtx().VisitUserValuesAll(func(key, value any) {
			values = append(values, &contextWrapperValue{
				Key:   key,
				Value: value,
			})
		})
		handler(&fiberWrapper{
			op:   op,
			orig: c,
			ctx: &contextWrapper{
				values:  values,
				Context: c.Context(),
			},
		})
		return nil
	})
}

func (a *fiberAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp, err := a.tester.Test(r)
	if resp != nil && resp.Body != nil {
		defer func() {
			_ = resp.Body.Close()
		}()
	}
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
	_, _ = io.Copy(w, resp.Body)
}

// New creates a new Huma API using the Fiber adapter.
func New(r *fiber.App, config huma.Config) huma.API {
	return huma.NewAPI(config, &fiberAdapter{tester: r, router: r})
}

// NewWithGroup creates a new Huma API using the Fiber adapter with a route group.
func NewWithGroup(r *fiber.App, g fiber.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &fiberAdapter{tester: r, router: g})
}

// fiberStreamWriter adapts fasthttp's buffered stream writer to the io.Writer,
// http.Flusher, and write-deadline interfaces the streaming code expects. It is
// shared by the Fiber v2 and v3 adapters.
type fiberStreamWriter struct {
	bw   *bufio.Writer
	conn net.Conn
}

func (w *fiberStreamWriter) Write(p []byte) (int, error) {
	return w.bw.Write(p)
}

func (w *fiberStreamWriter) Flush() {
	_ = w.bw.Flush()
}

func (w *fiberStreamWriter) SetWriteDeadline(t time.Time) error {
	if w.conn == nil {
		return nil
	}
	return w.conn.SetWriteDeadline(t)
}
