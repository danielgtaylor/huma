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

	// Top-level missing ref must not panic.
	require.NotPanics(t, func() {
		huma.Validate(registry, &huma.Schema{Ref: "#/components/schemas/Missing"}, pb, huma.ModeWriteToServer, map[string]any{"x": 1}, res)
	})
	require.NotEmpty(t, res.Errors)
	require.Contains(t, res.Errors[0].Error(), "expected schema $ref to resolve")

	// Property with missing ref.
	res.Reset()
	schema := &huma.Schema{
		Type: huma.TypeObject,
		Properties: map[string]*huma.Schema{
			"nested": {Ref: "#/components/schemas/AlsoMissing"},
		},
	}
	schema.PrecomputeMessages()
	require.NotPanics(t, func() {
		huma.Validate(registry, schema, pb, huma.ModeWriteToServer, map[string]any{"nested": "v"}, res)
	})
	require.NotEmpty(t, res.Errors)
	require.Contains(t, res.Errors[0].Error(), "expected schema $ref to resolve")
}
