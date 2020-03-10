package huma

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type helloResponse struct {
	Message string `json:"message"`
}

func BenchmarkGin(b *testing.B) {

	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.GET("/hello", func(c *gin.Context) {
		c.JSON(200, &helloResponse{
			Message: "Hello, world",
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/hello", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		g.ServeHTTP(w, req)
	}

	gin.SetMode(gin.DebugMode)
}

func BenchmarkHuma(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	r := NewRouterWithGin(gin.New(), &OpenAPI{
		Title:   "Benchmark test",
		Version: "1.0.0",
	})
	r.Register(&Operation{
		Method:      http.MethodGet,
		Path:        "/hello",
		Description: "Greet the world",
		Responses: []*Response{
			ResponseJSON(200, "Return a greeting"),
		},
		Handler: func() *helloResponse {
			return &helloResponse{
				Message: "Hello, world",
			}
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/hello", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r.ServeHTTP(w, req)
	}

	gin.SetMode(gin.DebugMode)
}

func TestRouter(t *testing.T) {
	type EchoResponse struct {
		Value string `json:"value" description:"The echoed back word"`
	}

	r := NewRouter(&OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(&Operation{
		Method:      http.MethodPut,
		Path:        "/echo/:word",
		Description: "Echo back an input word.",
		Params: []*Param{
			PathParam("word", "The word to echo back"),
			QueryParam("greet", "Return a greeting", false),
		},
		Responses: []*Response{
			ResponseJSON(http.StatusOK, "Successful echo response"),
			ResponseError(http.StatusBadRequest, "Invalid input"),
		},
		Handler: func(word string, greet bool) (*EchoResponse, *ErrorModel) {
			if word == "test" {
				return nil, &ErrorModel{Message: "Value not allowed: test"}
			}

			v := word
			if greet {
				v = "Hello, " + word
			}

			return &EchoResponse{Value: v}, nil
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

	// Check spec & docs routes
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/openapi.json", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/docs", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRouterRequestBody(t *testing.T) {
	type EchoRequest struct {
		Value string `json:"value"`
	}

	type EchoResponse struct {
		Value string `json:"value" description:"The echoed back word"`
	}

	r := NewRouter(&OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(&Operation{
		Method:      http.MethodPut,
		Path:        "/echo",
		Description: "Echo back an input word.",
		Responses: []*Response{
			ResponseJSON(http.StatusOK, "Successful echo response"),
		},
		Handler: func(in *EchoRequest) *EchoResponse {
			spew.Dump(in)
			return &EchoResponse{Value: in.Value}
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/echo", bytes.NewBufferString(`{"value": 123}`))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/echo", bytes.NewBufferString(`{"value": "hello"}`))
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, `{"value":"hello"}`+"\n", w.Body.String())
}

func TestRouterScalarResponse(t *testing.T) {
	r := NewRouter(&OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(&Operation{
		Method:      http.MethodPut,
		Path:        "/hello",
		Description: "Say hello.",
		Responses: []*Response{
			ResponseText(http.StatusOK, "Successful hello response"),
		},
		Handler: func() string {
			return "hello"
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/hello", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello", w.Body.String())
}

func TestRouterZeroScalarResponse(t *testing.T) {
	r := NewRouter(&OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(&Operation{
		Method:      http.MethodPut,
		Path:        "/bool",
		Description: "Say hello.",
		Responses: []*Response{
			ResponseText(http.StatusOK, "Successful zero bool response"),
		},
		Handler: func() *bool {
			resp := false
			return &resp
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/bool", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "false", w.Body.String())
}

func TestRouterResponseHeaders(t *testing.T) {
	r := NewRouter(&OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(&Operation{
		Method:      http.MethodGet,
		Path:        "/test",
		Description: "Test operation",
		ResponseHeaders: []*Header{
			ResponseHeader("Etag", "Identifies a specific version of this resource"),
			ResponseHeader("X-Test", "Custom test header"),
			ResponseHeader("X-Missing", "Won't get sent"),
		},
		Responses: []*Response{
			ResponseText(http.StatusOK, "Successful test", "Etag", "X-Test", "X-Missing"),
			ResponseError(http.StatusBadRequest, "Error example", "X-Test"),
		},
		Handler: func() (etag string, xTest *string, xMissing string, success string, fail string) {
			test := "test"
			return "\"abc123\"", &test, "", "hello", ""
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello", w.Body.String())
	assert.Equal(t, "\"abc123\"", w.Header().Get("Etag"))
	assert.Equal(t, "test", w.Header().Get("X-Test"))
	assert.Equal(t, "", w.Header().Get("X-Missing"))
}
