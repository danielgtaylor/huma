package huma

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

var types = []struct {
	in  interface{}
	out string
}{
	{false, "boolean"},
	{0, "integer"},
	{0.0, "number"},
	{"hello", "string"},
	{struct{}{}, "object"},
	{[]string{"foo"}, "array"},
	// TODO: map
}

func TestTypes(outer *testing.T) {
	outer.Parallel()
	for _, tt := range types {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.in), func(t *testing.T) {
			t.Parallel()
			s, err := GenerateSchema(reflect.ValueOf(local.in).Type())
			assert.NoError(t, err)
			assert.Equal(t, local.out, s.Type)
		})
	}
}

func TestRequiredFields(t *testing.T) {
	type Example struct {
		Optional string `json:"optional,omitempty"`
		Required string `json:"required"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Len(t, s.Properties, 2)
	assert.NotContains(t, s.Required, "optional")
	assert.Contains(t, s.Required, "required")
}

func TestRenameField(t *testing.T) {
	type Example struct {
		Foo string `json:"bar"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Empty(t, s.Properties["foo"])
	assert.NotEmpty(t, s.Properties["bar"])
}

func TestDescription(t *testing.T) {
	type Example struct {
		Foo string `json:"foo" description:"I am a test"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Equal(t, "I am a test", s.Properties["foo"].Description)
}
