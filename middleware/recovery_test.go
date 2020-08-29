package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/responses"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
)

func NewTestRouter(t testing.TB) (*huma.Router, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)

	router := huma.New("Test API", "1.0.0")
	router.Middleware(DefaultChain)

	NewLogger = func() (*zap.Logger, error) {
		l := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
		return l, nil
	}

	return router, logs
}

func TestRecoveryMiddleware(t *testing.T) {
	app, _ := NewTestRouter(t)

	app.Resource("/panic").Get("panic", "Panic recovery test",
		responses.NoContent(),
	).Run(func(ctx huma.Context) {
		panic(fmt.Errorf("Some error"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Result().Header.Get("content-type"))
}

func TestRecoveryMiddlewareString(t *testing.T) {
	app, _ := NewTestRouter(t)

	app.Resource("/panic").Get("panic", "Panic recovery test",
		responses.NoContent(),
	).Run(func(ctx huma.Context) {
		panic("Some error")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Result().Header.Get("content-type"))
}

func TestRecoveryMiddlewareLogBody(t *testing.T) {
	app, log := NewTestRouter(t)

	app.Resource("/panic").Put("panic", "Panic recovery test",
		responses.NoContent(),
	).Run(func(ctx huma.Context, input struct {
		Body struct {
			Foo string `json:"foo"`
		}
	}) {
		panic(fmt.Errorf("Some error"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/panic", strings.NewReader(`{"foo": "bar"}`))
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Result().Header.Get("content-type"))
	assert.Contains(t, log.All()[0].ContextMap()["request"], `{"foo": "bar"}`)
}
