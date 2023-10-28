package autopatch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
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

	app := api.Adapter()
	// New change but with wrong manual ETag, should fail!
	w = httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{"price": 4.56}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	req.Header.Set("If-Match", "abc123")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusPreconditionFailed, w.Code, w.Body.String())

	// Correct manual ETag should pass!
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{"price": 4.56}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	req.Header.Set("If-Match", "test1.23[US123 EU456][]")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][]", w.Result().Header.Get("ETag"))

	// Merge Patch: invalid
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// JSON Patch Test
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`[
		{"op": "add", "path": "/tags", "value": ["b"]},
		{"op": "add", "path": "/tags/0", "value": "a"}
	]`))
	req.Header.Set("Content-Type", "application/json-patch+json")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][a b]", w.Result().Header.Get("ETag"))

	// JSON Patch: bad JSON
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`[`))
	req.Header.Set("Content-Type", "application/json-patch+json")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// JSON Patch: invalid patch
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`[{"op": "unsupported"}]`))
	req.Header.Set("Content-Type", "application/json-patch+json")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Shorthand Patch Test
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{tags[]: c}`))
	req.Header.Set("Content-Type", "application/merge-patch+shorthand")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][a b c]", w.Result().Header.Get("ETag"))

	// Shorthand Patch: bad input
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`[`))
	req.Header.Set("Content-Type", "application/merge-patch+shorthand")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Bad content type
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/unsupported-content-type")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code, w.Body.String())

	// PATCH body read error
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/notfound", iotest.ErrReader(fmt.Errorf("test error")))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// GET error
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/notfound", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	app.ServeHTTP(w, req)
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
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/things/test", nil)
	api.Adapter().ServeHTTP(w, req)
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

	assert.Equal(t, true, api.OpenAPI().Paths["/things/{thing-id}"].Patch.Deprecated)
}
