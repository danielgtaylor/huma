package huma

import (
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestBlankConfig(t *testing.T) {
	adapter := &testAdapter{chi.NewMux()}

	assert.NotPanics(t, func() {
		NewAPI(Config{}, adapter)
	})
}
