package huma_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
)

func TestGroupNoPrefix(t *testing.T) {
	_, api := humatest.New(t)

	grp := huma.NewGroup(api)

	huma.Get(grp, "/users", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	assert.NotNil(t, api.OpenAPI().Paths["/users"])

	resp := api.Get("/users")
	assert.Equal(t, http.StatusNoContent, resp.Result().StatusCode)
}

func TestGroupMultiPrefix(t *testing.T) {
	_, api := humatest.New(t)

	grp := huma.NewGroup(api, "/v1", "/v2")

	huma.Get(grp, "/users", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	assert.Nil(t, api.OpenAPI().Paths["/users"])
	assert.NotNil(t, api.OpenAPI().Paths["/v1/users"])
	assert.NotNil(t, api.OpenAPI().Paths["/v2/users"])
	assert.NotEqual(t, api.OpenAPI().Paths["/v1/users"].Get.OperationID, api.OpenAPI().Paths["/v2/users"].Get.OperationID)

	resp := api.Get("/v1/users")
	assert.Equal(t, http.StatusNoContent, resp.Result().StatusCode)

	resp = api.Get("/v2/users")
	assert.Equal(t, http.StatusNoContent, resp.Result().StatusCode)
}

func TestGroupCustomizations(t *testing.T) {
	_, api := humatest.New(t)

	grp := huma.NewGroup(api, "/v1")

	opModifierCalled := false
	middlewareCalled := false
	transformerCalled := false
	grp.UseOperationModifier(func(op *huma.Operation) {
		opModifierCalled = true
	})

	grp.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		middlewareCalled = true
		next(ctx)
	})

	grp.UseTransformer(func(ctx huma.Context, status string, v any) (any, error) {
		transformerCalled = true
		return v, nil
	})

	huma.Get(grp, "/users", func(ctx context.Context, input *struct{}) (*struct {
		Body string
	}, error) {
		return &struct{ Body string }{Body: ""}, nil
	})

	resp := api.Get("/v1/users")
	assert.Equal(t, 200, resp.Result().StatusCode)
	assert.True(t, opModifierCalled)
	assert.True(t, middlewareCalled)
	assert.True(t, transformerCalled)
}

type FailingTransformAPI struct {
	huma.API
}

func (a *FailingTransformAPI) Transform(ctx huma.Context, status string, v any) (any, error) {
	return nil, fmt.Errorf("whoops")
}

func TestGroupTransformUnderlyingError(t *testing.T) {
	_, api := humatest.New(t)

	grp := huma.NewGroup(&FailingTransformAPI{API: api}, "/v1")

	huma.Get(grp, "/users", func(ctx context.Context, input *struct{}) (*struct {
		Body string
	}, error) {
		return &struct{ Body string }{Body: ""}, nil
	})

	assert.Panics(t, func() {
		api.Get("/v1/users")
	})
}

func TestGroupTransformError(t *testing.T) {
	_, api := humatest.New(t)

	grp := huma.NewGroup(api, "/v1")

	grp.UseTransformer(func(ctx huma.Context, status string, v any) (any, error) {
		return v, fmt.Errorf("whoops")
	})

	huma.Get(grp, "/users", func(ctx context.Context, input *struct{}) (*struct {
		Body string
	}, error) {
		return &struct{ Body string }{Body: ""}, nil
	})

	assert.Panics(t, func() {
		api.Get("/v1/users")
	})
}
