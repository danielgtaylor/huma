package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestRecoveryMiddleware(t *testing.T) {
	g := gin.New()

	l := zaptest.NewLogger(t)
	g.Use(LogMiddleware(l, nil))
	g.Use(Recovery())

	r := NewRouterWithGin(g, &OpenAPI{Title: "My API", Version: "1.0.0"})

	r.Register(http.MethodGet, "/panic", &Operation{
		Description: "Panic recovery test",
		Responses: []*Response{
			ResponseText(http.StatusOK, "Success"),
		},
		Handler: func() string {
			panic(fmt.Errorf("Some error"))
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Result().Header.Get("content-type"))
}

func TestPreferMinimalMiddleware(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(PreferMinimalMiddleware())

	r.Resource("/test").Get("desc", func() string {
		return "Hello, test"
	})

	r.Resource("/non200", ResponseText(http.StatusBadRequest, "desc")).Get("desc", func() string {
		return "Error details"
	})

	// Normal request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())

	// Prefer minimal should return 204 No Content
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Add("prefer", "return=minimal")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())

	// Prefer minimal which can still return non-200 response bodies
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/non200", nil)
	req.Header.Add("prefer", "return=minimal")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotEmpty(t, w.Body.String())
}

func TestHandler404(t *testing.T) {
	g := gin.New()
	g.NoRoute(Handler404)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/notfound", nil)
	g.ServeHTTP(w, req)
	assert.Equal(t, w.Result().StatusCode, http.StatusNotFound)
	assert.Equal(t, "application/json; charset=utf-8", w.Result().Header.Get("content-type"))
}
