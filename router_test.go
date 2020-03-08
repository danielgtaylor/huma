package huma

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	type EchoResponse struct {
		Value string `json:"value" description:"The echoed back word"`
	}

	r := NewRouter(&OpenAPI{
		Title:   "My API",
		Version: "1.0.0",
	})

	r.Register(&Operation{
		ID:          "echo",
		Method:      http.MethodPut,
		Path:        "/echo/:word",
		Description: "Echo back an input word.",
		Params: []*Param{
			PathParam("word", "The word to echo back"),
			QueryParam("greet", "Return a greeting"),
		},
		Responses: []*Response{
			ResponseJSON(http.StatusOK, "Successful echo response"),
			ResponseError(http.StatusBadRequest, "Invalid input"),
		},
		Handler: func(word string, greet bool) (int, *EchoResponse, *ErrorModel) {
			if word == "test" {
				return http.StatusBadRequest, nil, &ErrorModel{Message: "Value not allowed: test"}
			}

			v := word
			if greet {
				v = "Hello, " + word
			}

			return http.StatusOK, &EchoResponse{Value: v}, nil
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/echo/world", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"value":"world"}`+"\n", w.Body.String())

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/echo/world?greet=true", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"value":"Hello, world"}`+"\n", w.Body.String())

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/echo/world?greet=bad", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRouterRequestBody(t *testing.T) {
	type EchoRequest struct {
		Value string `json:"value"`
	}

	type EchoResponse struct {
		Value string `json:"value" description:"The echoed back word"`
	}

	r := NewRouter(&OpenAPI{
		Title:   "My API",
		Version: "1.0.0",
	})

	r.Register(&Operation{
		ID:          "echo",
		Method:      http.MethodPut,
		Path:        "/echo",
		Description: "Echo back an input word.",
		Responses: []*Response{
			ResponseJSON(http.StatusOK, "Successful echo response"),
		},
		Handler: func(in *EchoRequest) (int, *EchoResponse) {
			spew.Dump(in)
			return http.StatusOK, &EchoResponse{Value: in.Value}
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/echo", bytes.NewBufferString(`{"value": "hello"}`))
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"value":"hello"}`+"\n", w.Body.String())
}
