package huma

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type helloResponse struct {
	Message string `json:"message"`
}

func BenchmarkGin(b *testing.B) {
	g := gin.New()
	g.GET("/hello", func(c *gin.Context) {
		c.JSON(200, &helloResponse{
			Message: "Hello, world",
		})
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		g.ServeHTTP(w, req)
	}
}

func BenchmarkHuma(b *testing.B) {
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

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		r.ServeHTTP(w, req)
	}
}

func BenchmarkGinComplex(b *testing.B) {
	dep1 := "dep1"
	dep2 := func(c *gin.Context) string {
		_ = c.GetHeader("x-foo")
		return "dep2"
	}
	dep3 := func(c *gin.Context) (string, string) {
		return "xbar", "dep3"
	}

	g := gin.New()
	g.GET("/hello", func(c *gin.Context) {
		_ = dep1
		_ = dep2(c)
		h, _ := dep3(c)

		c.Header("x-bar", h)

		name := c.Query("name")
		if name == "test" {
			c.JSON(400, &ErrorModel{
				Message: "Name cannot be test",
			})
		}
		if name == "" {
			name = "world"
		}

		c.Header("x-baz", "xbaz")
		c.JSON(200, &helloResponse{
			Message: "Hello, " + name,
		})
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/hello", nil)
		g.ServeHTTP(w, req)
	}
}

func BenchmarkHumaComplex(b *testing.B) {
	r := NewRouterWithGin(gin.New(), &OpenAPI{
		Title:   "Benchmark test",
		Version: "1.0.0",
	})

	dep1 := &Dependency{
		Value: "dep1",
	}

	dep2 := &Dependency{
		Dependencies: []*Dependency{ContextDependency(), dep1},
		Params: []*Param{
			HeaderParam("x-foo", "desc", ""),
		},
		Value: func(c *gin.Context, d1 string, xfoo string) (string, error) {
			return "dep2", nil
		},
	}

	dep3 := &Dependency{
		Dependencies: []*Dependency{dep1},
		ResponseHeaders: []*ResponseHeader{
			Header("x-bar", "desc"),
		},
		Value: func(d1 string) (string, string, error) {
			return "xbar", "dep3", nil
		},
	}

	r.Register(&Operation{
		Method:      http.MethodGet,
		Path:        "/hello",
		Description: "Greet the world",
		Dependencies: []*Dependency{
			ContextDependency(), dep2, dep3,
		},
		Params: []*Param{
			QueryParam("name", "desc", "world"),
		},
		ResponseHeaders: []*ResponseHeader{
			Header("x-baz", "desc"),
		},
		Responses: []*Response{
			ResponseJSON(200, "Return a greeting", "x-baz"),
			ResponseError(500, "desc"),
		},
		Handler: func(c *gin.Context, d2, d3, name string) (string, *helloResponse, *ErrorModel) {
			if name == "test" {
				return "", nil, &ErrorModel{
					Message: "Name cannot be test",
				}
			}

			return "xbaz", &helloResponse{
				Message: "Hello, " + name,
			}, nil
		},
	})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/hello?name=Daniel", nil)
		r.ServeHTTP(w, req)
	}
}

func TestRouterDefault(t *testing.T) {
	// Just test we can create it without panic.
	_ = NewRouter(&OpenAPI{Title: "Default", Version: "1.0.0"})
}

func TestRouter(t *testing.T) {
	type EchoResponse struct {
		Value string `json:"value" description:"The echoed back word"`
	}

	r := NewRouterWithGin(gin.New(), &OpenAPI{Title: "My API", Version: "1.0.0"})

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

	r := NewRouterWithGin(gin.New(), &OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(&Operation{
		Method:      http.MethodPut,
		Path:        "/echo",
		Description: "Echo back an input word.",
		Responses: []*Response{
			ResponseJSON(http.StatusOK, "Successful echo response"),
		},
		Handler: func(in *EchoRequest) *EchoResponse {
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
	r := NewRouterWithGin(gin.New(), &OpenAPI{Title: "My API", Version: "1.0.0"})

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
	r := NewRouterWithGin(gin.New(), &OpenAPI{Title: "My API", Version: "1.0.0"})

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
	r := NewRouterWithGin(gin.New(), &OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(&Operation{
		Method:      http.MethodGet,
		Path:        "/test",
		Description: "Test operation",
		ResponseHeaders: []*ResponseHeader{
			Header("Etag", "Identifies a specific version of this resource"),
			Header("X-Test", "Custom test header"),
			Header("X-Missing", "Won't get sent"),
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

func TestRouterDependencies(t *testing.T) {
	r := NewRouterWithGin(gin.New(), &OpenAPI{Title: "My API", Version: "1.0.0"})

	type DB struct {
		Get func() string
	}

	// Datastore is a global dependency, set by value.
	db := &Dependency{
		Value: &DB{
			Get: func() string {
				return "Hello, "
			},
		},
	}

	type Logger struct {
		Log func(msg string)
	}

	// Logger is a contextual instance from the gin request context.
	captured := ""
	log := &Dependency{
		Dependencies: []*Dependency{
			ContextDependency(),
		},
		Value: func(c *gin.Context) (*Logger, error) {
			return &Logger{
				Log: func(msg string) {
					captured = fmt.Sprintf("%s [uri:%s]", msg, c.FullPath())
				},
			}, nil
		},
	}

	r.Register(&Operation{
		Method:       http.MethodGet,
		Path:         "/hello",
		Description:  "Basic hello world",
		Dependencies: []*Dependency{ContextDependency(), db, log},
		Params: []*Param{
			QueryParam("name", "Your name", ""),
		},
		Responses: []*Response{
			ResponseText(http.StatusOK, "Successful hello response"),
		},
		Handler: func(c *gin.Context, db *DB, l *Logger, name string) string {
			if name == "" {
				name = c.Request.RemoteAddr
			}
			l.Log("Hello logger!")
			return db.Get() + name
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/hello?name=foo", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello logger! [uri:/hello]", captured)
}
