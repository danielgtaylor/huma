package huma_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
)

func TestValidateUnresolvedSchemaRefNoPanic(t *testing.T) {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	pb := huma.NewPathBuffer([]byte{}, 0)
	res := &huma.ValidateResult{}

	assertUnresolved := func(t *testing.T, fn func()) {
		t.Helper()
		res.Reset()
		require.NotPanics(t, fn)
		require.NotEmpty(t, res.Errors)
		require.Contains(t, res.Errors[0].Error(), "expected schema $ref to resolve")
	}

	t.Run("top-level ref", func(t *testing.T) {
		assertUnresolved(t, func() {
			huma.Validate(registry, &huma.Schema{Ref: "#/components/schemas/Missing"}, pb, huma.ModeWriteToServer, map[string]any{"x": 1}, res)
		})
	})

	t.Run("nil schema", func(t *testing.T) {
		assertUnresolved(t, func() {
			huma.Validate(registry, nil, pb, huma.ModeWriteToServer, map[string]any{"x": 1}, res)
		})
	})

	t.Run("discriminator mapping ref", func(t *testing.T) {
		schema := &huma.Schema{
			OneOf: []*huma.Schema{{Type: huma.TypeObject}},
			Discriminator: &huma.Discriminator{
				PropertyName: "kind",
				Mapping:      map[string]string{"cat": "#/components/schemas/MissingCat"},
			},
		}
		assertUnresolved(t, func() {
			huma.Validate(registry, schema, pb, huma.ModeWriteToServer, map[string]any{"kind": "cat"}, res)
		})
	})

	t.Run("property ref map[string]any", func(t *testing.T) {
		schema := &huma.Schema{
			Type: huma.TypeObject,
			Properties: map[string]*huma.Schema{
				"nested": {Ref: "#/components/schemas/AlsoMissing"},
			},
		}
		schema.PrecomputeMessages()
		assertUnresolved(t, func() {
			huma.Validate(registry, schema, pb, huma.ModeWriteToServer, map[string]any{"nested": "v"}, res)
		})
	})

	t.Run("property ref map[any]any", func(t *testing.T) {
		schema := &huma.Schema{
			Type: huma.TypeObject,
			Properties: map[string]*huma.Schema{
				"nested": {Ref: "#/components/schemas/StillMissing"},
			},
		}
		schema.PrecomputeMessages()
		assertUnresolved(t, func() {
			huma.Validate(registry, schema, pb, huma.ModeWriteToServer, map[any]any{"nested": "v"}, res)
		})
	})
}
