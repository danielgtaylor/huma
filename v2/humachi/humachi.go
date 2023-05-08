package humachi

import (
	"context"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi"
)

type chiContext struct {
	r *http.Request
	w http.ResponseWriter
}

func (ctx *chiContext) GetMatched() string {
	return chi.RouteContext(ctx.r.Context()).RoutePattern()
}

func (ctx *chiContext) GetContext() context.Context {
	return ctx.r.Context()
}

func (ctx *chiContext) GetParam(name string) string {
	return chi.URLParam(ctx.r, name)
}

func (ctx *chiContext) GetQuery(name string) string {
	// TODO: figure out some way to not parse the query params each time...
	return ctx.r.URL.Query().Get(name)
}

func (ctx *chiContext) GetHeader(name string) string {
	return ctx.r.Header.Get(name)
}

func (ctx *chiContext) GetBody() ([]byte, error) {
	return io.ReadAll(ctx.r.Body)
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

func (a *chiAdapter) Handle(method, path string, handler func(huma.Context)) {
	a.router.MethodFunc(method, path, func(w http.ResponseWriter, r *http.Request) {
		handler(&chiContext{r: r, w: w})
	})
}

func NewChi(r chi.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &chiAdapter{router: r})
}
