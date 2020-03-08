package huma

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathParam(t *testing.T) {
	p := PathParam("test", "desc")
	assert.Equal(t, "test", p.Name)
	assert.Equal(t, "desc", p.Description)
	assert.Equal(t, "path", p.In)
	assert.Equal(t, true, p.Required)
}

func TestQueryParam(t *testing.T) {
	p := QueryParam("test", "desc")
	assert.Equal(t, "test", p.Name)
	assert.Equal(t, "desc", p.Description)
	assert.Equal(t, "query", p.In)
	assert.Equal(t, false, p.Required)
}

func TestHeaderParam(t *testing.T) {
	p := HeaderParam("test", "desc")
	assert.Equal(t, "test", p.Name)
	assert.Equal(t, "desc", p.Description)
	assert.Equal(t, "header", p.In)
	assert.Equal(t, false, p.Required)
}
