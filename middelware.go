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

// LogMiddleware creates a new middleware to set a tagged `*zap.SugarLogger` in the
// Gin context under the `log` key. It debug logs request info. If the current
// terminal is a TTY, it will try to use colored output automatically.
func LogMiddleware() func(*gin.Context) {
	var l *zap.Logger
	var err error
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		l, err = config.Build()
	} else {
		l, err = zap.NewProduction()
	}
	if err != nil {
		panic(err)
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
