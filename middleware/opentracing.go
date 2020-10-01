package middleware

import (
	"net/http"

	"github.com/go-chi/chi"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

type spanContext interface {
	// SpanID returns the span ID that this context is carrying.
	SpanID() uint64

	// TraceID returns the trace ID that this context is carrying.
	TraceID() uint64
}

// OpenTracing provides a middleware for cross-service tracing support.
func OpenTracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracer := opentracing.GlobalTracer()

		// Get any incoming tracing context via HTTP headers & create the span.
		ctx, _ := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(r.Header))
		span := tracer.StartSpan("http.request", ext.RPCServerOption(ctx))
		defer span.Finish()

		// Set basic HTTP info
		ext.HTTPMethod.Set(span, r.Method)
		ext.HTTPUrl.Set(span, r.URL.String())
		ext.Component.Set(span, "huma")
		span.SetTag("span.type", "web")

		// Update context & continue the middleware chain.
		r = r.WithContext(opentracing.ContextWithSpan(r.Context(), span))
		ws := statusRecorder{ResponseWriter: w}
		next.ServeHTTP(&ws, r)

		// If we have a Chi route template, save it
		if chictx := chi.RouteContext(r.Context()); chictx != nil {
			span.SetTag("resource.name", chictx.RoutePattern())
			span.SetOperationName(r.Method + " " + chictx.RoutePattern())
		}

		// Save the status code
		ext.HTTPStatusCode.Set(span, uint16(ws.status))
	})
}
