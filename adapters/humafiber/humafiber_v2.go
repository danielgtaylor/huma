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
	fiberV2 "github.com/gofiber/fiber/v2"
)

// UnwrapV2 extracts the underlying Fiber v2 context from a Huma context. If
// passed a context from a different adapter it will panic. Keep in mind the
// limitations of the underlying Fiber/fasthttp libraries and how that impacts
// memory-safety: https://docs.gofiber.io/#zero-allocation. Do not keep
// references to the underlying context or its values!
func UnwrapV2(ctx huma.Context) *fiberV2.Ctx {
	for {
		if c, ok := ctx.(interface{ Unwrap() huma.Context }); ok {
			ctx = c.Unwrap()
			continue
		}
		break
	}
	if c, ok := ctx.(*fiberV2Wrapper); ok {
		return c.Unwrap()
	}
	panic("not a humafiber v2 context")
}

type fiberV2Adapter struct {
	tester requestTesterV2
	router routerV2
}

type fiberV2Wrapper struct {
	op     *huma.Operation
	status int
	orig   *fiberV2.Ctx
	ctx    context.Context
}

// check that fiberV2Wrapper implements huma.Context
var _ huma.Context = &fiberV2Wrapper{}

func (c *fiberV2Wrapper) Unwrap() *fiberV2.Ctx {
	return c.orig
}

func (c *fiberV2Wrapper) Operation() *huma.Operation {
	return c.op
}

func (c *fiberV2Wrapper) Matched() string {
	return c.orig.Route().Path
}

func (c *fiberV2Wrapper) Context() context.Context {
	return c.ctx
}

func (c *fiberV2Wrapper) Method() string {
	return c.orig.Method()
}

func (c *fiberV2Wrapper) Host() string {
	return c.orig.Hostname()
}

func (c *fiberV2Wrapper) RemoteAddr() string {
	return c.orig.Context().RemoteAddr().String()
}

func (c *fiberV2Wrapper) URL() url.URL {
	u, _ := url.Parse(string(c.orig.Request().RequestURI()))
	return *u
}

func (c *fiberV2Wrapper) Param(name string) string {
	return c.orig.Params(name)
}

func (c *fiberV2Wrapper) Query(name string) string {
	return c.orig.Query(name)
}

func (c *fiberV2Wrapper) Header(name string) string {
	return c.orig.Get(name)
}

func (c *fiberV2Wrapper) EachHeader(cb func(name, value string)) {
	c.orig.Request().Header.VisitAll(func(k, v []byte) {
		cb(string(k), string(v))
	})
}

func (c *fiberV2Wrapper) BodyReader() io.Reader {
	var orig = c.orig
	if orig.App().Server().StreamRequestBody {
		// Streaming is enabled, so send the reader.
		return orig.Request().BodyStream()
	}
	return bytes.NewReader(orig.BodyRaw())
}

func (c *fiberV2Wrapper) GetMultipartForm() (*multipart.Form, error) {
	return c.orig.MultipartForm()
}

func (c *fiberV2Wrapper) SetReadDeadline(deadline time.Time) error {
	// Note: for this to work properly you need to do two things:
	// 1. Set the Fiber app's `StreamRequestBody` to `true`
	// 2. Set the Fiber app's `BodyLimit` to some small value like `1`
	// Fiber will only call the request handler for streaming once the limit is
	// reached. This is annoying but currently how things work.
	return c.orig.Context().Conn().SetReadDeadline(deadline)
}

func (c *fiberV2Wrapper) SetStatus(code int) {
	var orig = c.orig
	c.status = code
	orig.Status(code)
}

func (c *fiberV2Wrapper) Status() int {
	return c.status
}

func (c *fiberV2Wrapper) AppendHeader(name string, value string) {
	c.orig.Append(name, value)
}

func (c *fiberV2Wrapper) SetHeader(name string, value string) {
	c.orig.Set(name, value)
}

func (c *fiberV2Wrapper) BodyWriter() io.Writer {
	return c.orig.Context()
}

func (c *fiberV2Wrapper) TLS() *tls.ConnectionState {
	return c.orig.Context().TLSConnectionState()
}

func (c *fiberV2Wrapper) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto: c.orig.Protocol(),
	}
}

type routerV2 interface {
	Add(method, path string, handlers ...fiberV2.Handler) fiberV2.Router
}

type requestTesterV2 interface {
	Test(*http.Request, ...int) (*http.Response, error)
}

type contextV2WrapperValue struct {
	Key   any
	Value any
}

type contextV2Wrapper struct {
	values []*contextV2WrapperValue
	context.Context
}

var (
	_ context.Context = &contextV2Wrapper{}
)

func (c *contextV2Wrapper) Value(key any) any {
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

func (a *fiberV2Adapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Add(op.Method, path, func(c *fiberV2.Ctx) error {
		var values []*contextV2WrapperValue
		c.Context().VisitUserValuesAll(func(key, value any) {
			values = append(values, &contextV2WrapperValue{
				Key:   key,
				Value: value,
			})
		})
		handler(&fiberV2Wrapper{
			op:   op,
			orig: c,
			ctx: &contextV2Wrapper{
				values:  values,
				Context: c.UserContext(),
			},
		})
		return nil
	})
}

func (a *fiberV2Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

// NewV2 creates a new Huma API using the Fiber v2.x adapter.
func NewV2(r *fiberV2.App, config huma.Config) huma.API {
	return huma.NewAPI(config, &fiberV2Adapter{tester: r, router: r})
}

// NewV2WithGroup creates a new Huma API using the Fiber v2.x adapter with a
// route group.
func NewV2WithGroup(r *fiberV2.App, g fiberV2.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &fiberV2Adapter{tester: r, router: g})
}
