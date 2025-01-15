package jsonrpc

import (
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

type JSONRPCErrorCode int

const (
	// ParseError defines invalid JSON was received by the server.
	// An error occurred on the server while parsing the JSON text.
	ParseError JSONRPCErrorCode = -32700

	// InvalidRequestError defines the JSON sent is not a valid Request object.
	InvalidRequestError JSONRPCErrorCode = -32600

	// MethodNotFoundError defines the method does not exist / is not available.
	MethodNotFoundError JSONRPCErrorCode = -32601

	// InvalidParamsError defines invalid method parameter(s).
	InvalidParamsError JSONRPCErrorCode = -32602

	// InternalError defines a server error
	InternalError JSONRPCErrorCode = -32603
)

var errorMessage = map[JSONRPCErrorCode]string{
	ParseError:          "An error occurred on the server while parsing JSON object.",
	InvalidRequestError: "The JSON sent is not a valid Request object.",
	MethodNotFoundError: "The method does not exist / is not available.",
	InvalidParamsError:  "Invalid method parameter(s).",
	InternalError:       "Internal JSON-RPC error.",
}

func GetDefaultErrorMessage(code JSONRPCErrorCode) string {
	return errorMessage[code]
}

// Error defines a JSON RPC error that can be returned in a Response from the spec
// http://www.jsonrpc.org/specification#error_object
type JSONRPCError struct {
	// The error type that occurred.
	Code JSONRPCErrorCode `json:"code"`

	// A short description of the error. The message SHOULD be limited to a concise
	// single sentence.
	Message string `json:"message"`

	// Additional information about the error. The value of this member is defined by
	// the sender (e.g. detailed error information, nested errors etc.).
	Data interface{} `json:"data,omitempty"`
}

// Error implements error.
func (e JSONRPCError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return errorMessage[e.Code]
}

// ErrorCode returns the JSON RPC error code associated with the error.
func (e JSONRPCError) ErrorCode() JSONRPCErrorCode {
	return e.Code
}

type ResponseStatusError struct {
	Response[any]
	status int `json:"-"`
}

func (e *ResponseStatusError) Error() string {
	if e.Response.Error != nil {
		return e.Response.Error.Message
	}
	return ""
}

func (e *ResponseStatusError) GetStatus() int {
	return e.status
}

func (e ResponseStatusError) Schema(r huma.Registry) *huma.Schema {

	errorObjectSchema := r.Schema(reflect.TypeOf(e.Response.Error), true, "")

	responseObjectSchema := &huma.Schema{
		Type:     huma.TypeObject,
		Required: []string{"jsonrpc"},
		Properties: map[string]*huma.Schema{
			"jsonrpc": {
				Type:        huma.TypeString,
				Enum:        []any{"2.0"},
				Description: "JSON-RPC version, must be '2.0'",
			},
			"id": {
				Description: "Request identifier. Compulsory for method responses. This MUST be null to the client in case of parse errors etc.",
				OneOf: []*huma.Schema{
					{Type: huma.TypeInteger},
					{Type: huma.TypeString},
				},
			},
			"error": errorObjectSchema,
		},
	}

	return responseObjectSchema
}
