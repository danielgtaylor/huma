package autopatch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"

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
	ID    string      `json:"id"`
	Price float32     `json:"price,omitempty"`
	Sales []SaleModel `json:"sales,omitempty"`
	Tags  []string    `json:"tags,omitempty"`
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
				for _, possible := range input.IfMatch {
					if possible == existing.ETag() {
						found = true
						break
					}
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
func TestMakeOptionalSchemaBasicProperties(t *testing.T) {
	originalSchema := &huma.Schema{
		Type: "object",
		Properties: map[string]*huma.Schema{
			"id":   {Type: "string"},
			"name": {Type: "string"},
		},
		Required: []string{"id", "name"},
	}

	optionalSchema := makeOptionalSchema(originalSchema)

	assert.Equal(t, "object", optionalSchema.Type)
	assert.Contains(t, optionalSchema.Properties, "id")
	assert.Contains(t, optionalSchema.Properties, "name")
	assert.Empty(t, optionalSchema.Required)
}

func TestMakeOptionalSchemaAnyOf(t *testing.T) {
	originalSchema := &huma.Schema{
		AnyOf: []*huma.Schema{
			{Type: "string"},
			{Type: "number"},
		},
	}

	optionalSchema := makeOptionalSchema(originalSchema)

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

	optionalSchema := makeOptionalSchema(originalSchema)

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

	optionalSchema := makeOptionalSchema(originalSchema)

	assert.NotNil(t, optionalSchema.Not)
	assert.Equal(t, "null", optionalSchema.Not.Type)
}

func TestMakeOptionalSchemaNilInput(t *testing.T) {
	assert.Nil(t, makeOptionalSchema(nil))
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

	optionalNestedSchema := makeOptionalSchema(nestedSchema)

	assert.Empty(t, optionalNestedSchema.Required)
	assert.Empty(t, optionalNestedSchema.Properties["nested"].Required)
}
