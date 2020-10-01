// Package humatest provides testing utilities for testing Huma-powered
// services.
package humatest

import (
	"testing"

	"github.com/istreamlabs/huma"
	"github.com/istreamlabs/huma/middleware"
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

	router := huma.New("Test API", "1.0.0")
	router.Middleware(middleware.DefaultChain)

	middleware.NewLogger = func() (*zap.Logger, error) {
		l := zaptest.NewLogger(t, zaptest.WrapOptions(zap.WrapCore(func(zapcore.Core) zapcore.Core { return core })))
		return l, nil
	}

	return router, logs
}
