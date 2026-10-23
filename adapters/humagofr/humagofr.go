// Package humagofr provides a Huma adapter for GoFr.
package humagofr

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	gofr "gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"
)

// MultipartMaxMemory is the maximum memory to use when parsing multipart form
// data.
var MultipartMaxMemory int64 = 8 * 1024

// Router describes the public GoFr routing API used by the adapter.
type Router interface {
	GET(string, gofr.Handler)
	PUT(string, gofr.Handler)
	POST(string, gofr.Handler)
	DELETE(string, gofr.Handler)
	PATCH(string, gofr.Handler)
	UseMiddleware(...gofrHTTP.Middleware)
}

type bridgeKey struct{}

type bridge struct {
	request *http.Request
	writer  *responseWriter
}

// responseWriter lets Huma write the response directly, then discards the
// response GoFr normally emits after a handler returns.
type responseWriter struct {
	http.ResponseWriter
	sealed       bool
	sealedHeader http.Header
}

func (w *responseWriter) Header() http.Header {
	if w.sealed {
		return w.sealedHeader
	}
	return w.ResponseWriter.Header()
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if !w.sealed {
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if w.sealed {
		return len(data), nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *responseWriter) Flush() {
	if !w.sealed {
		_ = http.NewResponseController(w.ResponseWriter).Flush()
	}
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseWriter) seal() {
	w.sealedHeader = w.ResponseWriter.Header().Clone()
	w.sealed = true
}

type requestWithContext struct {
	gofr.Request
	ctx context.Context
}

func (r *requestWithContext) Context() context.Context {
	return r.ctx
}

type serveHTTPRequest struct {
	*gofrHTTP.Request
	r *http.Request
}

func (r *serveHTTPRequest) Context() context.Context {
	return r.r.Context()
}

func (r *serveHTTPRequest) PathParam(name string) string {
	return r.r.PathValue(name)
}

type gofrContext struct {
	op     *huma.Operation
	orig   *gofr.Context
	r      *http.Request
	w      http.ResponseWriter
	status *int
}

var _ huma.Context = &gofrContext{}

// Unwrap extracts the underlying GoFr context from a Huma context. If passed a
// context from a different adapter it will panic.
func Unwrap(ctx huma.Context) *gofr.Context {
	for {
		if c, ok := ctx.(interface{ Unwrap() huma.Context }); ok {
			ctx = c.Unwrap()
			continue
		}
		break
	}
	if c, ok := ctx.(*gofrContext); ok {
		return c.orig
	}
	panic("not a humagofr context")
}

func (c *gofrContext) Operation() *huma.Operation {
	return c.op
}

func (c *gofrContext) Context() context.Context {
	return c.r.Context()
}

func (c *gofrContext) Method() string {
	return c.r.Method
}

func (c *gofrContext) Host() string {
	return c.r.Host
}

func (c *gofrContext) RemoteAddr() string {
	return c.r.RemoteAddr
}

func (c *gofrContext) URL() url.URL {
	return *c.r.URL
}

func (c *gofrContext) Param(name string) string {
	return c.orig.PathParam(name)
}

func (c *gofrContext) Query(name string) string {
	return c.r.URL.Query().Get(name)
}

func (c *gofrContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *gofrContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *gofrContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *gofrContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(MultipartMaxMemory)
	return c.r.MultipartForm, err
}

func (c *gofrContext) SetReadDeadline(deadline time.Time) error {
	return huma.SetReadDeadline(c.w, deadline)
}

func (c *gofrContext) SetStatus(code int) {
	*c.status = code
	c.w.WriteHeader(code)
}

func (c *gofrContext) Status() int {
	return *c.status
}

func (c *gofrContext) AppendHeader(name, value string) {
	c.w.Header().Add(name, value)
}

func (c *gofrContext) SetHeader(name, value string) {
	c.w.Header().Set(name, value)
}

func (c *gofrContext) BodyWriter() io.Writer {
	return c.w
}

func (c *gofrContext) TLS() *tls.ConnectionState {
	return c.r.TLS
}

func (c *gofrContext) Version() huma.ProtoVersion {
	return huma.ProtoVersion{
		Proto:      c.r.Proto,
		ProtoMajor: c.r.ProtoMajor,
		ProtoMinor: c.r.ProtoMinor,
	}
}

func (c *gofrContext) WithContext(ctx context.Context) huma.Context {
	orig := *c.orig
	orig.Context = ctx
	orig.Request = &requestWithContext{Request: c.orig.Request, ctx: ctx}
	return &gofrContext{
		op:     c.op,
		orig:   &orig,
		r:      c.r.WithContext(ctx),
		w:      c.w,
		status: c.status,
	}
}

func newContext(op *huma.Operation, orig *gofr.Context, r *http.Request, w http.ResponseWriter) huma.Context {
	return &gofrContext{op: op, orig: orig, r: r, w: w, status: new(int)}
}

type gofrAdapter struct {
	router Router
	mux    *http.ServeMux
}

func (a *gofrAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.mux.HandleFunc(strings.ToUpper(op.Method)+" "+op.Path, func(w http.ResponseWriter, r *http.Request) {
		request := &serveHTTPRequest{Request: gofrHTTP.NewRequest(r), r: r}
		ctx := &gofr.Context{Context: r.Context(), Request: request}
		handler(newContext(op, ctx, r, w))
	})

	gofrHandler := func(ctx *gofr.Context) (any, error) {
		state, ok := ctx.Value(bridgeKey{}).(*bridge)
		if !ok {
			return nil, errors.New("humagofr: raw HTTP context is unavailable")
		}

		r := state.request.WithContext(ctx.Context)
		handler(newContext(op, ctx, r, state.writer))
		state.writer.seal()

		return nil, nil
	}

	// ponytail: GoFr only exposes these five registration methods. Replace this
	// switch with generic registration if GoFr adds a public equivalent.
	switch strings.ToUpper(op.Method) {
	case http.MethodGet:
		a.router.GET(op.Path, gofrHandler)
	case http.MethodPut:
		a.router.PUT(op.Path, gofrHandler)
	case http.MethodPost:
		a.router.POST(op.Path, gofrHandler)
	case http.MethodDelete:
		a.router.DELETE(op.Path, gofrHandler)
	case http.MethodPatch:
		a.router.PATCH(op.Path, gofrHandler)
	default:
		panic(fmt.Sprintf("humagofr: GoFr does not support %s route registration", op.Method))
	}
}

func (a *gofrAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

// New creates a new Huma API using a GoFr application. GoFr middleware that
// wraps the response writer should be registered before calling New. GoFr only
// exposes route registration for GET, PUT, POST, DELETE, and PATCH; registering
// an operation using another method will panic.
//
//	app := gofr.New()
//	api := humagofr.New(app, huma.DefaultConfig("My API", "1.0.0"))
func New(app Router, config huma.Config) huma.API {
	adapter := &gofrAdapter{router: app, mux: http.NewServeMux()}
	app.UseMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writer := &responseWriter{ResponseWriter: w}
			state := &bridge{writer: writer}
			r = r.WithContext(context.WithValue(r.Context(), bridgeKey{}, state))
			state.request = r
			next.ServeHTTP(writer, r)
		})
	})
	return huma.NewAPI(config, adapter)
}
