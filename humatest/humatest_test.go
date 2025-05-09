package humatest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ExamplePrintRequest() {
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/foo?bar=baz", nil)
	req.Header.Set("Foo", "bar")
	req.Host = "example.com"
	PrintRequest(req)
	// Output: GET /foo?bar=baz HTTP/1.1
	// Host: example.com
	// Foo: bar
}

func ExamplePrintResponse() {
	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		ContentLength: -1,
		Body:          io.NopCloser(strings.NewReader(`{"foo": "bar"}`)),
	}
	PrintResponse(resp)
	// Output: HTTP/1.1 200 OK
	// Connection: close
	// Content-Type: application/json
	//
	// {
	//   "foo": "bar"
	// }
}

type Response struct {
	MyHeader string `header:"My-Header"`
	Body     struct {
		Echo string `json:"echo"`
	}
}

func TestHumaTestUtils(t *testing.T) {
	_, api := New(t)

	huma.Register(api, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPut,
		Path:        "/test/{id}",
	}, func(ctx context.Context, input *struct {
		ID          string `path:"id"`
		Q           string `query:"q"`
		ContentType string `header:"Content-Type"`
		Body        struct {
			Value string `json:"value"`
		}
	}) (*Response, error) {
		assert.Equal(t, "abc123", input.ID)
		assert.Equal(t, "foo", input.Q)
		assert.Equal(t, "application/json", input.ContentType)
		assert.Equal(t, "hello", input.Body.Value)
		resp := &Response{}
		resp.MyHeader = "my-value"
		resp.Body.Echo = input.Body.Value
		return resp, nil
	})

	w := api.Put("/test/abc123?q=foo",
		"Content-Type: application/json",
		"Host: example.com",
		strings.NewReader(`{"value": "hello"}`))

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "my-value", w.Header().Get("My-Header"))
	assert.JSONEq(t, `{"echo":"hello"}`, w.Body.String())

	// We can also serialize a slice/map/struct and the content type is set
	// automatically for us.
	w = api.Put("/test/abc123?q=foo",
		map[string]any{"value": "hello"})

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "my-value", w.Header().Get("My-Header"))
	assert.JSONEq(t, `{"echo":"hello"}`, w.Body.String())

	assert.Panics(t, func() {
		// Cannot JSON encode a function.
		api.Put("/test/abc123?q=foo",
			"Content-Type: application/json",
			map[string]any{"value": func() {}})
	})
}

func TestContext(t *testing.T) {
	op := &huma.Operation{}
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Foo", "foo")
	r.Header.Set("Bar", "bar")
	w := httptest.NewRecorder()
	ctx := NewContext(op, r, w)

	assert.Equal(t, op, ctx.Operation())
	assert.Equal(t, http.MethodGet, ctx.Method())
	assert.Equal(t, "/", ctx.URL().Path)

	headers := map[string]string{}
	ctx.EachHeader(func(name, value string) {
		headers[name] = value
	})
	assert.Equal(t, map[string]string{
		"Foo": "foo",
		"Bar": "bar",
	}, headers)

	ctx.AppendHeader("Baz", "baz")
	assert.Equal(t, "baz", w.Header().Get("Baz"))
}

func TestAdapter(t *testing.T) {
	var _ huma.Adapter = NewAdapter() //nolint:staticcheck
}

func TestNewAPI(t *testing.T) {
	var api huma.API
	_, api = New(t, huma.DefaultConfig("Test", "1.0.0"))

	// Should be able to wrap and call utility methods.
	wrapped := Wrap(t, api)
	wrapped.Get("/")
	wrapped.Post("/")
	wrapped.Put("/")
	wrapped.Patch("/")
	wrapped.Delete("/")

	assert.Panics(t, func() {
		// Invalid param type (only string headers and io.Reader bodies are allowed)
		wrapped.Post("/", 1234)
	})
}

func TestDumpBodyError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/foo?bar=baz", nil)
	req.Header.Set("Foo", "bar")
	req.Host = "example.com"
	req.Body = io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF))

	// Error should return.
	_, err := DumpRequest(req)
	require.Error(t, err)

	// Error should be passed through.
	_, err = io.ReadAll(req.Body)
	require.Error(t, err)
}

func TestDumpBodyInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/foo?bar=baz", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Host = "example.com"
	req.Body = io.NopCloser(strings.NewReader("invalid json"))

	b, err := DumpRequest(req)
	require.NoError(t, err)
	assert.Contains(t, string(b), "invalid json")
}

func TestOpenAPIRequired(t *testing.T) {
	assert.PanicsWithValue(t, "custom huma.Config structs must specify a value for OpenAPI", func() {
		New(t, huma.Config{})
	})
}
