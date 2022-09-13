package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

	app := newTestRouter()

	things := app.Resource("/things/{thing-id}")

	// Create the necessary GET/PUT
	things.Get("get-thing", "docs",
		NewResponse(http.StatusOK, "OK").Headers("ETag").Model(&ThingModel{}),
		NewResponse(http.StatusNotFound, "Not Found"),
		NewResponse(http.StatusPreconditionFailed, "Failed"),
	).Run(func(ctx Context, input struct {
		ThingIDParam
	}) {
		t := db[input.ThingID]
		if t == nil {
			ctx.WriteError(http.StatusNotFound, "Not found")
			return
		}
		ctx.Header().Set("ETag", t.ETag())
		ctx.WriteModel(http.StatusOK, t)
	})

	things.Put("put-thing", "docs",
		NewResponse(http.StatusOK, "OK").Headers("ETag").Model(&ThingModel{}),
		NewResponse(http.StatusPreconditionFailed, "Precondition failed").Model(&ErrorModel{}),
	).Run(func(ctx Context, input struct {
		ThingIDParam
		Body    ThingModel
		IfMatch []string `header:"If-Match" doc:"Succeeds if the server's resource matches one of the passed values."`
	}) {
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
				ctx.WriteError(http.StatusPreconditionFailed, "ETag does not match")
				return
			}
		} else {
			// Since the GET returns an ETag, and the auto-patch feature should always
			// use it when available, we can fail the test if we ever get here.
			t.Fatal("No If-Match header set during PUT")
		}
		db[input.ThingID] = &input.Body
		ctx.Header().Set("ETag", db[input.ThingID].ETag())
		ctx.WriteModel(http.StatusOK, db[input.ThingID])
	})

	// Merge Patch Test
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{"price": 1.23}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test1.23[US123 EU456][]", w.Result().Header.Get("ETag"))

	// Same change results in a 304 (patches are idempotent)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{"price": 1.23}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotModified, w.Code, w.Body.String())

	// New change but with wrong manual ETag, should fail!
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{"price": 4.56}`))
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

	// Bad content type
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPatch, "/things/test", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/unsupported-content-type")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code, w.Body.String())
}

func TestPatchPutNoBody(t *testing.T) {
	app := newTestRouter()

	things := app.Resource("/things/{thing-id}")

	things.Get("get-thing", "docs",
		NewResponse(http.StatusOK, "OK").Model(&ThingModel{}),
	).Run(func(ctx Context, input struct {
		ThingIDParam
	}) {
		ctx.WriteModel(http.StatusOK, &ThingModel{})
	})

	things.Put("put-thing", "docs",
		NewResponse(http.StatusNoContent, "No Content"),
	).Run(func(ctx Context, input struct {
		ThingIDParam
		// Note: no body!
	}) {
		ctx.WriteHeader(http.StatusNoContent)
	})

	// There should be no generated PATCH since there is nothing to
	// write in the PUT!
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, "/things/test", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code, w.Body.String())
}
