package huma

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	r.Resource("/players", "category").Post("player", "Create player",
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
}
