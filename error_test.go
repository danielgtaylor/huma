package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

// Ensure the default error models satisfy these interfaces.
var _ StatusError = (*ErrorModel)(nil)
var _ ContentTypeFilter = (*ErrorModel)(nil)
var _ ErrorDetailer = (*ErrorDetail)(nil)

func TestError(t *testing.T) {
	err := &ErrorModel{
		Status: 400,
		Detail: "test err",
	}

	// Add some children.
	err.Add(&ErrorDetail{
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
	assert.Equal(t, 304, Status304NotModified().GetStatus())

	for _, item := range []struct {
		constructor func(msg string, errs ...error) StatusError
		expected    int
	}{
		{Error400BadRequest, 400},
		{Error401Unauthorized, 401},
		{Error403Forbidden, 403},
		{Error404NotFound, 404},
		{Error405MethodNotAllowed, 405},
		{Error406NotAcceptable, 406},
		{Error409Conflict, 409},
		{Error410Gone, 410},
		{Error412PreconditionFailed, 412},
		{Error415UnsupportedMediaType, 415},
		{Error422UnprocessableEntity, 422},
		{Error429TooManyRequests, 429},
		{Error500InternalServerError, 500},
		{Error501NotImplemented, 501},
		{Error502BadGateway, 502},
		{Error503ServiceUnavailable, 503},
		{Error504GatewayTimeout, 504},
	} {
		err := item.constructor("test")
		assert.Equal(t, item.expected, err.GetStatus())
	}
}

func TestNegotiateError(t *testing.T) {
	r := chi.NewMux()
	api := NewTestAdapter(r, Config{})

	req, _ := http.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()
	ctx := &testContext{nil, req, resp}

	assert.Error(t, WriteErr(api, ctx, 400, "bad request"))
}
