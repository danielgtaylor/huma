package responses

import (
	"net/http"

	"github.com/danielgtaylor/huma"
)

func response(status int) huma.Response {
	return huma.NewResponse(status, http.StatusText(status))
}

func errorResponse(status int) huma.Response {
	return response(status).
		ContentType(huma.NegotiatedErrorContentType).
		Model(huma.ErrorModel{})
}

// OK HTTP 200 response.
func OK() huma.Response {
	return response(http.StatusOK)
}

// Created HTTP 201 response.
func Created() huma.Response {
	return response(http.StatusCreated)
}

// NoContent HTTP 204 response.
func NoContent() huma.Response {
	return response(http.StatusNoContent)
}

// BadRequest HTTP 400 response with a structured error body (e.g. JSON).
func BadRequest() huma.Response {
	return errorResponse(http.StatusBadRequest)
}

// NotFound HTTP 404 response with a structured error body (e.g. JSON).
func NotFound() huma.Response {
	return errorResponse(http.StatusNotFound)
}

// GatewayTimeout HTTP 504 response with a structured error body (e.g. JSON).
func GatewayTimeout() huma.Response {
	return errorResponse(http.StatusGatewayTimeout)
}

// InternalServerError HTTP 500 response with a structured error body (e.g. JSON).
func InternalServerError() huma.Response {
	return errorResponse(http.StatusInternalServerError)
}

// String HTTP response with the given status code.
func String(status int) huma.Response {
	return huma.NewResponse(status, http.StatusText(status)).ContentType("text/plain")
}

// // Model response with a structured body (e.g. JSON).
// func Model(status int, model interface{}) huma.Response {
// 	return huma.Response{
// 		Status:      status,
// 		ContentType: huma.NegotiatedContentType,
// 		Model:       reflect.TypeOf(model),
// 	}

// }

// // Error response with a structured error body (e.g. JSON).
// func Error(status int) huma.Response {
// 	return huma.Response{
// 		Status:      status,
// 		ContentType: huma.NegotiatedErrorContentType,
// 		Model:       reflect.TypeOf(huma.ErrorModel{}),
// 	}
// }
