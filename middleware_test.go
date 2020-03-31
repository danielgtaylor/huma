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

	r.Register(&Operation{
		Method:      http.MethodGet,
		Path:        "/panic",
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
