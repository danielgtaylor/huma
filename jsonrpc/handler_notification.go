package jsonrpc

import (
	"context"
	"encoding/json"
	"reflect"
)

// NotificationHandler is a RPC handler for methods that do not expect a response.
//
// Usage Scenarios:
//
//  1. Compulsory Parameters:
//     Use concrete types for I when input is required.
//
//  2. Optional Input Parameters:
//     Use a pointer type for I to allow passing nil when no input is provided.
//
//  3. No Input Parameters:
//     Use struct{} for I when the handler does not require any input.
//
// Example:
//
//	// Handler with no input
//	handler := NotificationHandler[struct{}]{
//	    Endpoint: func(ctx context.Context, _ struct{}) error {
//	        // Implementation
//	        return nil
//	    },
//	}
type NotificationHandler[I any] struct {
	Endpoint func(ctx context.Context, params I) error
}

// Handle processes a notification (no response expected).
func (n *NotificationHandler[I]) Handle(ctx context.Context, req Request[json.RawMessage]) error {
	params, err := unmarshalParams[I](req)
	if err != nil {
		// Cannot send error to client in notification; possibly log internally
		return err
	}

	// Call the endpoint
	return n.Endpoint(ctx, params)
}

// GetTypes returns the reflect.Type of the input
func (m *NotificationHandler[I]) GetTypes() reflect.Type {
	return reflect.TypeOf((*I)(nil)).Elem()

}
