// Package humatest provides testing utilities for Huma services. It is based
// on the `chi` router and the standard library `http.Request` &
// `http.ResponseWriter` types.
package humatest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// TB is a subset of the `testing.TB` interface used by the test API and
// implemented by the `*testing.T` and `*testing.B` structs.
type TB interface {
	Helper()
	Log(args ...any)
	Logf(format string, args ...any)
}

// NewContext creates a new test context from an HTTP request and response.
func NewContext(op *huma.Operation, r *http.Request, w http.ResponseWriter) huma.Context {
	return humachi.NewContext(op, r, w)
}

// NewAdapter creates a new test adapter from a chi router.
func NewAdapter(r chi.Router) huma.Adapter {
	return humachi.NewAdapter(r)
}

// TestAPI is a `huma.API` with additional methods specifically for testing.
type TestAPI interface {
	huma.API

	// Do a request against the API. Args, if provided, should be string headers
	// like `Content-Type: application/json`, an `io.Reader` for the request
	// body, or a slice/map/struct which will be serialized to JSON and sent
	// as the request body. Anything else will panic.
	Do(method, path string, args ...any) *httptest.ResponseRecorder

	// Get performs a GET request against the API. Args, if provided, should be
	// string headers like `Content-Type: application/json`, an `io.Reader`
	// for the request body, or a slice/map/struct which will be serialized to
	// JSON and sent as the request body. Anything else will panic.
	//
	// 	// Make a GET request
	// 	api.Get("/foo")
	//
	// 	// Make a GET request with a custom header.
	// 	api.Get("/foo", "X-My-Header: my-value")
	Get(path string, args ...any) *httptest.ResponseRecorder

	// Post performs a POST request against the API. Args, if provided, should be
	// string headers like `Content-Type: application/json`, an `io.Reader`
	// for the request body, or a slice/map/struct which will be serialized to
	// JSON and sent as the request body. Anything else will panic.
	//
	// 	// Make a POST request
	// 	api.Post("/foo", bytes.NewReader(`{"foo": "bar"}`))
	//
	// 	// Make a POST request with a custom header.
	// 	api.Post("/foo", "X-My-Header: my-value", MyBody{Foo: "bar"})
	Post(path string, args ...any) *httptest.ResponseRecorder

	// Put performs a PUT request against the API. Args, if provided, should be
	// string headers like `Content-Type: application/json`, an `io.Reader`
	// for the request body, or a slice/map/struct which will be serialized to
	// JSON and sent as the request body. Anything else will panic.
	//
	// 	// Make a PUT request
	// 	api.Put("/foo", bytes.NewReader(`{"foo": "bar"}`))
	//
	// 	// Make a PUT request with a custom header.
	// 	api.Put("/foo", "X-My-Header: my-value", MyBody{Foo: "bar"})
	Put(path string, args ...any) *httptest.ResponseRecorder

	// Patch performs a PATCH request against the API. Args, if provided, should
	// be string headers like `Content-Type: application/json`, an `io.Reader`
	// for the request body, or a slice/map/struct which will be serialized to
	// JSON and sent as the request body. Anything else will panic.
	//
	// 	// Make a PATCH request
	// 	api.Patch("/foo", bytes.NewReader(`{"foo": "bar"}`))
	//
	// 	// Make a PATCH request with a custom header.
	// 	api.Patch("/foo", "X-My-Header: my-value", MyBody{Foo: "bar"})
	Patch(path string, args ...any) *httptest.ResponseRecorder

	// Delete performs a DELETE request against the API. Args, if provided, should
	// be string headers like `Content-Type: application/json`, an `io.Reader`
	// for the request body, or a slice/map/struct which will be serialized to
	// JSON and sent as the request body. Anything else will panic.
	//
	// 	// Make a DELETE request
	// 	api.Delete("/foo")
	//
	// 	// Make a DELETE request with a custom header.
	// 	api.Delete("/foo", "X-My-Header: my-value")
	Delete(path string, args ...any) *httptest.ResponseRecorder
}

type testAPI struct {
	huma.API
	tb TB
}

func (a *testAPI) Do(method, path string, args ...any) *httptest.ResponseRecorder {
	a.tb.Helper()
	var b io.Reader
	isJSON := false
	for _, arg := range args {
		kind := reflect.Indirect(reflect.ValueOf(arg)).Kind()
		if reader, ok := arg.(io.Reader); ok {
			b = reader
			break
		} else if _, ok := arg.(string); ok {
			// do nothing
		} else if kind == reflect.Struct || kind == reflect.Map || kind == reflect.Slice {
			encoded, err := json.Marshal(arg)
			if err != nil {
				panic(err)
			}
			b = bytes.NewReader(encoded)
			isJSON = true
		} else {
			panic("unsupported argument type, expected string header or io.Reader/slice/map/struct body")
		}
	}

	req, _ := http.NewRequest(method, path, b)
	if isJSON {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, arg := range args {
		if s, ok := arg.(string); ok {
			parts := strings.Split(s, ":")
			req.Header.Set(parts[0], strings.TrimSpace(strings.Join(parts[1:], ":")))

			if strings.ToLower(parts[0]) == "host" {
				req.Host = strings.TrimSpace(parts[1])
			}
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
	return Wrap(tb, humachi.New(r, config))
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
				"json":             huma.DefaultJSONFormat,
			},
			DefaultFormat: "application/json",
		})
	}
	r := chi.NewRouter()
	return r, NewTestAPI(tb, r, configs[0])
}
