package huma

import (
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
