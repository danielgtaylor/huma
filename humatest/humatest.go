// Package humatest provides testing utilities for Huma services. It is based
// on the `chi` router and the standard library `http.Request` &
// `http.ResponseWriter` types.
package humatest

import (
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/go-chi/chi"
)

// TB is a subset of the `testing.TB` interface used by the test API and
// implemented by the `*testing.T` and `*testing.B` structs.
type TB interface {
	Helper()
	Log(args ...any)
	Logf(format string, args ...any)
}

type testContext struct {
	op *huma.Operation
	r  *http.Request
	w  http.ResponseWriter
}

// NewContext creates a new test context from a request/response pair.
func NewContext(op *huma.Operation, r *http.Request, w http.ResponseWriter) huma.Context {
	return &testContext{op, r, w}
}

func (c *testContext) Operation() *huma.Operation {
	return c.op
}

func (c *testContext) Context() context.Context {
	return c.r.Context()
}

func (c *testContext) Method() string {
	return c.r.Method
}

func (c *testContext) Host() string {
	return c.r.Host
}

func (c *testContext) URL() url.URL {
	return *c.r.URL
}

func (c *testContext) Param(name string) string {
	return chi.URLParam(c.r, name)
}

func (c *testContext) Query(name string) string {
	return queryparam.Get(c.r.URL.RawQuery, name)
}

func (c *testContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *testContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *testContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *testContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(8 * 1024)
	return c.r.MultipartForm, err
}

func (c *testContext) SetReadDeadline(deadline time.Time) error {
	return http.NewResponseController(c.w).SetReadDeadline(deadline)
}

func (c *testContext) SetStatus(code int) {
	c.w.WriteHeader(code)
}

func (c *testContext) AppendHeader(name string, value string) {
	c.w.Header().Add(name, value)
}

func (c *testContext) SetHeader(name string, value string) {
	c.w.Header().Set(name, value)
}

func (c *testContext) BodyWriter() io.Writer {
	return c.w
}

type testAdapter struct {
	router chi.Router
}

// NewAdapter creates a new adapter for the given chi router.
func NewAdapter(r chi.Router) huma.Adapter {
	return &testAdapter{router: r}
}

func (a *testAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.router.MethodFunc(op.Method, op.Path, func(w http.ResponseWriter, r *http.Request) {
		handler(&testContext{op: op, r: r, w: w})
	})
}

func (a *testAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

// TestAPI is a `huma.API` with additional methods specifically for testing.
type TestAPI interface {
	huma.API

	// Do a request against the API. Args, if provided, should be string headers
	// like `Content-Type: application/json` or an `io.Reader` for the request
	// body. Anything else will panic.
	Do(method, path string, args ...any) *httptest.ResponseRecorder

	// Get performs a GET request against the API. Args, if provided, should be
	// string headers like `Content-Type: application/json` or an `io.Reader`
	// for the request body. Anything else will panic.
	Get(path string, args ...any) *httptest.ResponseRecorder

	// Post performs a POST request against the API. Args, if provided, should be
	// string headers like `Content-Type: application/json` or an `io.Reader`
	// for the request body. Anything else will panic.
	Post(path string, args ...any) *httptest.ResponseRecorder

	// Put performs a PUT request against the API. Args, if provided, should be
	// string headers like `Content-Type: application/json` or an `io.Reader`
	// for the request body. Anything else will panic.
	Put(path string, args ...any) *httptest.ResponseRecorder

	// Patch performs a PATCH request against the API. Args, if provided, should
	// be string headers like `Content-Type: application/json` or an `io.Reader`
	// for the request body. Anything else will panic.
	Patch(path string, args ...any) *httptest.ResponseRecorder

	// Delete performs a DELETE request against the API. Args, if provided, should
	// be string headers like `Content-Type: application/json` or an `io.Reader`
	// for the request body. Anything else will panic.
	Delete(path string, args ...any) *httptest.ResponseRecorder
}

type testAPI struct {
	huma.API
	tb TB
}

func (a *testAPI) Do(method, path string, args ...any) *httptest.ResponseRecorder {
	a.tb.Helper()
	var b io.Reader
	for _, arg := range args {
		if reader, ok := arg.(io.Reader); ok {
			b = reader
			break
		} else if _, ok := arg.(string); ok {
			// do nothing
		} else {
			panic("unsupported argument type, expected string header or io.Reader body")
		}
	}

	req, _ := http.NewRequest(method, path, b)
	for _, arg := range args {
		if s, ok := arg.(string); ok {
			parts := strings.Split(s, ":")
			req.Header.Set(parts[0], strings.TrimSpace(strings.Join(parts[1:], ":")))
		}
	}
	resp := httptest.NewRecorder()

	bytes, _ := httputil.DumpRequest(req, b != nil)
	a.tb.Log("Making request:\n" + strings.TrimSpace(string(bytes)))

	a.Adapter().ServeHTTP(resp, req)

	bytes, _ = httputil.DumpResponse(resp.Result(), resp.Body.Len() > 0)
	a.tb.Log("Got response:\n" + strings.TrimSpace(string(bytes)))

	return resp
}

func (a *testAPI) Get(path string, args ...any) *httptest.ResponseRecorder {
	a.tb.Helper()
	return a.Do(http.MethodGet, path, args...)
}

func (a *testAPI) Post(path string, args ...any) *httptest.ResponseRecorder {
	a.tb.Helper()
	return a.Do(http.MethodPost, path, args...)
}

func (a *testAPI) Put(path string, args ...any) *httptest.ResponseRecorder {
	a.tb.Helper()
	return a.Do(http.MethodPut, path, args...)
}

func (a *testAPI) Patch(path string, args ...any) *httptest.ResponseRecorder {
	a.tb.Helper()
	return a.Do(http.MethodPatch, path, args...)
}

func (a *testAPI) Delete(path string, args ...any) *httptest.ResponseRecorder {
	a.tb.Helper()
	return a.Do(http.MethodDelete, path, args...)
}

// NewTestAPI creates a new test API from a chi router and API config.
func NewTestAPI(tb TB, r chi.Router, config huma.Config) TestAPI {
	api := huma.NewAPI(config, &testAdapter{router: r})
	return &testAPI{api, tb}
}

// Wrap returns a `TestAPI` wrapping the given API.
func Wrap(tb TB, api huma.API) TestAPI {
	return &testAPI{api, tb}
}

// New creates a new router and test API, making it easy to register operations
// and perform requests against them. Optionally takes a configuration object
// to customize how the API is created. If no configuration is provided then
// a simple default configuration supporting `application/json` is used.
func New(tb TB, configs ...huma.Config) (chi.Router, TestAPI) {
	if len(configs) == 0 {
		configs = append(configs, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{
					Title:   "Test API",
					Version: "1.0.0",
				},
			},
			Formats: map[string]huma.Format{
				"application/json": huma.DefaultJSONFormat,
			},
			DefaultFormat: "application/json",
		})
	}
	r := chi.NewRouter()
	return r, NewTestAPI(tb, r, configs[0])
}
