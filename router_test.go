package huma

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/schema"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func ExampleNewRouter_customGin() {
	g := gin.New()
	// ...Customize your gin instance...

	r := NewRouter("Example API", "1.0.0", Gin(g))
	r.Resource("/").Get("doc", func() string { return "Custom Gin" })
}

func ExampleNewRouter() {
	r := NewRouter("Example API", "1.0.0",
		DevServer("http://localhost:8888"),
		ContactEmail("Support", "support@example.com"),
	)

	r.Resource("/hello").Get("doc", func() string { return "Hello" })
}

func NewTestRouter(t *testing.T, options ...RouterOption) *Router {
	l := zaptest.NewLogger(t)
	g := gin.New()
	g.Use(LogMiddleware(Logger(l)))

	return NewRouter("Test API", "1.0.0", append([]RouterOption{Gin(g)}, options...)...)
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
	r := NewRouter("Benchmark test", "1.0.0", Gin(gin.New()))
	r.Resource("/hello").Get("Greet the world", func() *helloResponse {
		return &helloResponse{
			Message: "Hello, world",
		}
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
				Detail: "Name cannot be test",
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
	r := NewRouter("Benchmark test", "1.0.0", Gin(gin.New()))

	dep1 := SimpleDependency("dep1")

	dep2 := Dependency(DependencyOptions(
		ContextDependency(), dep1, HeaderParam("x-foo", "desc", ""),
	), func(c *gin.Context, d1 string, xfoo string) (string, error) {
		return "dep2", nil
	})

	dep3 := Dependency(DependencyOptions(
		dep1, ResponseHeader("x-bar", "desc"),
	), func(d1 string) (string, string, error) {
		return "xbar", "dep3", nil
	})

	r.Resource("/hello", dep1, dep2, dep3,
		QueryParam("name", "desc", "world"),
		ResponseHeader("x-baz", "desc"),
		ResponseJSON(200, "Return a greeting", Headers("x-baz")),
		ResponseError(500, "desc"),
	).Get("Greet the world", func(c *gin.Context, d2, d3, name string) (string, *helloResponse, *ErrorModel) {
		if name == "test" {
			return "", nil, &ErrorModel{
				Detail: "Name cannot be test",
			}
		}

		return "xbaz", &helloResponse{
			Message: "Hello, " + name,
		}, nil
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
	_ = NewTestRouter(t)
}

func TestRouterConfigurableCors(t *testing.T) {
	cfg := cors.DefaultConfig()
	cfg.AllowAllOrigins = true
	cfg.AllowHeaders = append(cfg.AllowHeaders, "Authorization", "X-My-Header")

	r := NewTestRouter(t, CORSHandler(cors.New(cfg)))

	type PongResponse struct {
		Value string `json:"value" description:"The echoed back word"`
	}

	r.Resource("/ping",
		ResponseJSON(http.StatusOK, "Successful echo response"),
		ResponseError(http.StatusBadRequest, "Invalid input"),
	).Get("ping", func() (*PongResponse, *ErrorModel) {

		return &PongResponse{Value: "pong"}, nil
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodOptions, "/ping", nil)
	req.Header.Add("Origin", "blah")
	r.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	allowedHeaders := w.Header().Get("Access-Control-Allow-Headers")
	assert.Equal(t, true, strings.Contains(allowedHeaders, "Authorization"))
	assert.Equal(t, true, strings.Contains(allowedHeaders, "X-My-Header"))

}

func TestRouter(t *testing.T) {
	type EchoResponse struct {
		Value string `json:"value" description:"The echoed back word"`
	}

	r := NewTestRouter(t)

	r.Resource("/echo",
		PathParam("word", "The word to echo back"),
		QueryParam("greet", "Return a greeting", false),
		ResponseJSON(http.StatusOK, "Successful echo response"),
		ResponseError(http.StatusBadRequest, "Invalid input"),
	).Put("Echo back an input word.", func(word string, greet bool) (*EchoResponse, *ErrorModel) {
		if word == "test" {
			return nil, &ErrorModel{Detail: "Value not allowed: test"}
		}

		v := word
		if greet {
			v = "Hello, " + word
		}

		return &EchoResponse{Value: v}, nil
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

	r := NewTestRouter(t)

	r.Resource("/echo").Put("Echo back an input word.", func(in *EchoRequest) *EchoResponse {
		return &EchoResponse{Value: in.Value}
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
	r := NewTestRouter(t)

	r.Resource("/hello").Put("Say hello", func() string {
		return "hello"
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/hello", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello", w.Body.String())
}

func TestRouterZeroScalarResponse(t *testing.T) {
	r := NewTestRouter(t)

	r.Resource("/bool").Put("Bool response", func() *bool {
		resp := false
		return &resp
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/bool", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "false\n", w.Body.String())
}

func TestRouterResponseHeaders(t *testing.T) {
	r := NewTestRouter(t)

	r.Resource("/test",
		ResponseHeader("Etag", "Identifies a specific version of this resource"),
		ResponseHeader("X-Test", "Custom test header"),
		ResponseHeader("X-Missing", "Won't get sent"),
		ResponseText(http.StatusOK, "Successful test", Headers("Etag", "X-Test", "X-Missing")),
		ResponseError(http.StatusBadRequest, "Error example", Headers("X-Test")),
	).Get("Test operation", func() (etag string, xTest *string, xMissing string, success string, fail string) {
		test := "test"
		return "\"abc123\"", &test, "", "hello", ""
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
	r := NewTestRouter(t)

	type DB struct {
		Get func() string
	}

	// Datastore is a global dependency, set by value.
	db := &DB{
		Get: func() string {
			return "Hello, "
		},
	}

	type Logger struct {
		Log func(msg string)
	}

	// Logger is a contextual instance from the gin request context.
	captured := ""
	log := Dependency(GinContextDependency(), func(c *gin.Context) (*Logger, error) {
		return &Logger{
			Log: func(msg string) {
				captured = fmt.Sprintf("%s [uri:%s]", msg, c.FullPath())
			},
		}, nil
	})

	r.Resource("/hello",
		GinContextDependency(),
		SimpleDependency(db),
		log,
		QueryParam("name", "Your name", ""),
	).Get("Basic hello world", func(c *gin.Context, db *DB, l *Logger, name string) string {
		if name == "" {
			name = c.Request.RemoteAddr
		}
		l.Log("Hello logger!")
		return db.Get() + name
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/hello?name=foo", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello logger! [uri:/hello]", captured)
}

func TestRouterBadHeader(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	l := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
	g := gin.New()
	g.Use(LogMiddleware(Logger(l)))
	r := NewRouter("Test API", "1.0.0", Gin(g))
	r.Resource("/test", ResponseHeader("foo", "desc"), ResponseError(http.StatusBadRequest, "desc", Headers("foo"))).Get("desc", func() (string, *ErrorModel, string) {
		return "header-value", nil, "response"
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, logs.FilterMessageSnippet("did not declare").All())
}

func TestRouterParams(t *testing.T) {
	r := NewTestRouter(t)

	r.Resource("/test",
		PathParam("id", "desc"),
		QueryParam("i", "desc", int16(0)),
		QueryParam("f32", "desc", float32(0.0)),
		QueryParam("f64", "desc", 0.0),
		QueryParam("schema", "desc", "test", Schema(schema.Schema{Pattern: "^a-z+$"})),
		QueryParam("items", "desc", []int{}),
		QueryParam("start", "desc", time.Time{}),
	).Get("desc", func(id string, i int16, f32 float32, f64 float64, schema string, items []int, start time.Time) string {
		return fmt.Sprintf("%s %v %v %v %v %v %v", id, i, f32, f64, schema, items, start)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test/someId?i=1&f32=1.0&f64=123.45&items=1,2,3&start=2020-01-01T12:00:00Z", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "someId 1 1 123.45 test [1 2 3] 2020-01-01 12:00:00 +0000 UTC", w.Body.String())

	// Arrays can be sent as JSON arrays
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?items=[1,2,3]", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Failure parsing tests
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?i=bad", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?f32=bad", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?f64=bad", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?schema=foo1", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?items=1,2,bad", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?start=bad", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Invalid Go number should return an error, may support these in the future.
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/someId?items=1e10", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInvalidParamLocation(t *testing.T) {
	r := NewTestRouter(t)

	test := r.Resource("/test", PathParam("name", "desc"))
	test.params[len(test.params)-1].In = "bad'"

	assert.Panics(t, func() {
		test.Get("desc", func(test string) string {
			return "Hello, test!"
		})
	})
}

func TestEmptyShutdownPanics(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Shutdown(context.TODO())
	})
}

func TestTooBigBody(t *testing.T) {
	r := NewTestRouter(t)

	type Input struct {
		ID string
	}

	r.Resource("/test", MaxBodyBytes(5)).Put("desc", func(input *Input) string {
		return "hello, " + input.ID
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{"id": "foo"}`))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
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
	r := NewTestRouter(t)

	type Input struct {
		ID string
	}

	r.Resource("/test", BodyReadTimeout(1)).Put("desc", func(input *Input) string {
		return "hello, " + input.ID
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/test", &slowReader{})
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestTimeout, w.Code)
	assert.Contains(t, w.Body.String(), "timed out")
}

func TestRouterUnsafeHandler(t *testing.T) {
	r := NewTestRouter(t)

	type Item struct {
		ID    string `json:"id" readOnly:"true"`
		Value int    `json:"value"`
	}

	readSchema, _ := schema.GenerateWithMode(reflect.TypeOf(Item{}), schema.ModeRead, nil)
	writeSchema, _ := schema.GenerateWithMode(reflect.TypeOf(Item{}), schema.ModeWrite, nil)

	items := map[string]Item{}

	res := r.Resource("/test", PathParam("id", "doc"))

	// Write handler
	res.With(
		RequestSchema(writeSchema),
		Response(http.StatusNoContent, "doc"),
	).Put("doc", UnsafeHandler(func(inputs ...interface{}) []interface{} {
		id := inputs[0].(string)
		item := inputs[1].(map[string]interface{})

		items[id] = Item{
			ID:    id,
			Value: int(item["value"].(float64)),
		}

		return []interface{}{true}
	}))

	// Read handler
	res.With(
		ResponseJSON(http.StatusOK, "doc", Schema(*readSchema)),
	).Get("doc", UnsafeHandler(func(inputs ...interface{}) []interface{} {
		id := inputs[0].(string)

		return []interface{}{items[id]}
	}))

	// Create an item
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/test/some-id", strings.NewReader(`{"value": 123}`))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	// Read the item
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test/some-id", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}
