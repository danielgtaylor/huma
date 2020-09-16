package middleware

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi"
	"github.com/mattn/go-isatty"
	"github.com/opentracing/opentracing-go"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

var logContextKey contextKey = "huma-middleware-logger"
var logConfig zap.Config

// LogLevel sets the current Zap root logger's level when using the logging
// middleware. This can be changed dynamically at runtime.
var LogLevel *zap.AtomicLevel

// LogTracePrefix is used to prefix OpenTracing trace and span ID key names in
// emitted log message tag names. Use this to integrate with DataDog and other
// tracing service providers.
var LogTracePrefix = "dd."

// NewDefaultLogger returns a new low-level `*zap.Logger` instance. If the
// current terminal is a TTY, it will try ot use colored output automatically.
func NewDefaultLogger() (*zap.Logger, error) {
	if LogLevel != nil {
		// Only set up the config once. The level will control all loggers.
		return logConfig.Build()
	}

	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		config := zap.NewDevelopmentConfig()
		LogLevel = &config.Level
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.EncodeTime = iso8601UTCTimeEncoder
		return config.Build()
	}

	logConfig = zap.NewProductionConfig()
	logConfig.EncoderConfig.EncodeTime = iso8601UTCTimeEncoder
	LogLevel = &logConfig.Level
	return logConfig.Build()
}

// NewLogger is a function that returns a new logger instance to use with
// the logger middleware.
var NewLogger func() (*zap.Logger, error) = NewDefaultLogger

// A UTC variation of ZapCore.ISO8601TimeEncoder with millisecond precision
func iso8601UTCTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.UTC().Format("2006-01-02T15:04:05.000Z"))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Logger creates a new middleware to set a tagged `*zap.SugarLogger` in the
// request context. It debug logs request info. If the current terminal is a
// TTY, it will try to use colored output automatically.
func Logger(next http.Handler) http.Handler {
	var err error
	var l *zap.Logger
	if l, err = NewLogger(); err != nil {
		panic(err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		chiCtx := chi.RouteContext(r.Context())

		contextLog := l.With(
			zap.String("http.version", r.Proto),
			zap.String("http.method", r.Method),
			// The route pattern isn't filled out until *after* the handler runs...
			// zap.String("http.template", chiCtx.RoutePattern()),
			zap.String("http.url", r.URL.String()),
			zap.String("network.client.ip", r.RemoteAddr),
		)

		if span := opentracing.SpanFromContext(r.Context()); span != nil {
			// We have a span context, so log its info to help with correlation.
			if sc, ok := span.Context().(spanContext); ok {
				contextLog = contextLog.With(
					zap.Uint64(LogTracePrefix+"trace_id", sc.TraceID()),
					zap.Uint64(LogTracePrefix+"span_id", sc.SpanID()),
				)
			}
		}

		r = r.WithContext(context.WithValue(r.Context(), logContextKey, contextLog.Sugar()))
		nw := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(nw, r)

		contextLog = contextLog.With(
			zap.String("http.template", chiCtx.RoutePattern()),
			zap.Int("http.status_code", nw.status),
			zap.Duration("duration", time.Since(start)),
		)

		if nw.status < 500 {
			contextLog.Debug("Request")
		} else {
			contextLog.Error("Request")
		}
	})
}

// AddLoggerOptions adds command line options for enabling debug logging.
func AddLoggerOptions(app Flagger) {
	// Add the debug flag to enable more logging
	app.Flag("debug", "d", "Enable debug logs", false)

	// Add pre-start handler
	app.PreStart(func() {
		if viper.GetBool("debug") {
			if LogLevel != nil {
				LogLevel.SetLevel(zapcore.DebugLevel)
			}
		}
	})
}

// GetLogger returns the contextual logger for the current request. If no
// logger is present, it returns a no-op logger so no nil check is required.
func GetLogger(ctx context.Context) *zap.SugaredLogger {
	log := ctx.Value(logContextKey)
	if log != nil {
		return log.(*zap.SugaredLogger)
	}

	return zap.NewNop().Sugar()
}
