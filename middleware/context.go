package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"sync/atomic"

	"github.com/danielgtaylor/huma/v2"
)

const (
	defaultRequestIDHeader  = "X-Request-Id"
	defaultTraceparent      = "traceparent"
	defaultTracestate       = "tracestate"
	defaultRequestIDMaxSize = 128
)

type requestInfoKey struct{}

var fallbackRequestID atomic.Uint64

// RequestContextConfig configures the request context middleware.
type RequestContextConfig struct {
	// RequestIDHeader is the request header used to accept an existing request
	// ID. The default is "X-Request-Id".
	RequestIDHeader string

	// TraceparentHeader is the request header used for W3C Trace Context. The
	// default is "traceparent".
	TraceparentHeader string

	// TracestateHeader is the request header used for W3C Trace Context state.
	// The default is "tracestate".
	TracestateHeader string

	// ResponseRequestIDHeader is the response header used to echo the request ID.
	// The default is the configured RequestIDHeader.
	ResponseRequestIDHeader string

	// DisableResponseRequestIDHeader disables echoing the request ID in the
	// response.
	DisableResponseRequestIDHeader bool

	// NewRequestID creates a request ID when the incoming request ID is missing
	// or invalid. The default uses crypto/rand and returns 16 lowercase hex
	// encoded random bytes.
	NewRequestID func() string
}

type requestInfo struct {
	RequestID     string
	CorrelationID string
	Trace         TraceContext
}

// RequestContext returns middleware that parses request correlation data and
// stores it on the underlying context.Context.
func RequestContext(config RequestContextConfig) func(huma.Context, func(huma.Context)) {
	config = withRequestContextDefaults(config)

	return func(ctx huma.Context, next func(huma.Context)) {
		requestID := ctx.Header(config.RequestIDHeader)
		if !validRequestID(requestID) {
			requestID = newRequestID(config.NewRequestID)
		}

		trace := ParseTraceparent(ctx.Header(config.TraceparentHeader))
		if trace.Valid {
			trace.Tracestate = ctx.Header(config.TracestateHeader)
		}

		correlationID := requestID
		if trace.Valid {
			correlationID = trace.TraceID
		}

		info := requestInfo{
			RequestID:     requestID,
			CorrelationID: correlationID,
			Trace:         trace,
		}

		if !config.DisableResponseRequestIDHeader {
			ctx.SetHeader(config.ResponseRequestIDHeader, requestID)
		}

		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), requestInfoKey{}, info)))
	}
}

// RequestID returns the request ID stored in ctx, if any.
func RequestID(ctx context.Context) string {
	return requestContextInfo(ctx).RequestID
}

// CorrelationID returns the correlation ID stored in ctx, if any.
func CorrelationID(ctx context.Context) string {
	return requestContextInfo(ctx).CorrelationID
}

// Trace returns the W3C Trace Context stored in ctx, if any.
func Trace(ctx context.Context) TraceContext {
	return requestContextInfo(ctx).Trace
}

func requestContextInfo(ctx context.Context) requestInfo {
	if ctx == nil {
		return requestInfo{}
	}
	info, _ := ctx.Value(requestInfoKey{}).(requestInfo)
	return info
}

func withRequestContextDefaults(config RequestContextConfig) RequestContextConfig {
	if config.RequestIDHeader == "" {
		config.RequestIDHeader = defaultRequestIDHeader
	}
	if config.TraceparentHeader == "" {
		config.TraceparentHeader = defaultTraceparent
	}
	if config.TracestateHeader == "" {
		config.TracestateHeader = defaultTracestate
	}
	if config.ResponseRequestIDHeader == "" {
		config.ResponseRequestIDHeader = config.RequestIDHeader
	}
	return config
}

func newRequestID(fn func() string) string {
	if fn != nil {
		if id := fn(); validRequestID(id) {
			return id
		}
	}
	return randomRequestID()
}

func randomRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return "fallback-" + strconv.FormatUint(fallbackRequestID.Add(1), 16)
}

func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > defaultRequestIDMaxSize {
		return false
	}
	for i := range len(id) {
		switch c := id[i]; {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.' || c == ':' || c == '/' || c == '=' || c == '+':
		default:
			return false
		}
	}
	return true
}
