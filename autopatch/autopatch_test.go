package autopatch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

type SaleModel struct {
	Location string `json:"location"`
	Count    int    `json:"count"`
}

func (m SaleModel) String() string {
	return fmt.Sprintf("%s%d", m.Location, m.Count)
}

type ThingModel struct {
	ID      string      `json:"id"`
	Price   float32     `json:"price,omitempty"`
	Sales   []SaleModel `json:"sales,omitempty"`
	Tags    []string    `json:"tags,omitempty"`
	Comment *string     `json:"comment"`
}

func (m ThingModel) ETag() string {
	return fmt.Sprintf("%s%v%v%v", m.ID, m.Price, m.Sales, m.Tags)
}

type ThingIDParam struct {
	ThingID string `path:"thing-id"`
}

func TestPatch(t *testing.T) {
	db := map[string]*ThingModel{
		"test": {
			ID:    "test",
			Price: 1.00,
			Sales: []SaleModel{
				{Location: "US", Count: 123},
				{Location: "EU", Count: 456},
			},
		},
	}

	_, api := humatest.New(t)

	type GetThingResponse struct {
		ETag string `header:"ETag"`
		Body *ThingModel
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*GetThingResponse, error) {
		thing := db[input.ThingID]
		if thing == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		resp := &GetThingResponse{
			ETag: thing.ETag(),
			Body: thing,
		}
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Errors:      []int{404, 412},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body    ThingModel
		IfMatch []string `header:"If-Match" doc:"Succeeds if the server's resource matches one of the passed values."`
	}) (*GetThingResponse, error) {
		if len(input.IfMatch) > 0 {
			found := false
			if existing := db[input.ThingID]; existing != nil {
				if slices.Contains(input.IfMatch, existing.ETag()) {
					found = true
				}
			}
			if !found {
				return nil, huma.Error412PreconditionFailed("ETag '" + strings.Join(input.IfMatch, ", ") + "' does not match")
			}
		} else {
			// Since the GET returns an ETag, and the auto-patch feature should always
			// use it when available, we can fail the test if we ever get here.
			t.Fatal("No If-Match header set during PUT")
		}
		db[input.ThingID] = &input.Body
		resp := &GetThingResponse{
			ETag: db[input.ThingID].ETag(),
			Body: db[input.ThingID],
		}
		return resp, nil
	})

	AutoPatch(api)

	w := api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"price": 1.23}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test1.23[US123 EU456][]", w.Result().Header.Get("ETag"))

	// Same change results in a 304 (patches are idempotent)
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"price": 1.23}`),
	)
	assert.Equal(t, http.StatusNotModified, w.Code, w.Body.String())

	// Extra headers should not be a problem, including `Accept`.
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		"Accept: application/json",
		"X-Some-Other: value",
		strings.NewReader(`{"price": 1.23}`),
	)
	assert.Equal(t, http.StatusNotModified, w.Code, w.Body.String())

	// New change but with wrong manual ETag, should fail!
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		"If-Match: abc123",
		strings.NewReader(`{"price": 4.56}`),
	)
	assert.Equal(t, http.StatusPreconditionFailed, w.Code, w.Body.String())

	// Correct manual ETag should pass!
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		"If-Match: test1.23[US123 EU456][]",
		strings.NewReader(`{"price": 4.56}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][]", w.Result().Header.Get("ETag"))

	// Merge Patch: invalid
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// JSON Patch Test
	w = api.Patch("/things/test",
		"Content-Type: application/json-patch+json",
		strings.NewReader(`[
			{"op": "add", "path": "/tags", "value": ["b"]},
			{"op": "add", "path": "/tags/0", "value": "a"}
		]`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][a b]", w.Result().Header.Get("ETag"))

	// JSON Patch: bad JSON
	w = api.Patch("/things/test",
		"Content-Type: application/json-patch+json",
		strings.NewReader(`[`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// JSON Patch: invalid patch
	w = api.Patch("/things/test",
		"Content-Type: application/json-patch+json",
		strings.NewReader(`[{"op": "unsupported"}]`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Shorthand Patch Test
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+shorthand",
		strings.NewReader(`{tags[]: c}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][a b c]", w.Result().Header.Get("ETag"))

	// Shorthand Patch: bad input
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+shorthand",
		strings.NewReader(`[`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Bad content type
	w = api.Patch("/things/test",
		"Content-Type: application/unsupported-content-type",
		strings.NewReader(`{}`),
	)
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code, w.Body.String())

	// PATCH body read error
	w = api.Patch("/things/notfound",
		"Content-Type: application/merge-patch+json",
		iotest.ErrReader(errors.New("test error")),
	)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// GET error
	w = api.Patch("/things/notfound",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{}`),
	)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestPatchPutNoBody(t *testing.T) {
	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		// Note: no body!
	}) (*struct{}, error) {
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		// Note: no body!
	}) (*struct{}, error) {
		return nil, nil
	})

	AutoPatch(api)

	// There should be no generated PATCH since there is nothing to
	// write in the PUT!
	assert.Nil(t, api.OpenAPI().Paths["/things/{thing-id}"].Patch)
	w := api.Patch("/things/test")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code, w.Body.String())
}

func TestExplicitDisable(t *testing.T) {
	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
		Metadata: map[string]any{
			"autopatch": false, //           <-- Disabled here!
		},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*struct{ Body struct{} }, error) {
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Errors:      []int{404, 412},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body    ThingModel
		IfMatch []string `header:"If-Match" doc:"Succeeds if the server's resource matches one of the passed values."`
	}) (*struct{ Body struct{} }, error) {
		return nil, nil
	})

	AutoPatch(api)

	// There should be no generated PATCH since there is nothing to
	// write in the PUT!
	assert.Nil(t, api.OpenAPI().Paths["/things/{thing-id}"].Patch)
	w := api.Patch("/things/test")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code, w.Body.String())
}

func TestPatchRecursiveNode(t *testing.T) {
	type Node struct {
		Name     string  `json:"name"`
		Children []*Node `json:"children,omitempty"`
	}

	db := map[string]*Node{
		"root": {Name: "root", Children: []*Node{{Name: "child"}}},
	}

	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-node",
		Method:      http.MethodGet,
		Path:        "/nodes/{id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct {
		Body *Node
	}, error) {
		node := db[input.ID]
		if node == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		return &struct{ Body *Node }{Body: node}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-node",
		Method:      http.MethodPut,
		Path:        "/nodes/{id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body Node
	}) (*struct {
		Body *Node
	}, error) {
		node := input.Body
		db[input.ID] = &node
		return &struct{ Body *Node }{Body: db[input.ID]}, nil
	})

	AutoPatch(api)

	path := api.OpenAPI().Paths["/nodes/{id}"]
	require.NotNil(t, path)
	require.NotNil(t, path.Patch)

	w := api.Patch("/nodes/root",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"name": "updated"}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"name":"updated"`)
}

func TestDeprecatedPatch(t *testing.T) {
	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*struct {
		Body *ThingModel
	}, error) {
		return &struct{ Body *ThingModel }{&ThingModel{}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Deprecated:  true,
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body ThingModel
	}) (*struct {
		Body *ThingModel
	}, error) {
		return &struct{ Body *ThingModel }{&ThingModel{}}, nil
	})

	AutoPatch(api)

	assert.True(t, api.OpenAPI().Paths["/things/{thing-id}"].Patch.Deprecated)
}

func TestNullabilityExtension(t *testing.T) {
	comment := "comment1"
	db := map[string]*ThingModel{
		"test": {
			ID:    "test",
			Price: 1.00,
			Sales: []SaleModel{
				{Location: "US", Count: 123},
			},
			Tags:    []string{"tag1", "tag2"},
			Comment: &comment,
		},
	}

	_, api := humatest.New(t)

	// Register the nullability extension
	RegisterNullabilityExtension(api, "__NULL__")

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*struct {
		Body *ThingModel
	}, error) {
		thing := db[input.ThingID]
		if thing == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		return &struct{ Body *ThingModel }{thing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body ThingModel
	}) (*struct {
		Body *ThingModel
	}, error) {
		db[input.ThingID] = &input.Body
		return &struct{ Body *ThingModel }{&input.Body}, nil
	})

	AutoPatch(api)

	// Test setting an array field to null using the string representation
	w := api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"tags": "__NULL__"}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Verify the field was actually set to null; expect it to be omitted
	w = api.Get("/things/test")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), `"tags"`)

	// Test setting a nullable string field to null
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"comment": null}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Verify the nested field was set to null
	w = api.Get("/things/test")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"comment":null`)

	// Test error handling for invalid JSON
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{invalid`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func testRegistry() huma.Registry {
	return huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
}

func testMakeOptionalSchema(registry huma.Registry, s *huma.Schema) *huma.Schema {
	return makeOptionalSchema(registry, s, map[string]struct{}{})
}

func TestMakeOptionalSchemaBasicProperties(t *testing.T) {
	originalSchema := &huma.Schema{
		Type: "object",
		Properties: map[string]*huma.Schema{
			"id":   {Type: "string"},
			"name": {Type: "string"},
		},
		Required: []string{"id", "name"},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Equal(t, "object", optionalSchema.Type)
	assert.Contains(t, optionalSchema.Properties, "id")
	assert.Contains(t, optionalSchema.Properties, "name")
	assert.Empty(t, optionalSchema.Required)
}

func TestMakeOptionalSchemaOneOf(t *testing.T) {
	originalSchema := &huma.Schema{
		OneOf: []*huma.Schema{
			{Type: "string"},
			{Type: "number"},
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Len(t, optionalSchema.OneOf, 2)
	assert.Equal(t, "string", optionalSchema.OneOf[0].Type)
	assert.Equal(t, "number", optionalSchema.OneOf[1].Type)
}

func TestMakeOptionalSchemaAnyOf(t *testing.T) {
	originalSchema := &huma.Schema{
		AnyOf: []*huma.Schema{
			{Type: "string"},
			{Type: "number"},
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Len(t, optionalSchema.AnyOf, 2)
	assert.Equal(t, "string", optionalSchema.AnyOf[0].Type)
	assert.Equal(t, "number", optionalSchema.AnyOf[1].Type)
}

func TestMakeOptionalSchemaAllOf(t *testing.T) {
	minLength := 1
	maxLength := 100
	originalSchema := &huma.Schema{
		AllOf: []*huma.Schema{
			{MinLength: &minLength},
			{MaxLength: &maxLength},
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Len(t, optionalSchema.AllOf, 2)
	assert.Equal(t, 1, *optionalSchema.AllOf[0].MinLength)
	assert.Equal(t, 100, *optionalSchema.AllOf[1].MaxLength)
}

func TestMakeOptionalSchemaNot(t *testing.T) {
	originalSchema := &huma.Schema{
		Not: &huma.Schema{
			Type: "null",
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.NotNil(t, optionalSchema.Not)
	assert.Equal(t, "null", optionalSchema.Not.Type)
}

func TestMakeOptionalSchemaNilInput(t *testing.T) {
	assert.Nil(t, testMakeOptionalSchema(testRegistry(), nil))
}

func TestReplaceNulls(t *testing.T) {
	settings := MergePatchNullabilitySettings{
		Enabled:                    true,
		StringRepresentationOfNull: "__NULL__",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple null",
			input:    `{"name": null}`,
			expected: `{"name":"__NULL__"}`,
		},
		{
			name:     "nested null",
			input:    `{"user": {"name": null, "active": true}}`,
			expected: `{"user":{"active":true,"name":"__NULL__"}}`,
		},
		{
			name:     "array with null",
			input:    `{"tags": ["a", null, "c"]}`,
			expected: `{"tags":["a","__NULL__","c"]}`,
		},
		{
			name:     "no nulls",
			input:    `{"name": "test", "count": 42}`,
			expected: `{"count":42,"name":"test"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := replaceNulls([]byte(tc.input), settings)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(result))
		})
	}

	// Test error case
	_, err := replaceNulls([]byte(`{invalid json`), settings)
	assert.Error(t, err)
}

func TestRestoreNulls(t *testing.T) {
	settings := MergePatchNullabilitySettings{
		Enabled:                    true,
		StringRepresentationOfNull: "__NULL__",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple null string",
			input:    `{"name": "__NULL__"}`,
			expected: `{"name":null}`,
		},
		{
			name:     "nested null string",
			input:    `{"user": {"name": "__NULL__", "active": true}}`,
			expected: `{"user":{"name":null,"active":true}}`,
		},
		{
			name:     "array with null string",
			input:    `{"tags": ["a", "__NULL__", "c"]}`,
			expected: `{"tags":["a",null,"c"]}`,
		},
		{
			name:     "no null strings",
			input:    `{"name": "test", "count": 42}`,
			expected: `{"name":"test","count":42}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := restoreNulls([]byte(tc.input), settings)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(result))
		})
	}

	// Test error case
	_, err := restoreNulls([]byte(`{invalid json`), settings)
	assert.Error(t, err)
}

func TestMakeOptionalSchemaNestedSchemas(t *testing.T) {
	nestedSchema := &huma.Schema{
		Type: "object",
		Properties: map[string]*huma.Schema{
			"nested": {
				Type: "object",
				Properties: map[string]*huma.Schema{
					"deeplyNested": {Type: "string"},
				},
				Required: []string{"deeplyNested"},
			},
		},
		Required: []string{"nested"},
	}

	optionalNestedSchema := testMakeOptionalSchema(testRegistry(), nestedSchema)

	assert.Empty(t, optionalNestedSchema.Required)
	assert.Empty(t, optionalNestedSchema.Properties["nested"].Required)
}

func TestMakeOptionalSchemaRefs(t *testing.T) {
	const addressRef = "#/components/schemas/Address"
	const tagRef = "#/components/schemas/Tag"

	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	registry.Map()["Address"] = &huma.Schema{
		Type: "object",
		Properties: map[string]*huma.Schema{
			"street": {Type: "string"},
			"city":   {Type: "string"},
		},
		Required: []string{"street", "city"},
	}
	registry.Map()["Tag"] = &huma.Schema{Type: "string"}

	t.Run("top-level ref", func(t *testing.T) {
		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: addressRef})

		assert.Empty(t, optional.Ref)
		assert.Equal(t, "object", optional.Type)
		assert.Empty(t, optional.Required)
		assert.Len(t, optional.Properties, 2)
		assert.Equal(t, "string", optional.Properties["street"].Type)
		assert.Equal(t, "string", optional.Properties["city"].Type)
	})

	t.Run("property ref", func(t *testing.T) {
		parent := &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"address": {Ref: addressRef},
			},
			Required: []string{"address"},
		}

		optional := testMakeOptionalSchema(registry, parent)

		assert.Empty(t, optional.Required)
		address := optional.Properties["address"]
		require.NotNil(t, address)
		assert.Empty(t, address.Ref)
		assert.Equal(t, "object", address.Type)
		assert.Empty(t, address.Required)
	})

	t.Run("items ref", func(t *testing.T) {
		parent := &huma.Schema{
			Type:  "array",
			Items: &huma.Schema{Ref: tagRef},
		}

		optional := testMakeOptionalSchema(registry, parent)

		require.NotNil(t, optional.Items)
		assert.Empty(t, optional.Items.Ref)
		assert.Equal(t, "string", optional.Items.Type)
	})

	t.Run("unresolvable external ref", func(t *testing.T) {
		const externalRef = "https://example.com/schemas/Thing.json"

		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: externalRef})

		require.NotNil(t, optional)
		assert.Equal(t, externalRef, optional.Ref)
		assert.Empty(t, optional.Type)
	})

	t.Run("recursive self reference", func(t *testing.T) {
		const nodeRef = "#/components/schemas/Node"

		registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
		registry.Map()["Node"] = &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"name": {Type: "string"},
				"children": {
					Type:  "array",
					Items: &huma.Schema{Ref: nodeRef},
				},
			},
			Required: []string{"name"},
		}

		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: nodeRef})

		assert.Empty(t, optional.Ref)
		assert.Equal(t, "object", optional.Type)
		assert.Empty(t, optional.Required)
		assert.Equal(t, "string", optional.Properties["name"].Type)
		assert.Equal(t, "array", optional.Properties["children"].Type)
		require.NotNil(t, optional.Properties["children"].Items)
		assert.Equal(t, nodeRef, optional.Properties["children"].Items.Ref)
	})
}

func TestMakeOptionalSchemaAdditionalProperties(t *testing.T) {
	t.Run("boolean additionalProperties", func(t *testing.T) {
		for _, val := range []bool{true, false} {
			original := &huma.Schema{
				Type: "object",
				Properties: map[string]*huma.Schema{
					"name": {Type: "string"},
				},
				Required:             []string{"name"},
				AdditionalProperties: val,
			}

			optional := testMakeOptionalSchema(testRegistry(), original)

			assert.Equal(t, val, optional.AdditionalProperties)
			assert.Empty(t, optional.Required)
		}
	})

	t.Run("inline schema additionalProperties", func(t *testing.T) {
		original := &huma.Schema{
			Type: "object",
			AdditionalProperties: &huma.Schema{
				Type: "object",
				Properties: map[string]*huma.Schema{
					"value": {Type: "string"},
				},
				Required: []string{"value"},
			},
		}

		optional := testMakeOptionalSchema(testRegistry(), original)

		addl, ok := optional.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Equal(t, "object", addl.Type)
		assert.Empty(t, addl.Required)
		assert.Equal(t, "string", addl.Properties["value"].Type)
	})

	t.Run("ref additionalProperties", func(t *testing.T) {
		const tagRef = "#/components/schemas/Tag"

		registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
		registry.Map()["Tag"] = &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"label": {Type: "string"},
			},
			Required: []string{"label"},
		}

		original := &huma.Schema{
			Type:                 "object",
			AdditionalProperties: &huma.Schema{Ref: tagRef},
		}

		optional := testMakeOptionalSchema(registry, original)

		addl, ok := optional.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Empty(t, addl.Ref)
		assert.Equal(t, "object", addl.Type)
		assert.Empty(t, addl.Required)
		assert.Equal(t, "string", addl.Properties["label"].Type)
	})

	t.Run("recursive ref additionalProperties", func(t *testing.T) {
		const nodeRef = "#/components/schemas/Node"

		registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
		registry.Map()["Node"] = &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"name": {Type: "string"},
				"named": {
					Type: "object",
					AdditionalProperties: &huma.Schema{Ref: nodeRef},
				},
			},
			Required: []string{"name"},
		}

		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: nodeRef})

		assert.Empty(t, optional.Required)
		named := optional.Properties["named"]
		require.NotNil(t, named)
		addl, ok := named.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Equal(t, nodeRef, addl.Ref)
	})

	t.Run("unresolvable ref additionalProperties", func(t *testing.T) {
		const externalRef = "https://example.com/schemas/Thing.json"

		original := &huma.Schema{
			Type:                 "object",
			AdditionalProperties: &huma.Schema{Ref: externalRef},
		}

		optional := testMakeOptionalSchema(testRegistry(), original)

		addl, ok := optional.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Equal(t, externalRef, addl.Ref)
	})
}

type findRelativeResourcePathTest struct {
	requestPath string
	putPath     string
	expected    string
}

func TestFindRelativeResourcePath(t *testing.T) {
	tests := []findRelativeResourcePathTest{
		{
			requestPath: "/things/test",
			putPath:     "/things/{id}",
			expected:    "/things/test",
		},
		{
			requestPath: "/api/things/test",
			putPath:     "/things/{id}",
			expected:    "/things/test",
		},
		{
			requestPath: "/test",
			putPath:     "/{id}",
			expected:    "/test",
		},
		{
			requestPath: "/api/v1/super/things/test",
			putPath:     "/things/{id}",
			expected:    "/things/test",
		},
		{
			requestPath: "/api/v1/test",
			putPath:     "{id}",
			expected:    "/api/v1/test", // we check that we falback to the request path if unsupported
		},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, findRelativeResourcePath(test.requestPath, test.putPath))
	}
}
