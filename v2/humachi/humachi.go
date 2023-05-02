package humachi

import (
	"context"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi"
)

type chiAdapter struct {
	router chi.Router
}

func (a *chiAdapter) Handle(method, path string, handler func(http.ResponseWriter, *http.Request)) {
	a.router.MethodFunc(method, path, handler)
}

func (a *chiAdapter) GetMatched(r *http.Request) string {
	return chi.RouteContext(r.Context()).RoutePattern()
}

func (a *chiAdapter) GetContext(r *http.Request) context.Context {
	return r.Context()
}

func (a *chiAdapter) GetParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}

func (a *chiAdapter) GetQuery(r *http.Request, name string) string {
	// TODO: figure out some way to not parse the query params each time...
	return r.URL.Query().Get(name)
}

func (a *chiAdapter) GetHeader(r *http.Request, name string) string {
	return r.Header.Get(name)
}

func (a *chiAdapter) GetBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(r.Body)
}

func (a *chiAdapter) WriteStatus(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
}

func (a *chiAdapter) AppendHeader(w http.ResponseWriter, name string, value string) {
	w.Header().Add(name, value)
}

func (a *chiAdapter) WriteHeader(w http.ResponseWriter, name string, value string) {
	w.Header().Set(name, value)
}

func (a *chiAdapter) BodyWriter(w http.ResponseWriter) io.Writer {
	return w
}

func NewChi(r chi.Router, config huma.Config) huma.StdAPI {
	return huma.NewAPI[*http.Request, http.ResponseWriter](config, &chiAdapter{router: r})
}
