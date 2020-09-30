package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLogger(t *testing.T) {
	// Make sure it returns a logger
	l, err := NewDefaultLogger()
	assert.NoError(t, err)
	assert.NotNil(t, l)
}
