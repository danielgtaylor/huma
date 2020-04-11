// Package humatest provides testing utilities for testing Huma-powered
// services.
package humatest

import (
	"testing"

	"github.com/danielgtaylor/huma"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
)

// NewRouter creates a new test router. It includes a logger attached to the
// test so if it fails you will see additional output. There is no recovery
// middleware so panics will get caught by the test runner.
func NewRouter(t testing.TB) *huma.Router {
	r, _ := NewRouterObserver(t)
	return r
}

// NewRouterObserver creates a new router and a log output observer for testing
// log output at "debug" level and above during requests.
func NewRouterObserver(t testing.TB) (*huma.Router, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	l := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))

	g := gin.New()
	g.Use(huma.LogMiddleware(l, nil))

	return huma.NewRouter("Test API", "1.0.0", huma.WithGin(g)), logs
}
