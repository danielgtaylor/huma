package huma_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Ensure the default error models satisfy these interfaces.
var _ huma.StatusError = (*huma.ErrorModel)(nil)
var _ huma.ContentTypeFilter = (*huma.ErrorModel)(nil)
var _ huma.ErrorDetailer = (*huma.ErrorDetail)(nil)

func TestError(t *testing.T) {
	err := &huma.ErrorModel{
		Status: 400,
		Detail: "test err",
	}

	// Add some children.
	err.Add(&huma.ErrorDetail{
		Message:  "test detail",
		Location: "body.foo",
		Value:    "bar",
	})

	err.Add(fmt.Errorf("plain error"))

	// Confirm errors were added.
	assert.Equal(t, "test err", err.Error())
	assert.Len(t, err.Errors, 2)
	assert.Equal(t, "test detail (body.foo: bar)", err.Errors[0].Error())
	assert.Equal(t, "plain error", err.Errors[1].Error())

	// Ensure problem content types.
	assert.Equal(t, "application/problem+json", err.ContentType("application/json"))
	assert.Equal(t, "application/problem+cbor", err.ContentType("application/cbor"))
	assert.Equal(t, "other", err.ContentType("other"))
}

func TestErrorResponses(t *testing.T) {
	// NotModified has a slightly different signature.
	assert.Equal(t, 304, huma.Status304NotModified().GetStatus())

	for _, item := range []struct {
		constructor func(msg string, errs ...error) huma.StatusError
		expected    int
	}{
		{huma.Error400BadRequest, 400},
		{huma.Error401Unauthorized, 401},
		{huma.Error403Forbidden, 403},
		{huma.Error404NotFound, 404},
		{huma.Error405MethodNotAllowed, 405},
		{huma.Error406NotAcceptable, 406},
		{huma.Error409Conflict, 409},
		{huma.Error410Gone, 410},
		{huma.Error412PreconditionFailed, 412},
		{huma.Error415UnsupportedMediaType, 415},
		{huma.Error422UnprocessableEntity, 422},
		{huma.Error429TooManyRequests, 429},
		{huma.Error500InternalServerError, 500},
		{huma.Error501NotImplemented, 501},
		{huma.Error502BadGateway, 502},
		{huma.Error503ServiceUnavailable, 503},
		{huma.Error504GatewayTimeout, 504},
	} {
		err := item.constructor("test")
		assert.Equal(t, item.expected, err.GetStatus())
	}
}

func TestNegotiateError(t *testing.T) {
	_, api := humatest.New(t, huma.Config{})

	req, _ := http.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, resp)

	require.Error(t, huma.WriteErr(api, ctx, 400, "bad request"))
}

func TestTransformError(t *testing.T) {
	config := huma.DefaultConfig("Test API", "1.0.0")
	config.Transformers = []huma.Transformer{
		func(ctx huma.Context, status string, v any) (any, error) {
			return nil, fmt.Errorf("whoops")
		},
	}
	_, api := humatest.New(t, config)

	req, _ := http.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, resp)

	require.Error(t, huma.WriteErr(api, ctx, 400, "bad request"))
}
