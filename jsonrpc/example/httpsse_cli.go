package example

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/danielgtaylor/huma/v2/jsonrpc"
)

// CLI options can be added as needed
type Options struct {
	Host  string `doc:"Host to listen on" default:"localhost"`
	Port  int    `doc:"Port to listen on" default:"8080"`
	Debug bool   `doc:"Enable debug logs" default:"false"`
}

// This is a huma middleware.
// Either a huma middleware can be added or a http handler middleware can be added
func loggingMiddleware(ctx huma.Context, next func(huma.Context)) {
	// log.Printf("Received request: %v %v", ctx.URL().RawPath, ctx.Operation().Path)
	next(ctx)
	// log.Printf("Responded to request: %v %v", ctx.URL().RawPath, ctx.Operation().Path)
}

// This is a http handler middleware.
// PanicRecoveryMiddleware recovers from panics in handlers
func PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic to stderr
				log.Printf("Recovered from panic: %+v", err)

				// Optionally, log the stack trace
				log.Printf("%s", debug.Stack())

				// Return a 500 Internal Server Error
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func SetupSSETransport() http.Handler {
	// Use default go router
	router := http.NewServeMux()

	api := humago.New(router, huma.DefaultConfig("Example JSONRPC API", "1.0.0"))
	// Add any middlewares
	api.UseMiddleware(loggingMiddleware)
	handler := PanicRecoveryMiddleware(router)

	// Init the servers method and notifications handlers
	methodMap := GetMethodHandlers()
	notificationMap := GetNotificationHandlers()
	op := jsonrpc.GetDefaultOperation()
	// Register the methods
	jsonrpc.Register(api, op, methodMap, notificationMap)

	return handler
}

func GetHTTPServerCLI() humacli.CLI {

	cli := humacli.New(func(hooks humacli.Hooks, opts *Options) {
		log.Printf("Options are %+v\n", opts)
		handler := SetupSSETransport()
		// Initialize the http server
		server := http.Server{
			Addr:    fmt.Sprintf("%s:%d", opts.Host, opts.Port),
			Handler: handler,
		}

		// Hook the HTTP server.
		hooks.OnStart(func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %s\n", err)
			}
		})

		hooks.OnStop(func() {
			// Gracefully shutdown your server here
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
		})
	})

	return cli
}

func StartHTTPServer() {
	cli := GetHTTPServerCLI()
	cli.Run()
}
