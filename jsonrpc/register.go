package jsonrpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// GetDefaultOperation gets the conventional values for jsonrpc as a single operation
func GetDefaultOperation() huma.Operation {

	return huma.Operation{
		Method:        http.MethodPost,
		Path:          "/jsonrpc",
		DefaultStatus: 200,

		Tags:        []string{"JSONRPC"},
		Summary:     "JSONRPC endpoint",
		Description: "Serve all jsonrpc methods",
		OperationID: "jsonrpc",
	}
}

// GetErrorHandler is a closure returning a function that converts any errors returned into a JSONRPC error
// response object. It implements the huma StatusError interface.
// IF the JSONRPC handler is invoked, it should never throw an error, but should return a error response object.
// JSONRPC requires a error case to be covered via the specifications error response object
func GetErrorHandler(
	methodMap map[string]IMethodHandler,
	notificationMap map[string]INotificationHandler,
) func(status int, message string, errs ...error) huma.StatusError {
	return func(gotStatus int, gotMessage string, errs ...error) huma.StatusError {
		var foundJSONRPCError *JSONRPCError
		message := gotMessage
		details := make([]string, 0)
		details = append(details, "Message: "+gotMessage)
		// Add the HTTP status to details and set status sent back as 200
		details = append(details, fmt.Sprintf("HTTP Status:%d", gotStatus))
		status := 200

		code := InternalError
		if gotStatus >= 400 && gotStatus < 500 {
			code = InvalidRequestError
			message = errorMessage[InvalidRequestError]
		}

		for _, err := range errs {
			if converted, ok := err.(huma.ErrorDetailer); ok {
				d := converted.ErrorDetail()
				// See if this is parse error
				if strings.Contains(d.Message, "unmarshal") ||
					strings.Contains(d.Message, "invalid character") ||
					strings.Contains(d.Message, "unexpected end") {
					code = ParseError
					message = errorMessage[ParseError]
				}
			} else if jsonRPCError, ok := err.(JSONRPCError); ok {
				// Check if the error is of type JSONRPCError
				foundJSONRPCError = &jsonRPCError
			}
			details = append(details, err.Error())
		}

		// If a JSONRPCError was found, update the message and append JSON-encoded details
		if foundJSONRPCError != nil {
			message = foundJSONRPCError.Message
			code = foundJSONRPCError.Code

			// JSON encode the Data field of the found JSONRPCError
			if jsonData, err := json.Marshal(foundJSONRPCError.Data); err == nil {
				details = append(details, string(jsonData))
			}
		}

		// Check for method not found
		if gotMessage == "validation failed" {
			// Assume that the method name is in one of the error messages
			// Look for "method:<methodName>"
			var methodName string
			for _, errMsg := range details {
				idx := strings.Index(errMsg, "method:")
				if idx != -1 {
					// Extract method name up to the next space or bracket or end of string
					rest := errMsg[idx+len("method:"):]
					endIdx := strings.IndexFunc(rest, func(r rune) bool {
						return r == ' ' || r == ']' || r == ')'
					})
					if endIdx == -1 {
						methodName = rest
					} else {
						methodName = rest[:endIdx]
					}
					break
				}
			}
			// Check if method exists in methodMap or notificationMap
			if methodName != "" {
				if _, exists := methodMap[methodName]; !exists {
					if _, exists := notificationMap[methodName]; !exists {
						// Method not found
						code = MethodNotFoundError // You need to define this constant
						message = fmt.Sprintf("Method '%s' not found", methodName)
					}
				}
			}
		}

		return &ResponseStatusError{
			status: status,
			Response: Response[any]{
				JSONRPC: JSONRPCVersion,
				ID:      nil,
				Error: &JSONRPCError{
					Code:    code,
					Message: message,
					Data:    details,
				},
			},
		}
	}
}

// Register a new JSONRPC operation.
// The `methodMap` maps from method name to request handlers. Request clients expect a response object
// The `notificationMap` maps from method name to notification handlers. Notification clients do not expect a response
//
// These maps can be instantiated as
//
//	methodMap := map[string]jsonrpc.IMethodHandler{
//		"add": &jsonrpc.MethodHandler[AddParams, int]{Endpoint: AddEndpoint},
//	}
//
//	notificationMap := map[string]jsonrpc.INotificationHandler{
//		"log": &jsonrpc.NotificationHandler[LogParams]{Endpoint: LogEndpoint},
//	}
func Register(
	api huma.API,
	op huma.Operation,
	methodMap map[string]IMethodHandler,
	notificationMap map[string]INotificationHandler,
) {
	AddSchemasToAPI(api, methodMap, notificationMap)
	huma.NewError = GetErrorHandler(methodMap, notificationMap)
	reqHandler := GetMetaRequestHandler(methodMap, notificationMap)
	huma.Register(api, op, reqHandler)
}
