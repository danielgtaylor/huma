package autopatch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
)

// TestPatchChiNoRouteContextReuse is a regression test for a panic that occurred
// when autopatch ran on the chi adapter. chi stores its matched-route state in
// the request context and reuses it instead of routing again, so propagating
// the PATCH request's context to the internal GET caused that bodyless GET to be
// routed back into the generated PATCH handler, recursing and panicking on
// io.ReadAll(nil). The chi adapter's SanitizeInternalContext strips that state
// so internal sub-requests route fresh.
func TestPatchChiNoRouteContextReuse(t *testing.T) {
	db := map[string]*ThingModel{
		"test": {ID: "test", Price: 1.00},
	}

	r := chi.NewRouter()
	api := humachi.New(r, huma.DefaultConfig("Test", "1.0.0"))

	type ThingResponse struct {
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
	}) (*ThingResponse, error) {
		thing := db[input.ThingID]
		if thing == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		return &ThingResponse{ETag: thing.ETag(), Body: thing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Errors:      []int{404, 412},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body    ThingModel
		IfMatch []string `header:"If-Match"`
	}) (*ThingResponse, error) {
		db[input.ThingID] = &input.Body
		return &ThingResponse{ETag: db[input.ThingID].ETag(), Body: db[input.ThingID]}, nil
	})

	AutoPatch(api)

	// Send a real PATCH through the chi router. Before the fix, the internal GET
	// reused the PATCH route context and recursed into the PATCH handler,
	// panicking on io.ReadAll of the bodyless internal request.
	req := httptest.NewRequest(http.MethodPatch, "/things/test",
		strings.NewReader(`{"price": 1.23}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		r.ServeHTTP(w, req)
	})

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test1.23[][]", w.Result().Header.Get("ETag"))
}
