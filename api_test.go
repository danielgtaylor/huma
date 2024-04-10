package huma_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

func TestBlankConfig(t *testing.T) {
	adapter := humatest.NewAdapter()

	assert.NotPanics(t, func() {
		huma.NewAPI(huma.Config{}, adapter)
	})
}

// ExampleAdapter_handle demonstrates how to use the adapter directly
// instead of using the `huma.Register` convenience function to add a new
// operation and handler to the API.
//
// Note that you are responsible for defining all of the operation details,
// including the parameter and response definitions & schemas.
func ExampleAdapter_handle() {
	// Create an adapter for your chosen router.
	adapter := NewExampleAdapter()

	// Register an operation with a custom handler.
	adapter.Handle(&huma.Operation{
		OperationID: "example-operation",
		Method:      "GET",
		Path:        "/example/{name}",
		Summary:     "Example operation",
		Parameters: []*huma.Param{
			{
				Name:        "name",
				In:          "path",
				Description: "Name to return",
				Required:    true,
				Schema: &huma.Schema{
					Type: "string",
				},
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "OK",
				Content: map[string]*huma.MediaType{
					"text/plain": {
						Schema: &huma.Schema{
							Type: "string",
						},
					},
				},
			},
		},
	}, func(ctx huma.Context) {
		// Get the `name` path parameter.
		name := ctx.Param("name")

		// Set the response content type, status code, and body.
		ctx.SetHeader("Content-Type", "text/plain; charset=utf-8")
		ctx.SetStatus(http.StatusOK)
		ctx.BodyWriter().Write([]byte("Hello, " + name))
	})
}

func TestContextValue(t *testing.T) {
	_, api := humatest.New(t)

	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		// Make an updated context available to the handler.
		ctx = huma.WithValue(ctx, "foo", "bar")
		next(ctx)
		assert.Equal(t, http.StatusNoContent, ctx.Status())
	})

	// Register a simple hello world operation in the API.
	huma.Get(api, "/test", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		assert.Equal(t, "bar", ctx.Value("foo"))
		return nil, nil
	})

	resp := api.Get("/test")
	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestChiRouterPrefix(t *testing.T) {
	mux := chi.NewMux()
	var api huma.API
	mux.Route("/api", func(r chi.Router) {
		config := huma.DefaultConfig("My API", "1.0.0")
		config.Servers = []*huma.Server{{URL: "http://localhost:8888/api"}}
		api = humachi.New(r, config)
	})

	type TestOutput struct {
		Body struct {
			Field string `json:"field"`
		}
	}

	// Register a simple hello world operation in the API.
	huma.Get(api, "/test", func(ctx context.Context, input *struct{}) (*TestOutput, error) {
		return &TestOutput{}, nil
	})

	// Create a test API around the underlying router to make easier requests.
	tapi := humatest.Wrap(t, humachi.New(mux, huma.DefaultConfig("Test", "1.0.0")))

	// The top-level router should respond to the full path even though the
	// operation was registered with just `/test`.
	resp := tapi.Get("/api/test")
	assert.Equal(t, http.StatusOK, resp.Code)

	// The transformer should generate links with the full URL path.
	assert.Contains(t, resp.Header().Get("Link"), "/api/schemas/TestOutputBody.json")

	// The docs HTML should point to the full URL including base path.
	resp = tapi.Get("/api/docs")
	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), "/api/openapi.yaml")
}
