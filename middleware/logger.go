package middleware

import (
	"context"
	"io"
	"log/slog"
	"os"

	"github.com/danielgtaylor/huma/v2"
)

type loggerKey struct{}

// LogPreset configures JSON and access-log field names for common log
// aggregation targets.
type LogPreset string

const (
	// LogPresetDefault uses slog's standard JSON field names.
	LogPresetDefault LogPreset = ""

	// LogPresetGCP uses Google Cloud Logging field names.
	LogPresetGCP LogPreset = "gcp"

	// LogPresetAWS uses AWS CloudWatch-friendly field names.
	LogPresetAWS LogPreset = "aws"

	// LogPresetAzure uses Azure Monitor-friendly field names.
	LogPresetAzure LogPreset = "azure"
)

// JSONLoggerConfig configures NewJSONLogger.
type JSONLoggerConfig struct {
	// Writer receives JSON log lines. The default is os.Stdout.
	Writer io.Writer

	// Level is the minimum enabled log level.
	Level slog.Leveler

	// Preset configures built-in JSON field names for a log aggregation target.
	// Use the same value for AccessLoggerConfig.Preset when composing
	// NewJSONLogger with AccessLogger.
	Preset LogPreset

	// ReplaceAttr optionally rewrites attributes after the preset mapping has
	// been applied.
	ReplaceAttr func(groups []string, attr slog.Attr) slog.Attr
}

// NewJSONLogger creates a slog JSON logger with optional field-name presets.
func NewJSONLogger(config JSONLoggerConfig) *slog.Logger {
	writer := config.Writer
	if writer == nil {
		writer = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level: config.Level,
	}
	if config.Preset != LogPresetDefault || config.ReplaceAttr != nil {
		opts.ReplaceAttr = func(groups []string, attr slog.Attr) slog.Attr {
			attr = replacePresetAttr(config.Preset, groups, attr)
			if config.ReplaceAttr != nil {
				attr = config.ReplaceAttr(groups, attr)
			}
			return attr
		}
	}

	return slog.New(slog.NewJSONHandler(writer, opts))
}

// RequestLogger returns the request-scoped logger stored in ctx, or
// slog.Default().
func RequestLogger(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return slog.Default()
	}
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}
	return slog.Default()
}

func withLogger(ctx huma.Context, logger *slog.Logger) huma.Context {
	if logger == nil {
		logger = slog.Default()
	}
	return huma.WithContext(ctx, context.WithValue(ctx.Context(), loggerKey{}, logger))
}

func replacePresetAttr(preset LogPreset, groups []string, attr slog.Attr) slog.Attr {
	if len(groups) != 0 {
		return attr
	}

	switch preset {
	case LogPresetGCP:
		switch attr.Key {
		case slog.LevelKey:
			attr.Key = "severity"
			attr.Value = slog.StringValue(gcpSeverity(attr.Value))
		case slog.MessageKey:
			attr.Key = "message"
		case slog.TimeKey:
			attr.Key = "timestamp"
		}
	case LogPresetAWS:
		switch attr.Key {
		case slog.LevelKey:
			attr.Value = slog.StringValue(levelString(attr.Value))
		case slog.MessageKey:
			attr.Key = "message"
		case slog.TimeKey:
			attr.Key = "timestamp"
		}
	case LogPresetAzure:
		switch attr.Key {
		case slog.LevelKey:
			attr.Value = slog.StringValue(azureLevel(attr.Value))
		case slog.MessageKey:
			attr.Key = "message"
		case slog.TimeKey:
			attr.Key = "timestamp"
		}
	}

	return attr
}

func gcpSeverity(value slog.Value) string {
	level, ok := value.Any().(slog.Level)
	if !ok {
		return value.String()
	}
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARNING"
	case level >= slog.LevelInfo:
		return "INFO"
	case level >= slog.LevelDebug:
		return "DEBUG"
	default:
		return "DEFAULT"
	}
}

func levelString(value slog.Value) string {
	level, ok := value.Any().(slog.Level)
	if !ok {
		return value.String()
	}
	return level.String()
}

func azureLevel(value slog.Value) string {
	level, ok := value.Any().(slog.Level)
	if !ok {
		return value.String()
	}
	switch {
	case level >= slog.LevelError+4:
		return "CRITICAL"
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARNING"
	case level >= slog.LevelInfo:
		return "INFO"
	case level >= slog.LevelDebug:
		return "DEBUG"
	default:
		return "TRACE"
	}
}
