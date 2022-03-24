package humatest_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/humatest"
	"github.com/danielgtaylor/huma/middleware"
	"github.com/danielgtaylor/huma/responses"
	"github.com/stretchr/testify/assert"
)

func Example() {
	// Normally you will have a T from your test runner.
	t := &testing.T{}

	// Create the test router. Logs will be hidden unless the test fails.
	r := humatest.NewRouter(t)

	// Set up routes & handlers.
	r.Resource("/test").Get("test", "Test get",
		responses.OK().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		ctx.Write([]byte("Hello, test!"))
	})

	// Make a test request.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello, test!", w.Body.String())
}

func TestPackage(t *testing.T) {
	// Create the test router. Logs will be hidden unless the test fails.
	r := humatest.NewRouter(t)

	// Set up routes & handlers.
	r.Resource("/test").Get("test", "Test get",
		responses.OK().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		ctx.Write([]byte("Hello, test!"))
	})

	// Make a test request.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello, test!", w.Body.String())
}

func TestCapturedLog(t *testing.T) {
	// Create the test router. Logs will be hidden unless the test fails.
	r, logs := humatest.NewRouterObserver(t)

	// Set up routes & handlers.
	r.Resource("/test").Get("test", "Test get",
		responses.OK().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		logger := middleware.GetLogger(ctx)
		logger.With("foo", "bar").Info("Just a test")
		ctx.Write([]byte(""))
	})

	// Make a test request.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Just a test", logs.All()[0].Message)
	assert.Equal(t, "bar", logs.All()[0].ContextMap()["foo"])
}
