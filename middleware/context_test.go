package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/middleware"
)

func TestRequestContextDefaults(t *testing.T) {
	_, api := humatest.New(t)
	api.UseMiddleware(middleware.RequestContext(middleware.RequestContextConfig{
		NewRequestID: func() string { return "req-123" },
	}))

	var gotRequestID string
	var gotCorrelationID string
	var gotTrace middleware.TraceContext
	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		gotRequestID = middleware.RequestID(ctx)
		gotCorrelationID = middleware.CorrelationID(ctx)
		gotTrace = middleware.Trace(ctx)
		return &struct{}{}, nil
	})

	resp := api.Get("/test")
	if header := resp.Header().Get("X-Request-Id"); header != "req-123" {
		t.Fatalf("response request ID header = %q, want req-123", header)
	}
	if gotRequestID != "req-123" {
		t.Fatalf("RequestID = %q, want req-123", gotRequestID)
	}
	if gotCorrelationID != "req-123" {
		t.Fatalf("CorrelationID = %q, want req-123", gotCorrelationID)
	}
	if gotTrace.Valid {
		t.Fatal("Trace.Valid = true, want false")
	}
}

func TestRequestContextTraceparent(t *testing.T) {
	const traceparent = "00-3d23d071b5bfd6579171efce907685cb-08f067aa0ba902b7-03"
	const tracestate = "congo=t61rcWkgMzE"

	_, api := humatest.New(t)
	api.UseMiddleware(middleware.RequestContext(middleware.RequestContextConfig{
		NewRequestID: func() string { return "req-123" },
	}))

	var gotRequestID string
	var gotCorrelationID string
	var gotTrace middleware.TraceContext
	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		gotRequestID = middleware.RequestID(ctx)
		gotCorrelationID = middleware.CorrelationID(ctx)
		gotTrace = middleware.Trace(ctx)
		return &struct{}{}, nil
	})

	resp := api.Get("/test", "traceparent: "+traceparent, "tracestate: "+tracestate)
	if header := resp.Header().Get("X-Request-Id"); header != "req-123" {
		t.Fatalf("response request ID header = %q, want req-123", header)
	}
	if gotRequestID != "req-123" {
		t.Fatalf("RequestID = %q, want req-123", gotRequestID)
	}
	if gotCorrelationID != "3d23d071b5bfd6579171efce907685cb" {
		t.Fatalf("CorrelationID = %q, want trace ID", gotCorrelationID)
	}
	if !gotTrace.Valid {
		t.Fatal("Trace.Valid = false, want true")
	}
	if !gotTrace.Sampled {
		t.Fatal("Trace.Sampled = false, want true")
	}
	if gotTrace.Tracestate != tracestate {
		t.Fatalf("Trace.Tracestate = %q, want %q", gotTrace.Tracestate, tracestate)
	}
}

func TestRequestContextRejectsUnsafeRequestID(t *testing.T) {
	_, api := humatest.New(t)
	api.UseMiddleware(middleware.RequestContext(middleware.RequestContextConfig{
		NewRequestID: func() string { return "safe-id" },
	}))

	var gotRequestID string
	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		gotRequestID = middleware.RequestID(ctx)
		return &struct{}{}, nil
	})

	resp := api.Get("/test", "X-Request-Id: bad id")
	if gotRequestID != "safe-id" {
		t.Fatalf("RequestID = %q, want safe-id", gotRequestID)
	}
	if header := resp.Header().Get("X-Request-Id"); header != "safe-id" {
		t.Fatalf("response request ID header = %q, want safe-id", header)
	}
}

func TestRequestContextAcceptsSafeRequestID(t *testing.T) {
	_, api := humatest.New(t)
	api.UseMiddleware(middleware.RequestContext(middleware.RequestContextConfig{
		NewRequestID: func() string { return "generated-id" },
	}))

	var gotRequestID string
	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		gotRequestID = middleware.RequestID(ctx)
		return &struct{}{}, nil
	})

	resp := api.Get("/test", "X-Request-Id: client-id_123")
	if gotRequestID != "client-id_123" {
		t.Fatalf("RequestID = %q, want client-id_123", gotRequestID)
	}
	if header := resp.Header().Get("X-Request-Id"); header != "client-id_123" {
		t.Fatalf("response request ID header = %q, want client-id_123", header)
	}
}

func TestRequestContextCanDisableResponseHeader(t *testing.T) {
	_, api := humatest.New(t)
	api.UseMiddleware(middleware.RequestContext(middleware.RequestContextConfig{
		NewRequestID:                   func() string { return "req-123" },
		DisableResponseRequestIDHeader: true,
	}))

	var gotRequestID string
	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		gotRequestID = middleware.RequestID(ctx)
		return &struct{}{}, nil
	})

	resp := api.Get("/test")
	if gotRequestID != "req-123" {
		t.Fatalf("RequestID = %q, want req-123", gotRequestID)
	}
	if header := resp.Header().Get("X-Request-Id"); header != "" {
		t.Fatalf("response request ID header = %q, want empty", header)
	}
}

func TestRequestContextHumagoAdapter(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Test API", "1.0.0"))
	api.UseMiddleware(middleware.RequestContext(middleware.RequestContextConfig{
		NewRequestID: func() string { return "humago-id" },
	}))

	var gotRequestID string
	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		gotRequestID = middleware.RequestID(ctx)
		return &struct{}{}, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)

	if gotRequestID != "humago-id" {
		t.Fatalf("RequestID = %q, want humago-id", gotRequestID)
	}
	if header := resp.Header().Get("X-Request-Id"); header != "humago-id" {
		t.Fatalf("response request ID header = %q, want humago-id", header)
	}
}
