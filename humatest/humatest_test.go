package humatest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi"
	"github.com/stretchr/testify/assert"
)

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
		strings.NewReader(`{"value": "hello"}`))

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "my-value", w.Header().Get("My-Header"))
	assert.JSONEq(t, `{"echo":"hello"}`, w.Body.String())
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
	var _ huma.Adapter = NewAdapter(chi.NewMux())
}

func TestNewAPI(t *testing.T) {
	r := chi.NewMux()
	var api huma.API = NewTestAPI(t, r, huma.DefaultConfig("Test", "1.0.0"))

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
