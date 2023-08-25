package humahttprouter

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/julienschmidt/httprouter"
)

type httprouterContext struct {
	op *huma.Operation
	r  *http.Request
	w  http.ResponseWriter
	ps httprouter.Params
}

func (ctx *httprouterContext) Operation() *huma.Operation {
	return ctx.op
}

func (ctx *httprouterContext) Context() context.Context {
	return ctx.r.Context()
}

func (ctx *httprouterContext) Method() string {
	return ctx.r.Method
}

func (ctx *httprouterContext) Host() string {
	return ctx.r.Host
}

func (ctx *httprouterContext) URL() url.URL {
	return *ctx.r.URL
}

func (ctx *httprouterContext) Param(name string) string {
	return ctx.ps.ByName(name)
}

func (ctx *httprouterContext) Query(name string) string {
	return queryparam.Get(ctx.r.URL.RawQuery, name)
}

func (ctx *httprouterContext) Header(name string) string {
	return ctx.r.Header.Get(name)
}

func (ctx *httprouterContext) EachHeader(cb func(name, value string)) {
	for name, values := range ctx.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (ctx *httprouterContext) BodyReader() io.Reader {
	return ctx.r.Body
}

func (ctx *httprouterContext) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(ctx.w, deadline)
}

func (ctx *httprouterContext) SetStatus(code int) {
	ctx.w.WriteHeader(code)
}

func (ctx *httprouterContext) AppendHeader(name string, value string) {
	ctx.w.Header().Add(name, value)
}

func (ctx *httprouterContext) SetHeader(name string, value string) {
	ctx.w.Header().Set(name, value)
}

func (ctx *httprouterContext) BodyWriter() io.Writer {
	return ctx.w
}

type httprouterAdapter struct {
	router *httprouter.Router
}

func (a *httprouterAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	// Convert {param} to :param
	path := op.Path
	path = strings.ReplaceAll(path, "{", ":")
	path = strings.ReplaceAll(path, "}", "")
	a.router.Handle(op.Method, path, func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		handler(&httprouterContext{op: op, r: r, w: w, ps: ps})
	})
}

func (a *httprouterAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

func New(r *httprouter.Router, config huma.Config) huma.API {
	return huma.NewAPI(config, &httprouterAdapter{router: r})
}
