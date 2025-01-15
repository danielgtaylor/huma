package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

// IMethodHandler	is an interface for handlers that process requests expecting a response.
type IMethodHandler interface {
	Handle(ctx context.Context, req Request[json.RawMessage]) (Response[json.RawMessage], error)
	GetTypes() (reflect.Type, reflect.Type)
}

// INotificationHandler is an interface for handlers that process notifications (no response expected).
type INotificationHandler interface {
	// Even though there is a error return allowed this is mainly present for any debugging logs etc in the server
	// The client will never receive any error for a notification
	Handle(ctx context.Context, req Request[json.RawMessage]) error
	GetTypes() reflect.Type
}

// GetMetaRequestHandler creates a handler function that processes MetaRequests.
func GetMetaRequestHandler(
	methodMap map[string]IMethodHandler,
	notificationMap map[string]INotificationHandler,
) func(context.Context, *MetaRequest) (*MetaResponse, error) {
	return func(ctx context.Context, metaReq *MetaRequest) (*MetaResponse, error) {
		if metaReq == nil || metaReq.Body == nil || len(metaReq.Body.Items) == 0 {
			item := Response[json.RawMessage]{
				JSONRPC: JSONRPCVersion,
				ID:      nil,
				Error: &JSONRPCError{
					Code:    ParseError,
					Message: "No input received for",
				},
			}
			// Return single error if invalid batch or even a single item cannot be found.
			ret := MetaResponse{
				Body: &Meta[Response[json.RawMessage]]{
					IsBatch: false,
					Items:   []Response[json.RawMessage]{item},
				},
			}
			return &ret, nil
		}

		resp := MetaResponse{
			Body: &Meta[Response[json.RawMessage]]{
				IsBatch: metaReq.Body.IsBatch,
				Items:   []Response[json.RawMessage]{},
			},
		}

		for _, request := range metaReq.Body.Items {
			// Need a valid JSONRPC version and method
			if request.JSONRPC != JSONRPCVersion || request.Method == "" {
				msg := fmt.Sprintf(
					"Invalid JSON-RPC version: '%s'",
					request.JSONRPC,
				)
				if request.Method == "" {
					msg = "Method name missing"
				}
				resp.Body.Items = append(resp.Body.Items, Response[json.RawMessage]{
					JSONRPC: JSONRPCVersion,
					ID:      request.ID,
					Error: &JSONRPCError{
						Code:    InvalidRequestError,
						Message: msg,
					},
				})
				continue
			}

			absentRequestID := request.ID == nil

			if absentRequestID {
				// Handle notification
				handler, ok := notificationMap[request.Method]
				if ok {
					// Create context with request info
					subCtx := contextWithRequestInfo(ctx, request.Method, true, nil)

					// Call the notification handler
					// Cannot return error; possibly log internally
					_ = handler.Handle(subCtx, request)
					// Notifications do not produce a response
					continue
				}

				// Notification not found, but requestid was nil
				// If it was a method, send a invalid request error. Else dont send anything.
				if _, ok = methodMap[request.Method]; ok {
					resp.Body.Items = append(resp.Body.Items, Response[json.RawMessage]{
						JSONRPC: JSONRPCVersion,
						ID:      nil,
						Error: &JSONRPCError{
							Code: InvalidRequestError,
							Message: fmt.Sprintf(
								"Received no requestID for method: '%s'",
								request.Method,
							),
						},
					})
				}

				continue
			}

			// Handle request expecting a response
			handler, ok := methodMap[request.Method]
			if !ok {
				// Method not found
				resp.Body.Items = append(resp.Body.Items, Response[json.RawMessage]{
					JSONRPC: JSONRPCVersion,
					ID:      request.ID,
					Error: &JSONRPCError{
						Code:    MethodNotFoundError,
						Message: fmt.Sprintf("Method '%s' not found", request.Method),
					},
				})
				continue
			}

			// Create context with request info
			subCtx := contextWithRequestInfo(ctx, request.Method, false, request.ID)

			// Call the method handler
			response, err := handler.Handle(subCtx, request)
			if err != nil {
				// Handler returned an error.
				// This should generally not happen as handler is expected to convert any errors into a jsonrpc response with error object
				resp.Body.Items = append(resp.Body.Items, Response[json.RawMessage]{
					JSONRPC: JSONRPCVersion,
					ID:      request.ID,
					Error: &JSONRPCError{
						Code:    InternalError,
						Message: fmt.Sprintf("Handler error: %v", err),
					},
				})
				continue
			}

			// Append the response
			resp.Body.Items = append(resp.Body.Items, response)
		}

		// If there are no responses to return, return nil response.
		if len(resp.Body.Items) == 0 {
			return nil, nil
		}

		return &resp, nil
	}
}
