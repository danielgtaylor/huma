package humachi

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/go-chi/chi"
)

type chiContext struct {
	op *huma.Operation
	r  *http.Request
	w  http.ResponseWriter
}

func (ctx *chiContext) GetOperation() *huma.Operation {
	return ctx.op
}

func (ctx *chiContext) GetMatched() string {
	return chi.RouteContext(ctx.r.Context()).RoutePattern()
}

func (ctx *chiContext) GetContext() context.Context {
	return ctx.r.Context()
}

func (ctx *chiContext) GetMethod() string {
	return ctx.r.Method
}

func (ctx *chiContext) GetURL() url.URL {
	return *ctx.r.URL
}

func (ctx *chiContext) GetParam(name string) string {
	return chi.URLParam(ctx.r, name)
}

func (ctx *chiContext) GetQuery(name string) string {
	return queryparam.Get(ctx.r.URL.RawQuery, name)
}

func (ctx *chiContext) GetHeader(name string) string {
	return ctx.r.Header.Get(name)
}

func (ctx *chiContext) EachHeader(cb func(name, value string)) {
	for name, values := range ctx.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (ctx *chiContext) GetBody() ([]byte, error) {
	return io.ReadAll(ctx.r.Body)
}

func (ctx *chiContext) GetBodyReader() io.Reader {
	return ctx.r.Body
}

func (ctx *chiContext) WriteStatus(code int) {
	ctx.w.WriteHeader(code)
}

func (ctx *chiContext) AppendHeader(name string, value string) {
	ctx.w.Header().Add(name, value)
}

func (ctx *chiContext) WriteHeader(name string, value string) {
	ctx.w.Header().Set(name, value)
}

func (ctx *chiContext) BodyWriter() io.Writer {
	return ctx.w
}

type chiAdapter struct {
	router chi.Router
}

func (a *chiAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.router.MethodFunc(op.Method, op.Path, func(w http.ResponseWriter, r *http.Request) {
		handler(&chiContext{op: op, r: r, w: w})
	})
}

func (a *chiAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

func New(r chi.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &chiAdapter{router: r})
}
