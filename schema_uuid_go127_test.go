//go:build go1.27

package huma_test

import (
	"reflect"
	"testing"
	"uuid"

	"github.com/stretchr/testify/assert"

	"github.com/danielgtaylor/huma/v2"
)

func TestSchemaStdlibUUID(t *testing.T) {
	type Input struct {
		ID   uuid.UUID   `json:"id"`
		IDs  []uuid.UUID `json:"ids"`
	}

	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeFor[Input](), false, "")

	id := s.Properties["id"]
	assert.Equal(t, huma.TypeString, id.Type)
	assert.Equal(t, "uuid", id.Format)

	ids := s.Properties["ids"]
	assert.Equal(t, huma.TypeArray, ids.Type)
	assert.Equal(t, huma.TypeString, ids.Items.Type)
	assert.Equal(t, "uuid", ids.Items.Format)
}
