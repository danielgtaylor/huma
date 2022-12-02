package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma"
	"github.com/stretchr/testify/assert"
)

func TestNewLogger(t *testing.T) {
	// Make sure it returns a logger
	l, err := NewDefaultLogger()
	assert.NoError(t, err)
	assert.NotNil(t, l)
}

func TestSetLoggerInContext(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	w := httptest.NewRecorder()
	ctx := huma.ContextFromRequest(nil, w, r)

	logger := GetLogger(ctx)

	logger = logger.With("my-value", 123)

	SetLoggerInContext(ctx, logger)
	updated := GetLogger(ctx)

	assert.Equal(t, logger, updated)
}
