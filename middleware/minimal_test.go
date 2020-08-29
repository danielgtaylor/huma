package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/responses"
	"github.com/stretchr/testify/assert"
)

func TestPreferMinimalMiddleware(t *testing.T) {
	app, _ := NewTestRouter(t)

	app.Resource("/test").Get("id", "desc",
		responses.OK().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		ctx.Write([]byte("Hello, test"))
	})

	app.Resource("/non200").Get("id", "desc",
		responses.BadRequest().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		ctx.WriteHeader(http.StatusBadRequest)
		ctx.Write([]byte("Error details"))
	})

	// Normal request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())

	// Prefer minimal should return 204 No Content
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Add("prefer", "return=minimal")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())

	// Prefer minimal which can still return non-200 response bodies
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/non200", nil)
	req.Header.Add("prefer", "return=minimal")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotEmpty(t, w.Body.String())
}
