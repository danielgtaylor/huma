package huma_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
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

	err.Add(errors.New("plain error"))

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
		// Client errors.
		{huma.Error400BadRequest, 400},
		{huma.Error401Unauthorized, 401},
		{huma.Error402PaymentRequired, 402},
		{huma.Error403Forbidden, 403},
		{huma.Error404NotFound, 404},
		{huma.Error405MethodNotAllowed, 405},
		{huma.Error406NotAcceptable, 406},
		{huma.Error407ProxyAuthRequired, 407},
		{huma.Error408RequestTimeout, 408},
		{huma.Error409Conflict, 409},
		{huma.Error410Gone, 410},
		{huma.Error411LengthRequired, 411},
		{huma.Error412PreconditionFailed, 412},
		{huma.Error413RequestEntityTooLarge, 413},
		{huma.Error414RequestURITooLong, 414},
		{huma.Error415UnsupportedMediaType, 415},
		{huma.Error416RequestedRangeNotSatisfiable, 416},
		{huma.Error417ExpectationFailed, 417},
		{huma.Error418Teapot, 418},
		{huma.Error421MisdirectedRequest, 421},
		{huma.Error422UnprocessableEntity, 422},
		{huma.Error423Locked, 423},
		{huma.Error424FailedDependency, 424},
		{huma.Error425TooEarly, 425},
		{huma.Error426UpgradeRequired, 426},
		{huma.Error428PreconditionRequired, 428},
		{huma.Error429TooManyRequests, 429},
		{huma.Error431RequestHeaderFieldsTooLarge, 431},
		{huma.Error451UnavailableForLegalReasons, 451},

		// Server errors.
		{huma.Error500InternalServerError, 500},
		{huma.Error501NotImplemented, 501},
		{huma.Error502BadGateway, 502},
		{huma.Error503ServiceUnavailable, 503},
		{huma.Error504GatewayTimeout, 504},
		{huma.Error505HTTPVersionNotSupported, 505},
		{huma.Error506VariantAlsoNegotiates, 506},
		{huma.Error507InsufficientStorage, 507},
		{huma.Error508LoopDetected, 508},
		{huma.Error510NotExtended, 510},
		{huma.Error511NetworkAuthenticationRequired, 511},
	} {
		err := item.constructor("test")
		assert.Equal(t, item.expected, err.GetStatus())
	}
}

func TestNegotiateError(t *testing.T) {
	_, api := humatest.New(t, huma.Config{OpenAPI: &huma.OpenAPI{Info: &huma.Info{Title: "Test API", Version: "1.0.0"}}})

	req, _ := http.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()
	ctx := humatest.NewContext(&huma.Operation{}, req, resp)
	require.Error(t, huma.WriteErr(api, ctx, 400, "bad request"))
}

func TestTransformError(t *testing.T) {
	config := huma.DefaultConfig("Test API", "1.0.0")
	config.Transformers = []huma.Transformer{
		func(ctx huma.Context, status string, v any) (any, error) {
			return nil, errors.New("whoops")
		},
	}
	_, api := humatest.New(t, config)

	req, _ := http.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()
	ctx := humatest.NewContext(&huma.Operation{}, req, resp)

	require.Error(t, huma.WriteErr(api, ctx, 400, "bad request"))
}

func TestErrorAs(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", huma.Error400BadRequest("test"))

	var e huma.StatusError
	require.ErrorAs(t, err, &e)
	assert.Equal(t, 400, e.GetStatus())
}

func TestErrorWithHeaders(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Get(api, "/test", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		err := huma.ErrorWithHeaders(
			huma.Error400BadRequest("test"),
			http.Header{
				"My-Header": {"bar"},
			},
		)

		assert.Equal(t, "test", err.Error())

		// Call again and have all the headers merged
		err = huma.ErrorWithHeaders(err, http.Header{
			"Another": {"bar"},
		})

		return nil, fmt.Errorf("wrapped: %w", err)
	})

	resp := api.Get("/test")
	assert.Equal(t, 400, resp.Code)
	assert.Equal(t, "bar", resp.Header().Get("My-Header"))
	assert.Equal(t, "bar", resp.Header().Get("Another"))
	assert.Contains(t, resp.Body.String(), "test")
}
