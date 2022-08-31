package huma

import (
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/schema"
	"github.com/stretchr/testify/assert"
)

type componentFoo struct {
	Field   string `json:"field"`
	Another string `json:"another" readOnly:"true"`
}

type componentBar struct {
	Field string `json:"field"`
}

func TestComponentSchemas(t *testing.T) {
	components := oaComponents{
		Schemas: map[string]*schema.Schema{},
	}

	// Adding two different versions of the same component.
	ref := components.AddSchema(reflect.TypeOf(&componentFoo{}), schema.ModeRead, "hint", true)
	assert.Equal(t, ref, "#/components/schemas/componentFoo")
	assert.NotNil(t, components.Schemas["componentFoo"])

	ref = components.AddSchema(reflect.TypeOf(&componentFoo{}), schema.ModeWrite, "hint", true)
	assert.Equal(t, ref, "#/components/schemas/componentFoo2")
	assert.NotNil(t, components.Schemas["componentFoo2"])

	// Re-adding the second should not create a third.
	ref = components.AddSchema(reflect.TypeOf(&componentFoo{}), schema.ModeWrite, "hint", true)
	assert.Equal(t, ref, "#/components/schemas/componentFoo2")
	assert.Nil(t, components.Schemas["componentFoo3"])

	// Adding a list of pointers to a struct.
	ref = components.AddSchema(reflect.TypeOf([]*componentBar{}), schema.ModeAll, "hint", true)
	assert.Equal(t, ref, "#/components/schemas/componentBarList")
	assert.NotNil(t, components.Schemas["componentBarList"])

	// Adding an anonymous empty struct, should use the hint.
	ref = components.AddSchema(reflect.TypeOf(struct{}{}), schema.ModeAll, "hint", true)
	assert.Equal(t, ref, "#/components/schemas/hint")
	assert.NotNil(t, components.Schemas["hint"])
}
