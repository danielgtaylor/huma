package autopatch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fiberV2 "github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
)

// TestPatchFiberV2NoHost is a regression test for a panic that occurred when
// autopatch ran on the Fiber v2 adapter. The internal GET/PUT have no Host,
// fasthttp rejects such an HTTP/1.1 request, and the adapter's ServeHTTP
// turned that error into a panic.
func TestPatchFiberV2NoHost(t *testing.T) {
	db := map[string]*ThingModel{
		"test": {ID: "test", Price: 1.00},
	}

	app := fiberV2.New()
	api := humafiber.NewV2(app, huma.DefaultConfig("Test", "1.0.0"))

	type ThingResponse struct {
		ETag string `header:"ETag"`
		Body *ThingModel
	}

	// Fiber ends a route param name at "-", so {thing-id} can't be used here.
	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*ThingResponse, error) {
		thing := db[input.ID]
		if thing == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		return &ThingResponse{ETag: thing.ETag(), Body: thing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{id}",
		Errors:      []int{404, 412},
	}, func(ctx context.Context, input *struct {
		ID      string `path:"id"`
		Body    ThingModel
		IfMatch []string `header:"If-Match"`
	}) (*ThingResponse, error) {
		db[input.ID] = &input.Body
		return &ThingResponse{ETag: db[input.ID].ETag(), Body: db[input.ID]}, nil
	})

	AutoPatch(api)

	req := httptest.NewRequest(http.MethodPatch, "/things/test",
		strings.NewReader(`{"price": 1.23}`))
	req.Header.Set("Content-Type", "application/merge-patch+json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "test1.23[][]", resp.Header.Get("ETag"))
}
