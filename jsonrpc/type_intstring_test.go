package jsonrpc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestIntString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantValue  interface{}
		wantIsInt  bool
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:      "Valid integer",
			input:     `123`,
			wantValue: 123,
			wantIsInt: true,
			wantErr:   false,
		},
		{
			name:      "Valid string",
			input:     `"hello"`,
			wantValue: "hello",
			wantIsInt: false,
			wantErr:   false,
		},
		{
			name:       "Invalid type (float)",
			input:      `123.45`,
			wantErr:    true,
			wantErrMsg: "IntString must be a string or an integer",
		},
		{
			name:       "Invalid type (boolean)",
			input:      `true`,
			wantErr:    true,
			wantErrMsg: "IntString must be a string or an integer",
		},
		{
			name:       "Null value",
			input:      `null`,
			wantErr:    true,
			wantErrMsg: "IntString cannot be null",
		},
		{
			name:       "Invalid JSON",
			input:      `{}`,
			wantErr:    true,
			wantErrMsg: "IntString must be a string or an integer",
		},
		{
			name:      "Empty string",
			input:     `""`,
			wantValue: "",
			wantIsInt: false,
			wantErr:   false,
		},
		{
			name:      "Negative integer",
			input:     `-42`,
			wantValue: -42,
			wantIsInt: true,
			wantErr:   false,
		},
		{
			name:      "Zero integer",
			input:     `0`,
			wantValue: 0,
			wantIsInt: true,
			wantErr:   false,
		},
		{
			name:      "String containing number",
			input:     `"123"`,
			wantValue: "123",
			wantIsInt: false,
			wantErr:   false,
		},
		{
			name:      "String containing special characters",
			input:     `"special_chars!@#$%^&*()"`,
			wantValue: "special_chars!@#$%^&*()",
			wantIsInt: false,
			wantErr:   false,
		},
		{
			name:      "String containing special characters html escaped",
			input:     `"special_chars!@#$%^\u0026*()"`,
			wantValue: "special_chars!@#$%^&*()",
			wantIsInt: false,
			wantErr:   false,
		},
		{
			name:      "Whitespace string",
			input:     `"   "`,
			wantValue: "   ",
			wantIsInt: false,
			wantErr:   false,
		},
		{
			name:      "Unicode string",
			input:     `"こんにちは"`,
			wantValue: "こんにちは",
			wantIsInt: false,
			wantErr:   false,
		},
		{
			name:       "Invalid JSON (missing quotes)",
			input:      `hello`,
			wantErr:    true,
			wantErrMsg: "invalid character",
		},
		{
			name:       "Array input",
			input:      `["hello", 123]`,
			wantErr:    true,
			wantErrMsg: "IntString must be a string or an integer",
		},
		{
			name:       "Object input",
			input:      `{"key": "value"}`,
			wantErr:    true,
			wantErrMsg: "IntString must be a string or an integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var is IntString
			err := json.Unmarshal([]byte(tt.input), &is)
			if (err != nil) != tt.wantErr {
				t.Logf("Got is: %v", is)
				t.Fatalf("IntString.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"IntString.UnmarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}

			if !reflect.DeepEqual(is.Value, tt.wantValue) {
				t.Errorf("IntString.Value = %v, want %v", is.Value, tt.wantValue)
			}

			if is.IsInt() != tt.wantIsInt {
				t.Errorf("IntString.IsInt() = %v, want %v", is.IsInt(), tt.wantIsInt)
			}

			if is.IsString() != !tt.wantIsInt {
				t.Errorf("IntString.IsString() = %v, want %v", is.IsString(), !tt.wantIsInt)
			}
		})
	}
}

func TestIntString_MarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		value      interface{}
		wantOutput string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "Integer value",
			value:      123,
			wantOutput: `123`,
			wantErr:    false,
		},
		{
			name:       "String value",
			value:      "hello",
			wantOutput: `"hello"`,
			wantErr:    false,
		},
		{
			name:       "Unsupported type (float)",
			value:      123.45,
			wantErr:    true,
			wantErrMsg: "IntString contains unsupported type",
		},
		{
			name:       "Unsupported type (boolean)",
			value:      true,
			wantErr:    true,
			wantErrMsg: "IntString contains unsupported type",
		},
		{
			name:       "Nil value",
			value:      nil,
			wantErr:    true,
			wantErrMsg: "IntString contains unsupported type",
		},
		{
			name:       "Empty string",
			value:      "",
			wantOutput: `""`,
			wantErr:    false,
		},
		{
			name:       "Negative integer",
			value:      -42,
			wantOutput: `-42`,
			wantErr:    false,
		},
		{
			name:       "Zero integer",
			value:      0,
			wantOutput: `0`,
			wantErr:    false,
		},
		{
			name:       "String containing number",
			value:      "123",
			wantOutput: `"123"`,
			wantErr:    false,
		},
		{
			name:  "String containing special characters",
			value: "special_chars!@#$%^&*()",
			// Need html escaped output
			wantOutput: `"special_chars!@#$%^\u0026*()"`,
			wantErr:    false,
		},
		{
			name:       "Whitespace string",
			value:      "   ",
			wantOutput: `"   "`,
			wantErr:    false,
		},
		{
			name:       "Unicode string",
			value:      "こんにちは",
			wantOutput: `"こんにちは"`,
			wantErr:    false,
		},
		{
			name:       "Unsupported type (slice)",
			value:      []int{1, 2, 3},
			wantErr:    true,
			wantErrMsg: "IntString contains unsupported type",
		},
		{
			name:       "Unsupported type (map)",
			value:      map[string]string{"key": "value"},
			wantErr:    true,
			wantErrMsg: "IntString contains unsupported type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := IntString{Value: tt.value}
			data, err := json.Marshal(is)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IntString.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if tt.wantErrMsg != "" && !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf(
						"IntString.MarshalJSON() error message = %v, want %v",
						err.Error(),
						tt.wantErrMsg,
					)
				}
				return
			}

			if string(data) != tt.wantOutput {
				t.Errorf(
					"IntString.MarshalJSON() output = %s, want %s",
					string(data),
					tt.wantOutput,
				)
			}
		})
	}
}

func TestIntString_HelperMethods(t *testing.T) {
	tests := []struct {
		name         string
		value        interface{}
		wantIsInt    bool
		wantIsString bool
		wantIntValue int
		wantStrValue string
	}{
		{
			name:         "Integer value",
			value:        123,
			wantIsInt:    true,
			wantIsString: false,
			wantIntValue: 123,
		},
		{
			name:         "String value",
			value:        "hello",
			wantIsInt:    false,
			wantIsString: true,
			wantStrValue: "hello",
		},
		{
			name:         "Nil value",
			value:        nil,
			wantIsInt:    false,
			wantIsString: false,
		},
		{
			name:         "Unsupported type (float)",
			value:        123.45,
			wantIsInt:    false,
			wantIsString: false,
		},
		{
			name:         "Unsupported type (boolean)",
			value:        true,
			wantIsInt:    false,
			wantIsString: false,
		},
		{
			name:         "Negative integer value",
			value:        -42,
			wantIsInt:    true,
			wantIsString: false,
			wantIntValue: -42,
		},
		{
			name:         "Empty string value",
			value:        "",
			wantIsInt:    false,
			wantIsString: true,
			wantStrValue: "",
		},
		{
			name:         "String containing number",
			value:        "123",
			wantIsInt:    false,
			wantIsString: true,
			wantStrValue: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := IntString{Value: tt.value}

			if is.IsInt() != tt.wantIsInt {
				t.Errorf("IntString.IsInt() = %v, want %v", is.IsInt(), tt.wantIsInt)
			}

			if is.IsString() != tt.wantIsString {
				t.Errorf("IntString.IsString() = %v, want %v", is.IsString(), tt.wantIsString)
			}

			intVal, ok := is.IntValue()
			if tt.wantIsInt != ok {
				t.Errorf("IntString.IntValue() ok = %v, want %v", ok, tt.wantIsInt)
			}
			if ok && intVal != tt.wantIntValue {
				t.Errorf("IntString.IntValue() = %v, want %v", intVal, tt.wantIntValue)
			}

			strVal, ok := is.StringValue()
			if tt.wantIsString != ok {
				t.Errorf("IntString.StringValue() ok = %v, want %v", ok, tt.wantIsString)
			}
			if ok && strVal != tt.wantStrValue {
				t.Errorf("IntString.StringValue() = %v, want %v", strVal, tt.wantStrValue)
			}
		})
	}
}

// Additional test to ensure proper error messages
func TestIntString_ErrorMessages(t *testing.T) {
	// Unmarshaling an array should return a specific error message
	data := `["hello", 123]`
	var is IntString
	err := json.Unmarshal([]byte(data), &is)
	if err == nil {
		t.Fatalf("Expected error when unmarshaling array, but got none")
	}
	expectedErrMsg := "IntString must be a string or an integer"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Error message = %v, want %v", err.Error(), expectedErrMsg)
	}

	// Marshaling an unsupported type should return a specific error message
	is = IntString{Value: []int{1, 2, 3}}
	_, err = json.Marshal(is)
	if err == nil {
		t.Fatalf("Expected error when marshaling unsupported type, but got none")
	}
	expectedErrMsg = "IntString contains unsupported type"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Error message = %v, want %v", err.Error(), expectedErrMsg)
	}
}
