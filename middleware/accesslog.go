package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// StatusLeveler maps HTTP response statuses to log levels.
type StatusLeveler func(status int) slog.Level

// AccessLoggerConfig configures AccessLogger.
type AccessLoggerConfig struct {
	// Logger is the base logger used for request and access logs. The default is
	// slog.Default().
	Logger *slog.Logger

	// Preset configures built-in field names for a log aggregation target.
	Preset LogPreset

	// GCP configures Google Cloud Logging fields.
	GCP GCPConfig

	// Now returns the current time. The default is time.Now.
	Now func() time.Time

	// StatusLevel maps response statuses to log levels. The default maps 5xx to
	// error, 4xx to warn, and all others to info.
	StatusLevel StatusLeveler

	// ExtraAttrs returns additional access-log attributes.
	ExtraAttrs func(huma.Context) []slog.Attr
}

// GCPConfig configures Google Cloud Logging fields.
type GCPConfig struct {
	// ProjectID is the Google Cloud project ID used to format Cloud Trace links.
	ProjectID string

	// CloudTraceContextHeader is the optional provider-specific request header
	// used by Cloud Run for request-log nesting. The default is
	// "X-Cloud-Trace-Context" when LogPresetGCP is selected.
	CloudTraceContextHeader string
}

// AccessLogger returns middleware that stores a request-scoped logger and emits
// one structured access log after the operation completes.
func AccessLogger(config AccessLoggerConfig) func(huma.Context, func(huma.Context)) {
	config = withAccessLoggerDefaults(config)

	return func(ctx huma.Context, next func(huma.Context)) {
		start := config.Now()
		logger := requestLogger(ctx, config)
		ctx = withLogger(ctx, logger)

		defer func() {
			if recovered := recover(); recovered != nil {
				writeAccessLog(ctx, logger, config, start, httpStatusInternalServerError)
				panic(recovered)
			}
		}()

		next(ctx)
		writeAccessLog(ctx, logger, config, start, statusOrDefault(ctx.Status(), httpStatusOK))
	}
}

func withAccessLoggerDefaults(config AccessLoggerConfig) AccessLoggerConfig {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.StatusLevel == nil {
		config.StatusLevel = defaultStatusLevel
	}
	if config.Preset == LogPresetGCP && config.GCP.CloudTraceContextHeader == "" {
		config.GCP.CloudTraceContextHeader = "X-Cloud-Trace-Context"
	}
	return config
}

func requestLogger(ctx huma.Context, config AccessLoggerConfig) *slog.Logger {
	attrs := loggerRequestAttrs(ctx, config)
	if len(attrs) == 0 {
		return config.Logger
	}
	args := make([]any, len(attrs))
	for i := range attrs {
		args[i] = attrs[i]
	}
	return config.Logger.With(args...)
}

func loggerRequestAttrs(ctx huma.Context, config AccessLoggerConfig) []slog.Attr {
	info := requestInfo(ctx.Context())
	switch config.Preset {
	case LogPresetGCP:
		return gcpRequestAttrs(ctx, config, info)
	case LogPresetAWS:
		return awsRequestAttrs(info)
	default:
		return genericRequestAttrs(info)
	}
}

func writeAccessLog(ctx huma.Context, logger *slog.Logger, config AccessLoggerConfig, start time.Time, status int) {
	attrs := accessAttrs(ctx, config, status, config.Now().Sub(start))
	logger.LogAttrs(ctx.Context(), config.StatusLevel(status), "request completed", attrs...)
}

func accessAttrs(ctx huma.Context, config AccessLoggerConfig, status int, duration time.Duration) []slog.Attr {
	info := requestInfo(ctx.Context())
	switch config.Preset {
	case LogPresetGCP:
		return gcpAccessAttrs(ctx, config, info, status, duration)
	case LogPresetAWS:
		return awsAccessAttrs(ctx, info, status, duration, config.ExtraAttrs)
	default:
		return genericAccessAttrs(ctx, info, status, duration, config.ExtraAttrs)
	}
}

func genericAccessAttrs(ctx huma.Context, info RequestInfo, status int, duration time.Duration, extra func(huma.Context) []slog.Attr) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("method", ctx.Method()),
		slog.Int("status", status),
		slog.Float64("duration_ms", durationMilliseconds(duration)),
	}
	if op := ctx.Operation(); op != nil {
		attrs = append(attrs, slog.String("path_template", op.Path))
		if op.OperationID != "" {
			attrs = append(attrs, slog.String("operation_id", op.OperationID))
		}
	}
	attrs = append(attrs, genericRequestAttrs(info)...)
	if extra != nil {
		attrs = append(attrs, extra(ctx)...)
	}
	return attrs
}

func gcpAccessAttrs(ctx huma.Context, config AccessLoggerConfig, info RequestInfo, status int, duration time.Duration) []slog.Attr {
	attrs := []slog.Attr{
		slog.Group("httpRequest",
			slog.String("requestMethod", ctx.Method()),
			slog.String("requestUrl", ctx.URL().Path),
			slog.Int("status", status),
			slog.String("userAgent", ctx.Header("User-Agent")),
			slog.String("remoteIp", ctx.RemoteAddr()),
		),
		slog.Float64("duration_ms", durationMilliseconds(duration)),
	}
	if op := ctx.Operation(); op != nil {
		attrs = append(attrs, slog.String("pathTemplate", op.Path))
		if op.OperationID != "" {
			attrs = append(attrs, slog.String("operationId", op.OperationID))
		}
	}
	attrs = append(attrs, gcpRequestAttrs(ctx, config, info)...)
	if config.ExtraAttrs != nil {
		attrs = append(attrs, config.ExtraAttrs(ctx)...)
	}
	return attrs
}

func awsAccessAttrs(ctx huma.Context, info RequestInfo, status int, duration time.Duration, extra func(huma.Context) []slog.Attr) []slog.Attr {
	route := ""
	if op := ctx.Operation(); op != nil {
		route = op.Path
	}
	attrs := []slog.Attr{
		slog.Group("http",
			slog.Group("request",
				slog.String("method", ctx.Method()),
			),
			slog.String("route", route),
			slog.Group("response",
				slog.Int("status_code", status),
			),
		),
		slog.Float64("duration_ms", durationMilliseconds(duration)),
	}
	attrs = append(attrs, awsRequestAttrs(info)...)
	if extra != nil {
		attrs = append(attrs, extra(ctx)...)
	}
	return attrs
}

func genericRequestAttrs(info RequestInfo) []slog.Attr {
	var attrs []slog.Attr
	if info.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", info.RequestID))
	}
	if info.CorrelationID != "" {
		attrs = append(attrs, slog.String("correlation_id", info.CorrelationID))
	}
	if info.Trace.Valid {
		attrs = append(attrs,
			slog.String("trace_id", info.Trace.TraceID),
			slog.String("span_id", info.Trace.ParentID),
			slog.Bool("sampled", info.Trace.Sampled),
		)
	}
	return attrs
}

func gcpRequestAttrs(ctx huma.Context, config AccessLoggerConfig, info RequestInfo) []slog.Attr {
	var attrs []slog.Attr
	if info.RequestID != "" {
		attrs = append(attrs, slog.String("requestId", info.RequestID))
	}
	if info.CorrelationID != "" {
		attrs = append(attrs, slog.String("correlationId", info.CorrelationID))
	}

	traceID := ""
	spanID := ""
	sampled := false
	if info.Trace.Valid {
		traceID = info.Trace.TraceID
		spanID = info.Trace.ParentID
		sampled = info.Trace.Sampled
	} else if config.GCP.CloudTraceContextHeader != "" {
		trace := parseCloudTraceContext(ctx.Header(config.GCP.CloudTraceContextHeader))
		if trace.Valid {
			traceID = trace.TraceID
			spanID = trace.SpanID
			sampled = trace.Sampled
		}
	}

	if traceID == "" {
		return attrs
	}
	if config.GCP.ProjectID == "" {
		return append(attrs, slog.String("traceId", traceID))
	}
	attrs = append(attrs, slog.String("logging.googleapis.com/trace", "projects/"+config.GCP.ProjectID+"/traces/"+traceID))
	if spanID != "" {
		attrs = append(attrs, slog.String("logging.googleapis.com/spanId", spanID))
	}
	return append(attrs, slog.Bool("logging.googleapis.com/trace_sampled", sampled))
}

func awsRequestAttrs(info RequestInfo) []slog.Attr {
	var attrs []slog.Attr
	if info.RequestID != "" {
		attrs = append(attrs, slog.String("requestId", info.RequestID))
	}
	if info.CorrelationID != "" {
		attrs = append(attrs, slog.String("correlationId", info.CorrelationID))
	}
	if info.Trace.Valid {
		attrs = append(attrs,
			slog.String("traceId", info.Trace.TraceID),
			slog.String("spanId", info.Trace.ParentID),
			slog.Bool("sampled", info.Trace.Sampled),
		)
	}
	return attrs
}

func defaultStatusLevel(status int) slog.Level {
	switch {
	case status >= httpStatusInternalServerError:
		return slog.LevelError
	case status >= httpStatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func statusOrDefault(status, defaultStatus int) int {
	if status == 0 {
		return defaultStatus
	}
	return status
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

const (
	httpStatusOK                  = 200
	httpStatusBadRequest          = 400
	httpStatusInternalServerError = 500
)

type cloudTraceContext struct {
	TraceID string
	SpanID  string
	Sampled bool
	Valid   bool
}

func parseCloudTraceContext(header string) cloudTraceContext {
	if header == "" {
		return cloudTraceContext{}
	}
	tracePart, options, _ := strings.Cut(header, ";")
	traceID, spanID, ok := strings.Cut(tracePart, "/")
	if !ok {
		traceID = tracePart
	}
	if len(traceID) != traceIDSize || !isLowerHex(traceID) || allZero(traceID) {
		return cloudTraceContext{}
	}
	if spanID != "" && !allDigits(spanID) {
		return cloudTraceContext{}
	}
	return cloudTraceContext{
		TraceID: traceID,
		SpanID:  spanID,
		Sampled: strings.Contains(options, "o=1"),
		Valid:   true,
	}
}

func allDigits(value string) bool {
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
