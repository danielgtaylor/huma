package huma

import (
	"reflect"
	"strings"
)

// Response describes an HTTP response that can be returned from an operation.
type Response struct {
	description string
	status      int
	contentType string
	headers     []string
	model       reflect.Type
}

// NewResponse creates a new response representation.
func NewResponse(status int, description string) Response {
	return Response{
		status:      status,
		description: description,
	}
}

// ContentType sets the response's content type header.
func (r Response) ContentType(ct string) Response {
	return Response{
		description: r.description,
		status:      r.status,
		contentType: ct,
		headers:     r.headers,
		model:       r.model,
	}
}

// Headers returns a new response with the named headers added. Sending
// headers to the client is optional, but they must be named here before
// you can send them.
func (r Response) Headers(names ...string) Response {
	headers := r.headers
	if headers == nil {
		headers = []string{}
	}

	return Response{
		description: r.description,
		status:      r.status,
		contentType: r.contentType,
		headers:     append(headers, names...),
		model:       r.model,
	}
}

// Model returns a new response with the given model representing the body.
// Because Go cannot pass types, `bodyModel` should be an instance of the
// response body.
func (r Response) Model(bodyModel interface{}) Response {
	// Add a content type if none has been set. We prefer JSON since it's easy to
	// represent in OpenAPI. Content negotiation means we also support other
	// content types which the client can dynamically request.
	ct := r.contentType
	if ct == "" {
		ct = "application/json"
	}

	// Allow the `Content-Type` header if not already allowed.
	found := false
	for _, h := range r.headers {
		if strings.ToLower(h) == "content-type" {
			found = true
			break
		}
	}

	headers := r.headers
	if !found {
		headers = append(headers, "Content-Type")
	}

	return Response{
		description: r.description,
		status:      r.status,
		contentType: ct,
		headers:     headers,
		model:       reflect.TypeOf(bodyModel),
	}
}
