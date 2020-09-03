package huma

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func newTestRouter() *Router {
	app := New("Test API", "1.0.0")
	return app
}

func TestRouterServiceLink(t *testing.T) {
	r := newTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Header().Get("Link"), `</openapi.json>; rel="service-desc"`)
	assert.Contains(t, w.Header().Get("Link"), `</docs>; rel="service-doc"`)
}

func TestRouterHello(t *testing.T) {
	r := New("Test", "1.0.0")
	r.Resource("/test").Get("test", "Test",
		NewResponse(http.StatusNoContent, "test"),
	).Run(func(ctx Context) {
		ctx.WriteHeader(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	// Assert the response is as expected.
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestStreamingInput(t *testing.T) {
	r := New("Test", "1.0.0")
	r.Resource("/stream").Post("stream", "Stream test",
		NewResponse(http.StatusNoContent, "test"),
		NewResponse(http.StatusInternalServerError, "error"),
	).Run(func(ctx Context, input struct {
		Body io.Reader
	}) {
		_, err := ioutil.ReadAll(input.Body)
		if err != nil {
			ctx.WriteError(http.StatusInternalServerError, "Problem reading input", err)
		}

		ctx.WriteHeader(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	body := bytes.NewReader(make([]byte, 1024))
	req, _ := http.NewRequest(http.MethodPost, "/stream", body)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestModelInputOutput(t *testing.T) {
	type Response struct {
		Category string `json:"category"`
		Hidden   bool   `json:"hidden"`
		Auth     string `json:"auth"`
		ID       string `json:"id"`
		Age      int    `json:"age"`
	}

	r := New("Test", "1.0.0")
	r.Resource("/players").SubResource("category").Post("player", "Create player",
		NewResponse(http.StatusOK, "test").Model(Response{}),
	).Run(func(ctx Context, input struct {
		Category string `path:"category"`
		Hidden   bool   `query:"hidden"`
		Auth     string `header:"Authorization"`
		Body     struct {
			ID  string `json:"id"`
			Age int    `json:"age" minimum:"16"`
		}
	}) {
		ctx.WriteModel(http.StatusOK, Response{
			Category: input.Category,
			Hidden:   input.Hidden,
			Auth:     input.Auth,
			ID:       input.Body.ID,
			Age:      input.Body.Age,
		})
	})

	w := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"id": "abc123", "age": 25}`))
	req, _ := http.NewRequest(http.MethodPost, "/players/fps?hidden=true", body)
	req.Header.Set("Authorization", "dummy")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{
			"category": "fps",
			"hidden": true,
			"auth": "dummy",
			"id": "abc123",
			"age": 25
		}`, w.Body.String())

	// Should be able to get OpenAPI describing this API with its resource,
	// operation, schema, etc.
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/openapi.json", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTooBigBody(t *testing.T) {
	app := newTestRouter()

	type Input struct {
		Body struct {
			ID string `json:"id"`
		}
	}

	op := app.Resource("/test").Put("put", "desc",
		NewResponse(http.StatusNoContent, "desc"),
	)
	op.MaxBodyBytes(5)
	op.Run(func(ctx Context, input Input) {
		// Do nothing...
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{"id": "foo"}`))
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Request body too large")

	// With content length
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{"id": "foo"}`))
	req.Header.Set("Content-Length", "13")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Request body too large")
}

type timeoutError struct{}

func (e *timeoutError) Error() string {
	return "timed out"
}

func (e *timeoutError) Timeout() bool {
	return true
}

func (e *timeoutError) Temporary() bool {
	return false
}

type slowReader struct{}

func (r *slowReader) Read(p []byte) (int, error) {
	return 0, &timeoutError{}
}

func TestBodySlow(t *testing.T) {
	app := newTestRouter()

	type Input struct {
		Body struct {
			ID string
		}
	}

	op := app.Resource("/test").Put("put", "desc",
		NewResponse(http.StatusNoContent, "desc"),
	)
	op.BodyReadTimeout(1 * time.Millisecond)
	op.Run(func(ctx Context, input Input) {
		// Do nothing...
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/test", &slowReader{})
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "timed out")
}

func TestErrorHandlers(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("root", "desc",
		NewResponse(http.StatusNoContent, "desc"),
	).Run(func(ctx Context) {
		// Do nothing
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/notfound", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "/notfound")

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "PUT")
}
