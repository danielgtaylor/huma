package jsonrpc

import (
	"bytes"
	"encoding/json"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// checkForEmptyOrNullData checks if the data is zero or 'null' and returns a standardized error.
func checkForEmptyOrNullData(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return &JSONRPCError{
			Code:    ParseError,
			Message: "Received empty data",
		}
	}
	if bytes.Equal(data, []byte("null")) {
		return &JSONRPCError{
			Code:    ParseError,
			Message: "Received null data",
		}
	}
	return nil
}

// Generic function to unmarshal Meta structures
func unmarshalMeta[T any](data []byte, isBatch *bool, items *[]T) error {
	if err := checkForEmptyOrNullData(data); err != nil {
		return err
	}

	data = bytes.TrimSpace(data)
	// Try to unmarshal into []json.RawMessage to detect if it's a batch
	var rawMessages []json.RawMessage
	if err := json.Unmarshal(data, &rawMessages); err == nil {
		// Data is a batch
		*isBatch = true
		// Process each message in the batch, empty slice input is also ok and valid
		for _, msg := range rawMessages {
			// Empty or null single item also should not be present
			if err := checkForEmptyOrNullData(msg); err != nil {
				return err
			}
			var item T
			if err := json.Unmarshal(msg, &item); err != nil {
				return &JSONRPCError{
					Code:    ParseError,
					Message: "Failed to unmarshal batch item: " + err.Error(),
				}
			}
			*items = append(*items, item)
		}
	} else {
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return &JSONRPCError{
				Code:    ParseError,
				Message: "Failed to unmarshal single item: %s" + err.Error(),
			}
		}
		*isBatch = false
		*items = append(*items, item)
	}
	return nil
}

// Generic function to marshal Meta structures
func marshalMeta[T any](isBatch bool, items []T) ([]byte, error) {
	if isBatch {
		return json.Marshal(items)
	}
	if len(items) > 0 {
		return json.Marshal(items[0])
	}
	return nil, &JSONRPCError{Code: ParseError, Message: "Received empty input"}
}

func intPtr(i int) *int {
	return &i
}

// Meta is a generic struct to handle both MetaRequest and MetaResponse
type Meta[T any] struct {
	IsBatch bool `json:"-"`
	Items   []T
}

// UnmarshalJSON implements json.Unmarshaler for Meta[T]
func (m *Meta[T]) UnmarshalJSON(data []byte) error {
	m.Items = make([]T, 0)
	err := unmarshalMeta(data, &m.IsBatch, &m.Items)
	return err
}

// MarshalJSON implements json.Marshaler for Meta[T]
func (m Meta[T]) MarshalJSON() ([]byte, error) {
	return marshalMeta(m.IsBatch, m.Items)
}

func (m Meta[T]) Schema(r huma.Registry) *huma.Schema {
	// Get the type of the Items slice
	itemsType := reflect.TypeOf(m.Items)

	// Get the type of the element T
	elementType := itemsType.Elem()

	// Use the elementType to get the schema
	elementSchema := r.Schema(elementType, true, "")

	s := &huma.Schema{
		OneOf: []*huma.Schema{elementSchema, {
			Type:     huma.TypeArray,
			Items:    elementSchema,
			MinItems: intPtr(1),
		},
		},
	}
	return s
}

// Now, we can define MetaRequest and MetaResponse using Meta[T]
type MetaRequest struct {
	Body *Meta[Request[json.RawMessage]]
}

type MetaResponse struct {
	Body *Meta[Response[json.RawMessage]]
}
