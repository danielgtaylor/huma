package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
)

type contextKey string

const (
	ctxKeyRequestID      contextKey = "jsonrpcRequestID"
	ctxKeyMethodName     contextKey = "jsonrpcMethodName"
	ctxKeyIsNotification contextKey = "jsonrpcIsNotification"
)

// GetRequestID retrieves the RequestID from the context.
func GetRequestID(ctx context.Context) (RequestID, bool) {
	id, ok := ctx.Value(ctxKeyRequestID).(RequestID)
	return id, ok
}

// GetMethodName retrieves the MethodName from the context.
func GetMethodName(ctx context.Context) (string, bool) {
	method, ok := ctx.Value(ctxKeyMethodName).(string)
	return method, ok
}

// IsNotification checks if the request is a notification.
func IsNotification(ctx context.Context) bool {
	isNotification, ok := ctx.Value(ctxKeyIsNotification).(bool)
	return ok && isNotification
}

// Helper function to create context with request information.
func contextWithRequestInfo(
	parentCtx context.Context,
	methodName string,
	isNotification bool,
	requestID *RequestID,
) context.Context {
	ctx := context.WithValue(parentCtx, ctxKeyMethodName, methodName)
	ctx = context.WithValue(ctx, ctxKeyIsNotification, isNotification)
	if !isNotification && requestID != nil {
		ctx = context.WithValue(ctx, ctxKeyRequestID, *requestID)
	}
	return ctx
}

// Helper function to unmarshal parameters from the request.
func unmarshalParams[I any](req Request[json.RawMessage]) (I, error) {
	var params I
	if req.Params == nil {
		return params, nil
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return params, err
	}
	return params, nil
}

// Helper function to create an InvalidParamsError response
func invalidParamsResponse(req Request[json.RawMessage], err error) Response[json.RawMessage] {
	return Response[json.RawMessage]{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
		Error: &JSONRPCError{
			Code:    InvalidParamsError,
			Message: fmt.Sprintf("Invalid parameters: %v", err),
		},
	}
}
