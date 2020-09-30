package middleware

import (
	"context"
	"net/http"

	"github.com/go-chi/chi"
	"go.uber.org/zap"
)

// Middlewarer lets you add middlewares
type Middlewarer interface {
	Middleware(middlewares ...func(next http.Handler) http.Handler)
}

// Flagger lets you create command line flags and functions that use them.
type Flagger interface {
	Flag(name string, short string, description string, defaultValue interface{})
	PreStart(f func())
}

// DefaultChain sets up the default middlewares conveniently chained together
// into a single easy-to-add handler.
func DefaultChain(next http.Handler) http.Handler {
	// Note: logger goes before recovery so that recovery can use it. We don't
	// expect the logger to cause panics.
	return chi.Chain(
		OpenTracing,
		Logger,
		Recovery(func(ctx context.Context, err error, request string) {
			log := GetLogger(ctx)
			log = log.With(zap.Error(err))
			log.With(
				zap.String("http.request", request),
				zap.String("http.template", chi.RouteContext(ctx).RoutePattern()),
			).Error("Caught panic")
		}),
		ContentEncoding,
		PreferMinimal,
	).Handler(next)
}

// Defaults sets up the default middleware. This convenience function adds the
// `DefaultChain` to the router and adds the `--debug` option for logging to
// the CLI if app is a CLI.
func Defaults(app Middlewarer) {
	// Add the default middleware chain.
	app.Middleware(DefaultChain)

	// Add the command line options.
	if flagger, ok := app.(Flagger); ok {
		AddLoggerOptions(flagger)
	}
}
