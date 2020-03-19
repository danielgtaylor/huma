package huma

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var types = []struct {
	in     interface{}
	out    string
	format string
}{
	{false, "boolean", ""},
	{0, "integer", ""},
	{0.0, "number", ""},
	{"hello", "string", ""},
	{struct{}{}, "object", ""},
	{[]string{"foo"}, "array", ""},
	{net.IP{}, "string", "ipv4"},
	{time.Time{}, "string", "date-time"},
	{url.URL{}, "string", "uri"},
	// TODO: map
}

func TestSchemaTypes(outer *testing.T) {
	outer.Parallel()
	for _, tt := range types {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.in), func(t *testing.T) {
			t.Parallel()
			s, err := GenerateSchema(reflect.ValueOf(local.in).Type())
			assert.NoError(t, err)
			assert.Equal(t, local.out, s.Type)
			assert.Equal(t, local.format, s.Format)
		})
	}
}

func TestSchemaRequiredFields(t *testing.T) {
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

func TestSchemaRenameField(t *testing.T) {
	type Example struct {
		Foo string `json:"bar"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Empty(t, s.Properties["foo"])
	assert.NotEmpty(t, s.Properties["bar"])
}

func TestSchemaDescription(t *testing.T) {
	type Example struct {
		Foo string `json:"foo" description:"I am a test"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Equal(t, "I am a test", s.Properties["foo"].Description)
}

func TestSchemaFormat(t *testing.T) {
	type Example struct {
		Foo string `json:"foo" format:"date-time"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Equal(t, "date-time", s.Properties["foo"].Format)
}

func TestSchemaEnum(t *testing.T) {
	type Example struct {
		Foo string `json:"foo" enum:"one,two,three"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Equal(t, []interface{}{"one", "two", "three"}, s.Properties["foo"].Enum)
}

func TestSchemaDefault(t *testing.T) {
	type Example struct {
		Foo string `json:"foo" default:"def"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Equal(t, "def", s.Properties["foo"].Default)
}

func TestSchemaExample(t *testing.T) {
	type Example struct {
		Foo string `json:"foo" example:"ex"`
	}

	s, err := GenerateSchema(reflect.ValueOf(Example{}).Type())
	assert.NoError(t, err)
	assert.Equal(t, "ex", s.Properties["foo"].Example)
}
