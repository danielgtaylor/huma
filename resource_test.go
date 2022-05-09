package huma

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResource(t *testing.T) {
	r := Resource{
		path: "/test",
	}

	assert.NotNil(t, r.toOpenAPI(&oaComponents{}))
}

func TestHiddenResource(t *testing.T) {
	r := Resource{
		path: "/test",
	}
	r.Hidden()

	assert.Nil(t, r.toOpenAPI(&oaComponents{}))
}
