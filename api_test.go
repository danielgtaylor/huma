package huma_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
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

func TestResponseContentTypeWithExtensions(t *testing.T) {
	_, api := humatest.New(t)

	type output struct {
		ContentType string `header:"Content-Type"`
		Body        struct {
			Foo string `json:"foo"`
		}
	}

	huma.Get(api, "/charset", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/json; charset=utf-8",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	huma.Get(api, "/suffix", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/problem+json",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	huma.Get(api, "/both", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/problem+json; charset=utf-8",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	assert.NotPanics(t, func() {
		resp := api.Get("/charset")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"foo": "bar"}`, resp.Body.String())
	})

	assert.NotPanics(t, func() {
		resp := api.Get("/suffix")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/problem+json", resp.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"foo": "bar"}`, resp.Body.String())
	})

	assert.NotPanics(t, func() {
		resp := api.Get("/both")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/problem+json; charset=utf-8", resp.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"foo": "bar"}`, resp.Body.String())
	})

	t.Run("UnmarshalSuffix", func(t *testing.T) {
		type input struct {
			Foo string `json:"foo"`
		}
		var v input
		err := api.Unmarshal("application/problem+json; charset=utf-8", []byte(`{"foo": "bar"}`), &v)
		assert.NoError(t, err)
		assert.Equal(t, "bar", v.Foo)
	})

	huma.Get(api, "/malformed", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/json; charset=utf-8+wrong",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	t.Run("PanicOnMalformed", func(t *testing.T) {
		assert.Panics(t, func() {
			api.Get("/malformed")
		})
	})
}
