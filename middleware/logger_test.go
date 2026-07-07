package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/middleware"
)

func TestNewJSONLoggerPresets(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		var buf bytes.Buffer
		logger := middleware.NewJSONLogger(middleware.JSONLoggerConfig{Writer: &buf})

		logger.Info("hello")

		entry := decodeLogEntries(t, &buf)[0]
		if entry["msg"] != "hello" {
			t.Fatalf("msg = %v, want hello", entry["msg"])
		}
		if entry["level"] != "INFO" {
			t.Fatalf("level = %v, want INFO", entry["level"])
		}
		if _, ok := entry["severity"]; ok {
			t.Fatalf("generic log should not include severity: %#v", entry)
		}
	})

	t.Run("gcp", func(t *testing.T) {
		var buf bytes.Buffer
		logger := middleware.NewJSONLogger(middleware.JSONLoggerConfig{
			Writer: &buf,
			Preset: middleware.LogPresetGCP,
		})

		logger.Warn("slow")

		entry := decodeLogEntries(t, &buf)[0]
		if entry["message"] != "slow" {
			t.Fatalf("message = %v, want slow", entry["message"])
		}
		if entry["severity"] != "WARNING" {
			t.Fatalf("severity = %v, want WARNING", entry["severity"])
		}
		if _, ok := entry["msg"]; ok {
			t.Fatalf("GCP log should not include msg: %#v", entry)
		}
	})

	t.Run("azure", func(t *testing.T) {
		var buf bytes.Buffer
		logger := middleware.NewJSONLogger(middleware.JSONLoggerConfig{
			Writer: &buf,
			Preset: middleware.LogPresetAzure,
		})

		logger.Warn("slow")

		entry := decodeLogEntries(t, &buf)[0]
		if entry["message"] != "slow" {
			t.Fatalf("message = %v, want slow", entry["message"])
		}
		if entry["level"] != "WARNING" {
			t.Fatalf("level = %v, want WARNING", entry["level"])
		}
		if _, ok := entry["msg"]; ok {
			t.Fatalf("Azure log should not include msg: %#v", entry)
		}
	})
}

func TestAccessLoggerGeneric(t *testing.T) {
	var buf bytes.Buffer
	_, api := humatest.New(t)
	api.UseMiddleware(
		middleware.RequestContext(middleware.RequestContextConfig{
			NewRequestID: func() string { return "req-123" },
		}),
		middleware.AccessLogger(middleware.AccessLoggerConfig{
			Logger: middleware.NewJSONLogger(middleware.JSONLoggerConfig{Writer: &buf}),
			Now:    sequenceNow(time.Unix(100, 0), time.Unix(100, int64(1500*time.Millisecond))),
		}),
	)

	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{ Status int }, error) {
		middleware.RequestLogger(ctx).InfoContext(ctx, "handler")
		return &struct{ Status int }{Status: http.StatusCreated}, nil
	})

	api.Get("/test")

	raw := strings.TrimSpace(buf.String())
	lines := strings.Split(raw, "\n")
	entries := decodeLogEntries(t, strings.NewReader(raw))
	if len(entries) != 2 {
		t.Fatalf("log entries = %d, want 2", len(entries))
	}
	if len(lines) != 2 {
		t.Fatalf("raw log lines = %d, want 2: %q", len(lines), raw)
	}
	if entries[0]["request_id"] != "req-123" {
		t.Fatalf("handler request_id = %v, want req-123", entries[0]["request_id"])
	}
	if count := strings.Count(lines[1], `"request_id":`); count != 1 {
		t.Fatalf("access log request_id count = %d, want 1: %s", count, lines[1])
	}
	if count := strings.Count(lines[1], `"correlation_id":`); count != 1 {
		t.Fatalf("access log correlation_id count = %d, want 1: %s", count, lines[1])
	}

	access := entries[1]
	if access["msg"] != "request completed" {
		t.Fatalf("access msg = %v, want request completed", access["msg"])
	}
	if access["method"] != http.MethodGet {
		t.Fatalf("method = %v, want GET", access["method"])
	}
	if access["path_template"] != "/test" {
		t.Fatalf("path_template = %v, want /test", access["path_template"])
	}
	if access["status"] != float64(http.StatusCreated) {
		t.Fatalf("status = %v, want 201", access["status"])
	}
	if access["duration_ms"] != float64(1500) {
		t.Fatalf("duration_ms = %v, want 1500", access["duration_ms"])
	}
}

func TestAccessLoggerGCPPreset(t *testing.T) {
	const traceparent = "00-3d23d071b5bfd6579171efce907685cb-08f067aa0ba902b7-01"

	var buf bytes.Buffer
	_, api := humatest.New(t)
	api.UseMiddleware(
		middleware.RequestContext(middleware.RequestContextConfig{
			NewRequestID: func() string { return "req-123" },
		}),
		middleware.AccessLogger(middleware.AccessLoggerConfig{
			Logger: middleware.NewJSONLogger(middleware.JSONLoggerConfig{
				Writer: &buf,
				Preset: middleware.LogPresetGCP,
			}),
			Preset: middleware.LogPresetGCP,
			GCP: middleware.GCPConfig{
				ProjectID: "test-project",
			},
			Now: sequenceNow(time.Unix(100, 0), time.Unix(100, int64(time.Millisecond))),
		}),
	)

	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{ Status int }, error) {
		return &struct{ Status int }{Status: http.StatusOK}, nil
	})

	api.Get("/test", "traceparent: "+traceparent)

	entry := decodeLogEntries(t, &buf)[0]
	if entry["message"] != "request completed" {
		t.Fatalf("message = %v, want request completed", entry["message"])
	}
	if entry["severity"] != "INFO" {
		t.Fatalf("severity = %v, want INFO", entry["severity"])
	}
	if entry["logging.googleapis.com/trace"] != "projects/test-project/traces/3d23d071b5bfd6579171efce907685cb" {
		t.Fatalf("trace field = %v", entry["logging.googleapis.com/trace"])
	}
	if entry["parentId"] != "08f067aa0ba902b7" {
		t.Fatalf("parentId = %v, want 08f067aa0ba902b7", entry["parentId"])
	}
	if entry["traceFlags"] != "01" {
		t.Fatalf("traceFlags = %v, want 01", entry["traceFlags"])
	}
	if _, ok := entry["logging.googleapis.com/spanId"]; ok {
		t.Fatalf("GCP log should not emit spanId without an active span: %#v", entry)
	}
	if entry["logging.googleapis.com/trace_sampled"] != true {
		t.Fatalf("trace_sampled = %v, want true", entry["logging.googleapis.com/trace_sampled"])
	}
	httpRequest := entry["httpRequest"].(map[string]any)
	if httpRequest["requestMethod"] != http.MethodGet {
		t.Fatalf("httpRequest.requestMethod = %v, want GET", httpRequest["requestMethod"])
	}
	if httpRequest["latency"] != "0.001s" {
		t.Fatalf("httpRequest.latency = %v, want 0.001s", httpRequest["latency"])
	}
}

func TestAccessLoggerGCPTraceWithoutProjectID(t *testing.T) {
	const traceparent = "00-3d23d071b5bfd6579171efce907685cb-08f067aa0ba902b7-01"

	var buf bytes.Buffer
	_, api := humatest.New(t)
	api.UseMiddleware(
		middleware.RequestContext(middleware.RequestContextConfig{
			NewRequestID: func() string { return "req-123" },
		}),
		middleware.AccessLogger(middleware.AccessLoggerConfig{
			Logger: middleware.NewJSONLogger(middleware.JSONLoggerConfig{
				Writer: &buf,
				Preset: middleware.LogPresetGCP,
			}),
			Preset: middleware.LogPresetGCP,
			Now:    sequenceNow(time.Unix(100, 0), time.Unix(100, int64(time.Millisecond))),
		}),
	)

	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		return &struct{}{}, nil
	})

	api.Get("/test", "traceparent: "+traceparent)

	entry := decodeLogEntries(t, &buf)[0]
	if entry["logging.googleapis.com/trace"] != "3d23d071b5bfd6579171efce907685cb" {
		t.Fatalf("trace field = %v", entry["logging.googleapis.com/trace"])
	}
	if _, ok := entry["traceId"]; ok {
		t.Fatalf("GCP log should use Cloud Logging trace field, not plain traceId: %#v", entry)
	}
	if entry["logging.googleapis.com/trace_sampled"] != true {
		t.Fatalf("trace_sampled = %v, want true", entry["logging.googleapis.com/trace_sampled"])
	}
}

func TestAccessLoggerAWSCloudWatchShape(t *testing.T) {
	const traceparent = "00-3d23d071b5bfd6579171efce907685cb-08f067aa0ba902b7-01"

	var buf bytes.Buffer
	_, api := humatest.New(t)
	api.UseMiddleware(
		middleware.RequestContext(middleware.RequestContextConfig{
			NewRequestID: func() string { return "req-123" },
		}),
		middleware.AccessLogger(middleware.AccessLoggerConfig{
			Logger: middleware.NewJSONLogger(middleware.JSONLoggerConfig{
				Writer: &buf,
				Preset: middleware.LogPresetAWS,
			}),
			Preset: middleware.LogPresetAWS,
			Now:    sequenceNow(time.Unix(100, 0), time.Unix(100, int64(time.Millisecond))),
			ExtraAttrs: func(huma.Context) []slog.Attr {
				return []slog.Attr{slog.String("service", "billing")}
			},
		}),
	)

	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{ Status int }, error) {
		return &struct{ Status int }{Status: http.StatusNoContent}, nil
	})

	api.Get("/test", "traceparent: "+traceparent)

	entry := decodeLogEntries(t, &buf)[0]
	if entry["message"] != "request completed" {
		t.Fatalf("message = %v, want request completed", entry["message"])
	}
	if entry["requestId"] != "req-123" {
		t.Fatalf("requestId = %v, want req-123", entry["requestId"])
	}
	if entry["traceId"] != "3d23d071b5bfd6579171efce907685cb" {
		t.Fatalf("traceId = %v, want trace ID", entry["traceId"])
	}
	if entry["parentId"] != "08f067aa0ba902b7" {
		t.Fatalf("parentId = %v, want parent ID", entry["parentId"])
	}
	if entry["traceFlags"] != "01" {
		t.Fatalf("traceFlags = %v, want 01", entry["traceFlags"])
	}
	if _, ok := entry["spanId"]; ok {
		t.Fatalf("AWS log should not emit spanId without an active span: %#v", entry)
	}
	if entry["service"] != "billing" {
		t.Fatalf("service = %v, want billing", entry["service"])
	}
	httpGroup := entry["http"].(map[string]any)
	response := httpGroup["response"].(map[string]any)
	if response["status_code"] != float64(http.StatusNoContent) {
		t.Fatalf("http.response.status_code = %v, want 204", response["status_code"])
	}
}

func TestAccessLoggerAzureMonitorShape(t *testing.T) {
	const traceparent = "00-3d23d071b5bfd6579171efce907685cb-08f067aa0ba902b7-01"

	var buf bytes.Buffer
	_, api := humatest.New(t)
	api.UseMiddleware(
		middleware.RequestContext(middleware.RequestContextConfig{
			NewRequestID: func() string { return "req-123" },
		}),
		middleware.AccessLogger(middleware.AccessLoggerConfig{
			Logger: middleware.NewJSONLogger(middleware.JSONLoggerConfig{
				Writer: &buf,
				Preset: middleware.LogPresetAzure,
			}),
			Preset: middleware.LogPresetAzure,
			Now:    sequenceNow(time.Unix(100, 0), time.Unix(100, int64(time.Millisecond))),
		}),
	)

	huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{ Status int }, error) {
		return &struct{ Status int }{Status: http.StatusAccepted}, nil
	})

	api.Get("/test", "traceparent: "+traceparent)

	entry := decodeLogEntries(t, &buf)[0]
	if entry["message"] != "request completed" {
		t.Fatalf("message = %v, want request completed", entry["message"])
	}
	if entry["level"] != "INFO" {
		t.Fatalf("level = %v, want INFO", entry["level"])
	}
	if entry["operationId"] != "3d23d071b5bfd6579171efce907685cb" {
		t.Fatalf("operationId = %v, want trace ID", entry["operationId"])
	}
	if entry["traceId"] != "3d23d071b5bfd6579171efce907685cb" {
		t.Fatalf("traceId = %v, want trace ID", entry["traceId"])
	}
	if entry["parentId"] != "08f067aa0ba902b7" {
		t.Fatalf("parentId = %v, want parent ID", entry["parentId"])
	}
	if entry["traceFlags"] != "01" {
		t.Fatalf("traceFlags = %v, want 01", entry["traceFlags"])
	}
	httpGroup := entry["http"].(map[string]any)
	response := httpGroup["response"].(map[string]any)
	if response["status_code"] != float64(http.StatusAccepted) {
		t.Fatalf("http.response.status_code = %v, want 202", response["status_code"])
	}
}

func TestAccessLoggerGCPCloudTraceContextSampleOption(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		sampled bool
	}{
		{
			name:    "sampled",
			header:  "3d23d071b5bfd6579171efce907685cb/123;o=1",
			sampled: true,
		},
		{
			name:   "not substring matched",
			header: "3d23d071b5bfd6579171efce907685cb/123;o=10",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			_, api := humatest.New(t)
			api.UseMiddleware(
				middleware.RequestContext(middleware.RequestContextConfig{
					NewRequestID: func() string { return "req-123" },
				}),
				middleware.AccessLogger(middleware.AccessLoggerConfig{
					Logger: middleware.NewJSONLogger(middleware.JSONLoggerConfig{
						Writer: &buf,
						Preset: middleware.LogPresetGCP,
					}),
					Preset: middleware.LogPresetGCP,
					GCP: middleware.GCPConfig{
						ProjectID: "test-project",
					},
					Now: sequenceNow(time.Unix(100, 0), time.Unix(100, int64(time.Millisecond))),
				}),
			)

			huma.Get(api, "/test", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
				return &struct{}{}, nil
			})

			api.Get("/test", "X-Cloud-Trace-Context: "+tc.header)

			entry := decodeLogEntries(t, &buf)[0]
			if entry["logging.googleapis.com/trace"] != "projects/test-project/traces/3d23d071b5bfd6579171efce907685cb" {
				t.Fatalf("trace field = %v", entry["logging.googleapis.com/trace"])
			}
			if entry["logging.googleapis.com/trace_sampled"] != tc.sampled {
				t.Fatalf("trace_sampled = %v, want %v", entry["logging.googleapis.com/trace_sampled"], tc.sampled)
			}
		})
	}
}

func TestAccessLoggerLogsPanicAndRepanics(t *testing.T) {
	var buf bytes.Buffer
	_, api := humatest.New(t)
	api.UseMiddleware(
		middleware.RequestContext(middleware.RequestContextConfig{
			NewRequestID: func() string { return "req-123" },
		}),
		middleware.AccessLogger(middleware.AccessLoggerConfig{
			Logger: middleware.NewJSONLogger(middleware.JSONLoggerConfig{Writer: &buf}),
			Now:    sequenceNow(time.Unix(100, 0), time.Unix(100, int64(time.Millisecond))),
		}),
	)

	huma.Get(api, "/panic", func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		panic("boom")
	})

	defer func() {
		if recovered := recover(); recovered != "boom" {
			t.Fatalf("recover = %v, want boom", recovered)
		}
		entry := decodeLogEntries(t, &buf)[0]
		if entry["level"] != "ERROR" {
			t.Fatalf("level = %v, want ERROR", entry["level"])
		}
		if entry["status"] != float64(http.StatusInternalServerError) {
			t.Fatalf("status = %v, want 500", entry["status"])
		}
	}()

	api.Get("/panic")
}

func decodeLogEntries(t *testing.T, reader io.Reader) []map[string]any {
	t.Helper()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode log %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func sequenceNow(times ...time.Time) func() time.Time {
	i := 0
	return func() time.Time {
		if i >= len(times) {
			return times[len(times)-1]
		}
		now := times[i]
		i++
		return now
	}
}

func TestRequestLoggerFallback(t *testing.T) {
	if middleware.RequestLogger(nil) == nil {
		t.Fatal("RequestLogger(nil) returned nil")
	}
}
