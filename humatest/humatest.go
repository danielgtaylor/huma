package humatest

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/go-chi/chi"
)

type testContext struct {
	op *huma.Operation
	r  *http.Request
	w  http.ResponseWriter
}

func NewContext(op *huma.Operation, r *http.Request, w http.ResponseWriter) huma.Context {
	return &testContext{op, r, w}
}

func (ctx *testContext) GetOperation() *huma.Operation {
	return ctx.op
}

func (ctx *testContext) GetMatched() string {
	return chi.RouteContext(ctx.r.Context()).RoutePattern()
}

func (ctx *testContext) GetContext() context.Context {
	return ctx.r.Context()
}

func (ctx *testContext) GetMethod() string {
	return ctx.r.Method
}

func (ctx *testContext) GetURL() url.URL {
	return *ctx.r.URL
}

func (ctx *testContext) GetParam(name string) string {
	return chi.URLParam(ctx.r, name)
}

func (ctx *testContext) GetQuery(name string) string {
	return queryparam.Get(ctx.r.URL.RawQuery, name)
}

func (ctx *testContext) GetHeader(name string) string {
	return ctx.r.Header.Get(name)
}

func (ctx *testContext) EachHeader(cb func(name, value string)) {
	for name, values := range ctx.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (ctx *testContext) GetBody() ([]byte, error) {
	return io.ReadAll(ctx.r.Body)
}

func (ctx *testContext) GetBodyReader() io.Reader {
	return ctx.r.Body
}

func (ctx *testContext) WriteStatus(code int) {
	ctx.w.WriteHeader(code)
}

func (ctx *testContext) AppendHeader(name string, value string) {
	ctx.w.Header().Add(name, value)
}

func (ctx *testContext) WriteHeader(name string, value string) {
	ctx.w.Header().Set(name, value)
}

func (ctx *testContext) BodyWriter() io.Writer {
	return ctx.w
}

type testAdapter struct {
	router chi.Router
}

func (a *testAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.router.MethodFunc(op.Method, op.Path, func(w http.ResponseWriter, r *http.Request) {
		handler(&testContext{op: op, r: r, w: w})
	})
}

func (a *testAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

func NewTestAdapter(r chi.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &testAdapter{router: r})
}

func New() (chi.Router, huma.API) {
	r := chi.NewRouter()
	return r, NewTestAdapter(r, huma.Config{
		OpenAPI: &huma.OpenAPI{
			Info: &huma.Info{
				Title:   "Test API",
				Version: "1.0.0",
			},
		},
	})
}
