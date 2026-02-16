package huma

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type Output[T any] struct{}

type Embedded[P any] struct{}

type EmbeddedTwo[P, V any] struct{}

type S struct{}

type ü struct{}

type MP4 struct{}

func TestDefaultSchemaNamer(t *testing.T) {
	type Renamed Output[*[]Embedded[time.Time]]

	for _, example := range []struct {
		typ  any
		name string
		hint string
	}{
		{int(0), "Int", ""},
		{int64(0), "Int64", ""},
		{S{}, "S", ""},
		{time.Time{}, "Time", ""},
		{Output[int]{}, "OutputInt", ""},
		{Output[*int]{}, "OutputInt", ""},
		{Output[[]int]{}, "OutputListInt", ""},
		{Output[[]*int]{}, "OutputListInt", ""},
		{Output[[][]int]{}, "OutputListListInt", ""},
		{Output[map[string]int]{}, "OutputMapStringInt", ""},
		{Output[map[string][]*int]{}, "OutputMapStringListInt", ""},
		{Output[S]{}, "OutputS", ""},
		{Output[ü]{}, "OutputÜ", ""},
		{Output[MP4]{}, "OutputMP4", ""},
		{Output[Embedded[*time.Time]]{}, "OutputEmbeddedTime", ""},
		{Output[*[]Embedded[time.Time]]{}, "OutputListEmbeddedTime", ""},
		{Output[EmbeddedTwo[[]time.Time, **url.URL]]{}, "OutputEmbeddedTwoListTimeURL", ""},
		{Renamed{}, "Renamed", ""},
		{struct{}{}, "SomeGenericThing", "Some[pkg.Generic]Thing"},
		{struct{}{}, "Type1Type2Type3", "pkg1.Type1[path/to/pkg2.Type2]pkg3.Type3"},
	} {
		t.Run(example.name, func(t *testing.T) {
			name := DefaultSchemaNamer(reflect.TypeOf(example.typ), example.hint)
			assert.Equal(t, example.name, name)
		})
	}
}

func TestSchemaAlias(t *testing.T) {
	type StringContainer struct {
		Value string
	}
	type StructWithStringContainer struct {
		Name StringContainer `json:"name,omitzero"`
	}
	type StructWithString struct {
		Name string `json:"name,omitempty"`
	}
	registry := NewMapRegistry("#/components/schemas", DefaultSchemaNamer)
	registry.RegisterTypeAlias(reflect.TypeFor[StringContainer](), reflect.TypeFor[string]())
	schemaWithContainer := registry.Schema(reflect.TypeFor[StructWithStringContainer](), false, "")
	schemaWithString := registry.Schema(reflect.TypeFor[StructWithString](), false, "")
	assert.Equal(t, schemaWithString, schemaWithContainer)
}

func TestAllowAdditionalPropertiesByDefault(t *testing.T) {
	type MyStruct struct {
		Name string `json:"name"`
	}

	t.Run("DefaultIsFalse", func(t *testing.T) {
		r := NewMapRegistry("/schemas", DefaultSchemaNamer)

		// Confirm default is false.
		assert.False(t, r.Config().AllowAdditionalPropertiesByDefault)

		s := r.Schema(reflect.TypeFor[MyStruct](), false, "")
		assert.Equal(t, false, s.AdditionalProperties)
	})

	t.Run("OverrideViaRegistryConfig", func(t *testing.T) {
		r := NewMapRegistry("/schemas", DefaultSchemaNamer).(*mapRegistry)
		r.config.AllowAdditionalPropertiesByDefault = true

		s := r.Schema(reflect.TypeFor[MyStruct](), false, "")
		assert.Equal(t, true, s.AdditionalProperties)
	})
}

func TestRegistryConfigValue(t *testing.T) {
	r := NewMapRegistry("/schemas", DefaultSchemaNamer)

	cfg := r.Config()
	cfg.AllowAdditionalPropertiesByDefault = true

	// Config() returns a copy, so modifying it shouldn't affect the registry.
	assert.False(t, r.Config().AllowAdditionalPropertiesByDefault)
}
