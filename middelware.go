package huma

import (
	"fmt"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logLevel *zap.AtomicLevel

// NewLogger returns a new low-level `*zap.Logger` instance. If the current
// terminal is a TTY, it will try ot use colored output automatically.
func NewLogger() (*zap.Logger, error) {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		config := zap.NewDevelopmentConfig()
		logLevel = &config.Level
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		return config.Build()
	}

	config := zap.NewProductionConfig()
	logLevel = &config.Level
	return config.Build()
}

// LogMiddleware creates a new middleware to set a tagged `*zap.SugarLogger` in the
// Gin context under the `log` key. It debug logs request info. If passed `nil`
// for the logger, then it creates one. If the current terminal is a TTY, it
// will try to use colored output automatically.
func LogMiddleware(l *zap.Logger, tags map[string]string) func(*gin.Context) {
	var err error
	if l == nil {
		if l, err = NewLogger(); err != nil {
			panic(err)
		}
	}

	// Add any additional tags that were passed.
	for k, v := range tags {
		l = l.With(zap.String(k, v))
	}

	return func(c *gin.Context) {
		start := time.Now()
		contextLog := l.With(
			zap.String("method", c.Request.Method),
			zap.String("template", c.FullPath()),
			zap.String("path", c.Request.URL.RequestURI()),
			zap.String("ip", c.ClientIP()),
		)
		c.Set("log", contextLog.Sugar())

		c.Next()

		contextLog.Debug("Request",
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", time.Since(start)),
		)
	}
}

// LogDependency returns a dependency that resolves to a `*zap.SugaredLogger`
// for the current request. This dependency *requires* the use of
// `LogMiddleware` and will error if the logger is not in the request context.
func LogDependency() *Dependency {
	return &Dependency{
		Dependencies: []*Dependency{ContextDependency(), OperationDependency()},
		Value: func(c *gin.Context, op *Operation) (*zap.SugaredLogger, error) {
			l, ok := c.Get("log")
			if !ok {
				return nil, fmt.Errorf("missing logger in context")
			}
			sl := l.(*zap.SugaredLogger).With("operation", op.ID)
			sl.Desugar()
			return sl, nil
		},
	}
}
