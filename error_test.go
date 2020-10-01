package huma

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorDetailAsError(t *testing.T) {
	d := ErrorDetail{
		Message:  "foo",
		Location: "bar",
		Value:    "baz",
	}

	rendered := d.Error()
	assert.Contains(t, rendered, "foo")
	assert.Contains(t, rendered, "bar")
	assert.Contains(t, rendered, "baz")
}
