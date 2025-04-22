package huma_test

import (
	"context"
	"errors"
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

func TestGroupEmptyPath(t *testing.T) {
	_, api := humatest.New(t)

	grp := huma.NewGroup(api, "/users")

	huma.Get(grp, "", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	assert.Nil(t, api.OpenAPI().Paths["/"])
	assert.Nil(t, api.OpenAPI().Paths[""])
	assert.NotNil(t, api.OpenAPI().Paths["/users"])

	resp := api.Get("/users")
	assert.Equal(t, http.StatusNoContent, resp.Result().StatusCode)

}

func TestGroupMultiPrefix(t *testing.T) {
	_, api := humatest.New(t)

	// Ensure paths exist for when the shallow copy is made.
	api.OpenAPI().Paths = map[string]*huma.PathItem{}

	grp := huma.NewGroup(api, "/v1", "/v2")
	child := huma.NewGroup(grp, "/prefix")

	huma.Get(child, "/users", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	assert.Nil(t, api.OpenAPI().Paths["/users"])
	assert.NotNil(t, api.OpenAPI().Paths["/v1/prefix/users"])
	assert.NotNil(t, api.OpenAPI().Paths["/v2/prefix/users"])
	assert.NotEqual(t, api.OpenAPI().Paths["/v1/prefix/users"].Get.OperationID, api.OpenAPI().Paths["/v2/prefix/users"].Get.OperationID)

	resp := api.Get("/v1/prefix/users")
	assert.Equal(t, http.StatusNoContent, resp.Result().StatusCode)

	resp = api.Get("/v2/prefix/users")
	assert.Equal(t, http.StatusNoContent, resp.Result().StatusCode)
}

func TestGroupConvenienceEquivalency(t *testing.T) {
	_, api := humatest.New(t)

	// Register a normal route via convenience function.
	huma.Get(api, "/v1/users", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	// Upgrade to groups and expect the same behavior.
	grp2 := huma.NewGroup(api, "/v2")

	huma.Get(grp2, "/users", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	// Ensure convenience overrides still work.
	grp3 := huma.NewGroup(api, "/v3")
	huma.Get(grp3, "/users", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	}, func(o *huma.Operation) {
		o.OperationID = "custom-id"
		o.Summary = "Custom summary"
	})

	// Ensure group overrides still work.
	grp4 := huma.NewGroup(api, "/v4")
	grp4.UseModifier(func(op *huma.Operation, next func(*huma.Operation)) {
		op.OperationID = "custom-id"
		op.Summary = "Custom summary"
		next(op)
	})
	huma.Get(grp4, "/users", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	// Groups of groups should continue to work as well, including both groups in
	// the generated ID/summary.
	grp5 := huma.NewGroup(api, "/v5")
	grp6 := huma.NewGroup(grp5, "/users")
	huma.Get(grp6, "/", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	oapi := api.OpenAPI()

	assert.NotNil(t, oapi.Paths["/v1/users"])
	assert.NotNil(t, oapi.Paths["/v2/users"])
	assert.NotNil(t, oapi.Paths["/v3/users"])
	assert.NotNil(t, oapi.Paths["/v4/users"])
	assert.NotNil(t, oapi.Paths["/v5/users/"])

	assert.Equal(t, "get-v1-users", oapi.Paths["/v1/users"].Get.OperationID)
	assert.Equal(t, "get-v2-users", oapi.Paths["/v2/users"].Get.OperationID)
	assert.Equal(t, "custom-id", oapi.Paths["/v3/users"].Get.OperationID)
	assert.Equal(t, "custom-id", oapi.Paths["/v4/users"].Get.OperationID)
	assert.Equal(t, "get-v5-users", oapi.Paths["/v5/users/"].Get.OperationID)

	assert.Equal(t, "Get v1 users", oapi.Paths["/v1/users"].Get.Summary)
	assert.Equal(t, "Get v2 users", oapi.Paths["/v2/users"].Get.Summary)
	assert.Equal(t, "Custom summary", oapi.Paths["/v3/users"].Get.Summary)
	assert.Equal(t, "Custom summary", oapi.Paths["/v4/users"].Get.Summary)
	assert.Equal(t, "Get v5 users", oapi.Paths["/v5/users/"].Get.Summary)
}

func TestGroupCustomizations(t *testing.T) {
	_, api := humatest.New(t)

	grp := huma.NewGroup(api, "/v1")

	opModifier1Called := false
	opModifier2Called := false
	middleware1Called := false
	middleware2Called := false
	transformerCalled := false
	grp.UseSimpleModifier(func(op *huma.Operation) {
		opModifier1Called = true
	})

	grp.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		middleware1Called = true
		next(ctx)
	})
	grp.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		middleware2Called = true
		next(ctx)
	})

	grp.UseTransformer(func(ctx huma.Context, status string, v any) (any, error) {
		transformerCalled = true
		return v, nil
	})

	// Ensure nested groups behave properly.
	childGrp := huma.NewGroup(grp)
	childGrp.UseSimpleModifier(func(op *huma.Operation) {
		opModifier2Called = true
	})

	huma.Get(childGrp, "/users", func(ctx context.Context, input *struct{}) (*struct {
		Body string
	}, error) {
		return &struct{ Body string }{Body: ""}, nil
	})

	// Manual OpenAPI modification
	childGrp.OpenAPI().Info.Title = "Set from group"

	assert.NotNil(t, api.OpenAPI().Paths["/v1/users"])
	assert.Equal(t, "Set from group", api.OpenAPI().Info.Title)

	resp := api.Get("/v1/users")
	assert.Equal(t, 200, resp.Result().StatusCode)
	assert.True(t, opModifier1Called)
	assert.True(t, opModifier2Called)
	assert.True(t, middleware1Called)
	assert.True(t, middleware2Called)
	assert.True(t, transformerCalled)
}

func TestGroupHiddenOp(t *testing.T) {
	_, api := humatest.New(t)
	grp := huma.NewGroup(api, "/v1")
	huma.Register(grp, huma.Operation{
		OperationID: "get-users",
		Method:      http.MethodGet,
		Path:        "/users",
		Hidden:      true,
	}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
		return nil, nil
	})

	assert.Nil(t, api.OpenAPI().Paths["/v1/users"])
}

type FailingTransformAPI struct {
	huma.API
}

func (a *FailingTransformAPI) Transform(ctx huma.Context, status string, v any) (any, error) {
	return nil, errors.New("whoops")
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
		return v, errors.New("whoops")
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
