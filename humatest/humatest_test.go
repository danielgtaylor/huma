package humatest_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/istreamlabs/huma"
	"github.com/istreamlabs/huma/humatest"
	"github.com/stretchr/testify/assert"
)

func Example() {
	// Normally you will have a T from your test runner.
	t := &testing.T{}

	// Create the test router. Logs will be hidden unless the test fails.
	r := humatest.NewRouter(t)

	// Set up routes & handlers.
	r.Resource("/test").Get("Test get", func() string {
		return "Hello, test!"
	})

	// Make a test request.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello, test!", w.Body.String())
}

func TestNewRouter(t *testing.T) {
	// Should not panic
	humatest.NewRouter(t, huma.DevServer("http://localhost:8888"))
}
