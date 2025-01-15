package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// MethodHandler represents a generic handler with customizable input and output types.
// It generally expects a response to be returned to the client.
//
// Usage Scenarios:
//
//  1. Compulsory Parameters:
//     Use concrete types for both I and O when both input and output are required.
//
//  2. Optional Input or Output Parameters:
//     Use a pointer type for I or O to allow passing nil when no input or output is provided.
//
//  3. No Input or Output Parameters:
//     Use struct{} for I or O when the handler does not require any input or output.
//
// Example:
//
//	// Handler with no input and output
//	handler := MethodHandler[struct{}, struct{}]{
//	    Endpoint: func(ctx context.Context, _ struct{}) (struct{}, error) {
//	        // Implementation
//	        return struct{}{}, nil
//	    },
//	}
type MethodHandler[I any, O any] struct {
	Endpoint func(ctx context.Context, params I) (O, error)
}

// Handle processes a request expecting a response.
func (m *MethodHandler[I, O]) Handle(
	ctx context.Context,
	req Request[json.RawMessage],
) (Response[json.RawMessage], error) {
	params, err := unmarshalParams[I](req)
	if err != nil {
		// Return InvalidParamsError
		return invalidParamsResponse(req, err), nil
	}

	// Call the handler
	result, err := m.Endpoint(ctx, params)
	if err != nil {
		// Check if err is a *jsonrpc.Error (JSON-RPC error)
		var jsonrpcErr *JSONRPCError
		if errors.As(err, &jsonrpcErr) {
			// Handler returned a JSON-RPC error
			return Response[json.RawMessage]{
				JSONRPC: JSONRPCVersion,
				ID:      req.ID,
				Error:   jsonrpcErr,
			}, nil
		}
		// Handler returned a standard error
		return Response[json.RawMessage]{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    InternalError,
				Message: err.Error(),
			},
		}, nil
	}

	// Marshal the result.
	resultData, err := json.Marshal(result)
	if err != nil {
		return Response[json.RawMessage]{
			JSONRPC: JSONRPCVersion,
			ID:      req.ID,
			Error: &JSONRPCError{
				Code:    InternalError,
				Message: fmt.Sprintf("Error marshaling result: %v", err),
			},
		}, nil
	}

	// Return the response with the marshaled result
	return Response[json.RawMessage]{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Result:  json.RawMessage(resultData),
	}, nil
}

// GetTypes returns the reflect.Type of the input and output types.
func (m *MethodHandler[I, O]) GetTypes() (reflect.Type, reflect.Type) {
	iType := reflect.TypeOf((*I)(nil)).Elem()
	oType := reflect.TypeOf((*O)(nil)).Elem()
	return iType, oType
}
