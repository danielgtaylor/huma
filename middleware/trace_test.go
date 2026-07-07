package middleware_test

import (
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2/middleware"
)

func TestParseTraceparent(t *testing.T) {
	const validTraceID = "3d23d071b5bfd6579171efce907685cb"
	const validParentID = "08f067aa0ba902b7"

	tests := []struct {
		name    string
		header  string
		valid   bool
		sampled bool
	}{
		{
			name:    "valid sampled",
			header:  "00-" + validTraceID + "-" + validParentID + "-01",
			valid:   true,
			sampled: true,
		},
		{
			name:    "sampled bit mask",
			header:  "00-" + validTraceID + "-" + validParentID + "-03",
			valid:   true,
			sampled: true,
		},
		{
			name:   "future version with extra data",
			header: "01-" + validTraceID + "-" + validParentID + "-00-extra",
			valid:  true,
		},
		{
			name:   "all-zero trace ID",
			header: "00-00000000000000000000000000000000-" + validParentID + "-01",
		},
		{
			name:   "all-zero parent ID",
			header: "00-" + validTraceID + "-0000000000000000-01",
		},
		{
			name:   "invalid version ff",
			header: "ff-" + validTraceID + "-" + validParentID + "-01",
		},
		{
			name:   "uppercase rejected",
			header: "00-3D23d071b5bfd6579171efce907685cb-" + validParentID + "-01",
		},
		{
			name:   "version 00 must not have extra data",
			header: "00-" + validTraceID + "-" + validParentID + "-01-extra",
		},
		{
			name:   "bad delimiter",
			header: "00_" + validTraceID + "-" + validParentID + "-01",
		},
		{
			name:   "too long",
			header: strings.Repeat("a", 513),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trace := middleware.ParseTraceparent(tc.header)
			if trace.Valid != tc.valid {
				t.Fatalf("Valid = %v, want %v", trace.Valid, tc.valid)
			}
			if !tc.valid {
				return
			}
			if trace.TraceID != validTraceID {
				t.Fatalf("TraceID = %q, want %q", trace.TraceID, validTraceID)
			}
			if trace.ParentID != validParentID {
				t.Fatalf("ParentID = %q, want %q", trace.ParentID, validParentID)
			}
			if trace.Sampled != tc.sampled {
				t.Fatalf("Sampled = %v, want %v", trace.Sampled, tc.sampled)
			}
			if trace.Traceparent != tc.header {
				t.Fatalf("Traceparent = %q, want original header", trace.Traceparent)
			}
		})
	}
}
