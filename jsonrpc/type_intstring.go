package jsonrpc

import (
	"encoding/json"

	"errors"

	"github.com/danielgtaylor/huma/v2"
)

type IntString struct {
	Value interface{}
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (is *IntString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		// If the input is "null", return an error for non-pointer types
		// (UnmarshalJSON is called only for non-pointer types in this case)
		return errors.New("IntString cannot be null")
	}

	// Try to unmarshal data into an int
	var intValue int
	if err := json.Unmarshal(data, &intValue); err == nil {
		is.Value = intValue
		return nil
	}

	// Try to unmarshal data into a string
	var strValue string
	if err := json.Unmarshal(data, &strValue); err == nil {
		is.Value = strValue
		return nil
	}

	// If neither int nor string, return an error
	return errors.New("IntString must be a string or an integer")
}

// MarshalJSON implements the json.Marshaler interface.
func (is IntString) MarshalJSON() ([]byte, error) {
	switch v := is.Value.(type) {
	case int:
		return json.Marshal(v)
	case string:
		return json.Marshal(v)
	default:
		return nil, errors.New("IntString contains unsupported type")
	}
}

func (is IntString) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		OneOf: []*huma.Schema{
			{Type: huma.TypeInteger},
			{Type: huma.TypeString},
		},
	}
}

// Helper methods
func (is IntString) IsInt() bool {
	_, ok := is.Value.(int)
	return ok
}

func (is IntString) IsString() bool {
	_, ok := is.Value.(string)
	return ok
}

func (is IntString) IntValue() (int, bool) {
	v, ok := is.Value.(int)
	return v, ok
}

func (is IntString) StringValue() (string, bool) {
	v, ok := is.Value.(string)
	return v, ok
}

func (is *IntString) Equal(other *IntString) bool {
	// Handle nil cases
	if is == nil && other == nil {
		return true
	}
	if is == nil || other == nil {
		return false
	}
	// Compare the underlying values based on their types
	switch v := is.Value.(type) {
	case int:
		ov, ok := other.Value.(int)
		return ok && v == ov
	case string:
		ov, ok := other.Value.(string)
		return ok && v == ov
	default:
		return false
	}
}
