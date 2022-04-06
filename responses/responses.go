package responses

import (
	"net/http"

	"github.com/danielgtaylor/huma"
)

func newResponse(status int) huma.Response {
	return huma.NewResponse(status, http.StatusText(status))
}

var response func(int) huma.Response = newResponse

func errorResponse(status int) huma.Response {
	return response(status).
		ContentType("application/problem+json").
		Model(&huma.ErrorModel{})
}

// OK HTTP 200 response.
func OK() huma.Response {
	return response(http.StatusOK)
}

// Created HTTP 201 response.
func Created() huma.Response {
	return response(http.StatusCreated)
}

// Accepted HTTP 202 response.
func Accepted() huma.Response {
	return response(http.StatusAccepted)
}

// NoContent HTTP 204 response.
func NoContent() huma.Response {
	return response(http.StatusNoContent)
}

// PartialContent HTTP 206 response
func PartialContent() huma.Response {
	return response(http.StatusPartialContent)
}

// MovedPermanently HTTP 301 response.
func MovedPermanently() huma.Response {
	return response(http.StatusMovedPermanently)
}

// Found HTTP 302 response.
func Found() huma.Response {
	return response(http.StatusFound)
}

// NotModified HTTP 304 response.
func NotModified() huma.Response {
	return response(http.StatusNotModified)
}

// TemporaryRedirect HTTP 307 response.
func TemporaryRedirect() huma.Response {
	return response(http.StatusTemporaryRedirect)
}

// PermanentRedirect HTTP 308 response.
func PermanentRedirect() huma.Response {
	return response(http.StatusPermanentRedirect)
}

// BadRequest HTTP 400 response with a structured error body (e.g. JSON).
func BadRequest() huma.Response {
	return errorResponse(http.StatusBadRequest)
}

// Unauthorized HTTP 401 response with a structured error body (e.g. JSON).
func Unauthorized() huma.Response {
	return errorResponse(http.StatusUnauthorized)
}

// Forbidden HTTP 403 response with a structured error body (e.g. JSON).
func Forbidden() huma.Response {
	return errorResponse(http.StatusForbidden)
}

// NotFound HTTP 404 response with a structured error body (e.g. JSON).
func NotFound() huma.Response {
	return errorResponse(http.StatusNotFound)
}

// NotAcceptable HTTP 406 response with a structured error body (e.g. JSON).
func NotAcceptable() huma.Response {
	return errorResponse(http.StatusNotAcceptable)
}

// RequestTimeout HTTP 408 response with a structured error body (e.g. JSON).
func RequestTimeout() huma.Response {
	return errorResponse(http.StatusRequestTimeout)
}

// Conflict HTTP 409 response with a structured error body (e.g. JSON).
func Conflict() huma.Response {
	return errorResponse(http.StatusConflict)
}

// PreconditionFailed HTTP 412 response with a structured error body (e.g. JSON).
func PreconditionFailed() huma.Response {
	return errorResponse(http.StatusPreconditionFailed)
}

// RequestEntityTooLarge HTTP 413 response with a structured error body (e.g. JSON).
func RequestEntityTooLarge() huma.Response {
	return errorResponse(http.StatusRequestEntityTooLarge)
}

// PreconditionRequired HTTP 428 response with a structured error body (e.g. JSON).
func PreconditionRequired() huma.Response {
	return errorResponse(http.StatusPreconditionRequired)
}

// InternalServerError HTTP 500 response with a structured error body (e.g. JSON).
func InternalServerError() huma.Response {
	return errorResponse(http.StatusInternalServerError)
}

// NotImplemented HTTP 501 response with a structured error body (e.g. JSON).
func NotImplemented() huma.Response {
	return errorResponse(http.StatusNotImplemented)
}

// BadGateway HTTP 502 response with a structured error body (e.g. JSON).
func BadGateway() huma.Response {
	return errorResponse(http.StatusBadGateway)
}

// ServiceUnavailable HTTP 503 response with a structured error body (e.g. JSON).
func ServiceUnavailable() huma.Response {
	return errorResponse(http.StatusServiceUnavailable)
}

// GatewayTimeout HTTP 504 response with a structured error body (e.g. JSON).
func GatewayTimeout() huma.Response {
	return errorResponse(http.StatusGatewayTimeout)
}

// String HTTP response with the given status code and `text/plain` content
// type.
func String(status int) huma.Response {
	return response(status).ContentType("text/plain")
}

// ServeContent returns a slice containing all valid responses for
// context.ServeContent
func ServeContent() []huma.Response {
	return []huma.Response{
		OK().Headers("Last-Modified", "Content-Type"),
		PartialContent().Headers(
			"Last-Modified",
			"Content-Type",
			"Content-Range",
			"Content-Length",
			"multipart/byteranges",
			"Accept-Ranges",
			"Content-Encoding"),
		NotModified().Headers("Last-Modified"),
		PreconditionFailed().Headers("Last-Modified", "Content-Type"),
		InternalServerError(),
	}
}
