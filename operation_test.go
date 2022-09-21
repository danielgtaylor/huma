package huma

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOperation(t *testing.T) {
	op := Operation{}

	assert.Equal(t, op.toOpenAPI(&oaComponents{}).Data(), map[string]interface{}{
		"operationId": "",
	})
}

func TestDeprecatedOperation(t *testing.T) {
	op := Operation{}
	op.Deprecated()

	assert.Contains(t, op.toOpenAPI(&oaComponents{}).Data(), "deprecated")
}
