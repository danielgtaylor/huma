package huma

import "reflect"

// NegotiatedContentType is a special value to tell Huma to use content
// negotiation with the client to determine what format to send, e.g. JSON
// or CBOR.
const NegotiatedContentType = "application/vnd.huma.negotiated"

// NegotiatedErrorContentType is a special value to tell Huma to use content
// negotiation with the client to determine what format to send, e.g. JSON
// or CBOR. Use this for errors to get e.g. application/problem+json.
const NegotiatedErrorContentType = "application/vnd.huma.negotiated.error"

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
	ct := r.contentType
	if ct == "" {
		ct = "application/json"
	}

	return Response{
		description: r.description,
		status:      r.status,
		contentType: ct,
		headers:     r.headers,
		model:       reflect.TypeOf(bodyModel),
	}
}
