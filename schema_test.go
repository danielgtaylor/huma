package huma_test

import (
	"bytes"
	"encoding"
	"encoding/json"
	"math/bits"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
)

type RecursiveChildKey struct {
	Key  string             `json:"key"`
	Self *RecursiveChildKey `json:"self,omitempty"`
}

type RecursiveChild struct {
	RecursiveChildLoop
}

type RecursiveChildLoop struct {
	*RecursiveChild
	Slice   []*RecursiveChildLoop                    `json:"slice"`
	Array   [1]*RecursiveChildLoop                   `json:"array"`
	Map     map[RecursiveChildKey]RecursiveChildLoop `json:"map"`
	ByValue RecursiveChildKey                        `json:"byValue"`
	ByRef   *RecursiveChildKey                       `json:"byRef"`
}

type EmbeddedChild struct {
	// This one should be ignored as it is overridden by `Embedded`.
	Value string `json:"value" doc:"old doc"`
}

type Embedded struct {
	EmbeddedChild
	Value string `json:"value" doc:"new doc"`
}

type CustomSchema struct{}

func (c CustomSchema) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type: "string",
	}
}

var _ huma.SchemaProvider = CustomSchema{}

type BadRefSchema struct{}

func (c BadRefSchema) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Ref: "bad",
	}
}

var _ huma.SchemaProvider = BadRefSchema{}

type TypedArrayWithCustomDesc [4]float64

func (t *TypedArrayWithCustomDesc) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	s.Description = "custom description"
	return s
}

var _ huma.SchemaTransformer = (*CustomSchemaPtr)(nil)

type CustomSchemaPtr struct {
	Value string `json:"value"`
}

func (c *CustomSchemaPtr) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	s.Description = "custom description"
	return s
}

type TypedStringWithCustomLength string

func (c TypedStringWithCustomLength) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{
		Type:      "string",
		MinLength: Ptr(1),
		MaxLength: Ptr(10),
	}
}

type TypedIntegerWithCustomLimits int

func (c *TypedIntegerWithCustomLimits) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	s.Minimum = Ptr(float64(1))
	s.Maximum = Ptr(float64(10))
	return s
}

func TestSchema(t *testing.T) {
	bitSize := strconv.Itoa(bits.UintSize)

	cases := []struct {
		name     string
		input    any
		expected string
		panics   string
	}{
		{
			name:     "bool",
			input:    true,
			expected: `{"type": "boolean"}`,
		},
		{
			name:     "bool-pointer",
			input:    Ptr(true),
			expected: `{"type": ["boolean", "null"]}`,
		},
		{
			name:     "int",
			input:    1,
			expected: `{"type": "integer", "format": "int` + bitSize + `"}`,
		},
		{
			name:     "int32",
			input:    int32(1),
			expected: `{"type": "integer", "format": "int32"}`,
		},
		{
			name:     "int64",
			input:    int64(1),
			expected: `{"type": "integer", "format": "int64"}`,
		},
		{
			name:     "uint",
			input:    uint(1),
			expected: `{"type": "integer", "format": "int` + bitSize + `", "minimum": 0}`,
		},
		{
			name:     "uint32",
			input:    uint32(1),
			expected: `{"type": "integer", "format": "int32", "minimum": 0}`,
		},
		{
			name:     "uint64",
			input:    uint64(1),
			expected: `{"type": "integer", "format": "int64", "minimum": 0}`,
		},
		{
			name:     "float64",
			input:    1.0,
			expected: `{"type": "number", "format": "double"}`,
		},
		{
			name:     "float32",
			input:    float32(1.0),
			expected: `{"type": "number", "format": "float"}`,
		},
		{
			name:     "string",
			input:    "test",
			expected: `{"type": "string"}`,
		},
		{
			name:     "time",
			input:    time.Now(),
			expected: `{"type": "string", "format": "date-time"}`,
		},
		{
			name:     "time-pointer",
			input:    Ptr(time.Now()),
			expected: `{"type": ["string", "null"], "format": "date-time"}`,
		},
		{
			name:     "url",
			input:    url.URL{},
			expected: `{"type": "string", "format": "uri"}`,
		},
		{
			name:     "ip",
			input:    net.IPv4(127, 0, 0, 1),
			expected: `{"type": "string", "format": "ipv4"}`,
		},
		{
			name:     "ipAddr",
			input:    netip.AddrFrom4([4]byte{127, 0, 0, 1}),
			expected: `{"type": "string", "format": "ip"}`,
		},
		{
			name:     "json.RawMessage",
			input:    &json.RawMessage{},
			expected: `{}`,
		},
		{
			name:     "bytes",
			input:    []byte("test"),
			expected: `{"type": "string", "contentEncoding": "base64"}`,
		},
		{
			name:     "array",
			input:    [2]int{1, 2},
			expected: `{"type": ["array", "null"], "items": {"type": "integer", "format": "int64"}, "minItems": 2, "maxItems": 2}`,
		},
		{
			name:     "slice",
			input:    []int{1, 2, 3},
			expected: `{"type": ["array", "null"], "items": {"type": "integer", "format": "int64"}}`,
		},
		{
			name:     "map",
			input:    map[string]string{"foo": "bar"},
			expected: `{"type": "object", "additionalProperties": {"type": "string"}}`,
		},
		{
			name: "additionalProps",
			input: struct {
				_     struct{} `json:"-" additionalProperties:"true"`
				Value string   `json:"value"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string"
					}
				},
				"required": ["value"],
				"additionalProperties": true
			}`,
		},
		{
			name: "field-int",
			input: struct {
				Value int `json:"value" minimum:"1" exclusiveMinimum:"0" maximum:"10" exclusiveMaximum:"11" multipleOf:"2"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "integer",
						"format": "int64",
						"minimum": 1,
						"exclusiveMinimum": 0,
						"maximum": 10,
						"exclusiveMaximum": 11,
						"multipleOf": 2
					}
				},
				"required": ["value"],
				"additionalProperties": false
			}`,
		},
		{
			name: "field-string",
			input: struct {
				Value string `json:"value" minLength:"1" maxLength:"10" pattern:"^foo$" format:"foo" encoding:"bar"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
						"minLength": 1,
						"maxLength": 10,
						"pattern": "^foo$",
						"format": "foo",
						"contentEncoding": "bar"
					}
				},
				"required": ["value"],
				"additionalProperties": false
			}`,
		},
		{
			name: "field-array",
			input: struct {
				Value []int `json:"value" minItems:"1" maxItems:"10" uniqueItems:"true"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": ["array", "null"],
						"minItems": 1,
						"maxItems": 10,
						"uniqueItems": true,
						"items": {"type": "integer", "format": "int64"}
					}
				},
				"required": ["value"],
				"additionalProperties": false
			}`,
		},
		{
			name: "field-map",
			input: struct {
				Value map[string]string `json:"value" minProperties:"2" maxProperties:"5"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "object",
						"minProperties": 2,
						"maxProperties": 5,
						"additionalProperties": {
							"type": "string"
						}
					}
				},
				"required": ["value"],
				"additionalProperties": false
			}`,
		},
		{
			name: "field-enum",
			input: struct {
				Value string `json:"value" enum:"one,two"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
						"enum": ["one", "two"]
					}
				},
				"required": ["value"],
				"additionalProperties": false
			}`,
		},
		{
			name: "field-array-enum",
			input: struct {
				Value []int `json:"value" enum:"1,2,3,5,8,11"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": ["array", "null"],
						"items": {
							"type": "integer",
							"format": "int64",
							"enum": [1, 2, 3, 5, 8, 11]
						}
					}
				},
				"required": ["value"],
				"additionalProperties": false
			}`,
		},
		{
			name: "field-readonly",
			input: struct {
				Value string `json:"value" readOnly:"true" writeOnly:"false"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
						"readOnly": true
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-readonly-struct",
			input: struct {
				Value struct {
					Foo string `json:"foo"`
				} `json:"value" readOnly:"true"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"$ref": "#/components/schemas/ValueStruct",
						"readOnly": true
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-default-string",
			input: struct {
				Value string `json:"value" default:"foo"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
						"default": "foo"
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-default-string-pointer",
			input: struct {
				Value *string `json:"value,omitempty" default:"foo"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
						"default": "foo"
					}
				},
				"additionalProperties": false
			}`,
		},
		{
			name: "field-default-array-string",
			input: struct {
				Value []string `json:"value" default:"foo,bar"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": ["array", "null"],
						"items": {
							"type": "string"
						},
						"default": ["foo", "bar"]
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-default-array-int",
			input: struct {
				Value []int `json:"value" default:"[1,2]"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": ["array", "null"],
						"items": {
							"type": "integer",
							"format": "int64"
						},
						"default": [1, 2]
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-default-duration",
			input: struct {
				Value time.Duration `json:"value" default:"5000"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "integer",
						"format": "int64",
						"default": 5000
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-optional-without-name",
			input: struct {
				Value string `json:",omitempty"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"Value": {
						"type": "string"
					}
				},
				"additionalProperties": false
			}`,
		},
		{
			name: "field-optional-with-omitempty",
			input: struct {
				Value string `json:"value,omitempty"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string"
					}
				},
				"additionalProperties": false
			}`,
		},
		{
			name: "field-optional-with-omitzero",
			input: struct {
				Value string `json:"value,omitzero"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string"
					}
				},
				"additionalProperties": false
			}`,
		},
		{
			name: "field-example-custom",
			input: struct {
				Value CustomSchema `json:"value" example:"foo"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
						"examples": ["foo"]
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-example-custom-pointer",
			input: struct {
				Value *CustomSchema `json:"value" example:"foo"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
						"examples": ["foo"]
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-enum-custom",
			input: struct {
				Value OmittableNullable[string] `json:"value,omitempty" enum:"foo,bar"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": ["string", "null"],
						"enum": ["foo", "bar"]
					}
				},
				"additionalProperties": false
			}`,
		},
		{
			name: "field-any",
			input: struct {
				Value any `json:"value" doc:"Some value"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"description": "Some value"
					}
				},
				"additionalProperties": false,
				"required": ["value"]
			}`,
		},
		{
			name: "field-dependent-required",
			input: struct {
				Value     string `json:"value,omitempty" dependentRequired:"dependent"`
				Dependent string `json:"dependent,omitempty"`
				Ignored   string `json:"ignored,omitempty" dependentRequired:""`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string"
					},
					"dependent": {
						"type": "string"
					},
					"ignored": {
						"type": "string"
					}
				},
				"dependentRequired": {
					"value": ["dependent"]
				},
				"additionalProperties": false
			}`,
		},
		{
			// Bad ref should not panic, but should be ignored. These could be valid
			// custom schemas that Huma won't understand.
			name: "field-custom-bad-ref",
			input: struct {
				Value  BadRefSchema `json:"value" example:"true"`
				Value2 struct {
					Foo BadRefSchema `json:"foo"`
				} `json:"value2" example:"{\"foo\": true}"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"$ref": "bad"
					},
					"value2": {
						"$ref": "#/components/schemas/Value2Struct",
						"examples": [{"foo": true}]
					}
				},
				"additionalProperties": false,
				"required": ["value", "value2"]
			}`,
		},
		{
			name: "field-skip",
			input: struct {
				// Not filtered out (just a normal field)
				Value1 string `json:"value1"`
				// Filtered out from JSON tag
				Value2 string `json:"-"`
				// Filtered because it's private
				value3 string
				// Filtered due to being an unsupported type
				Value4 func()
				// Filtered due to being hidden
				Value5 string `json:"value4,omitempty" hidden:"true"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"required": ["value1"],
				"properties": {
					"value1": {
						"type": "string"
					}
				}
			}`,
		},
		{
			name: "field-embed",
			input: struct {
				// Because this is embedded, the fields should be merged into
				// the parent object.
				*Embedded
				Value2 string `json:"value2"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"required": ["value2", "value"],
				"properties": {
					"value": {
						"type": "string",
						"description": "new doc"
					},
					"value2": {
						"type": "string"
					}
				}
			}`,
		},
		{
			name: "field-embed-override",
			input: struct {
				Embedded
				Value string `json:"override" doc:"override"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"required": ["override"],
				"properties": {
					"override": {
						"type": "string",
						"description": "override"
					}
				}
			}`,
		},
		{
			name: "field-pointer-example",
			input: struct {
				Int *int64  `json:"int" example:"123"`
				Str *string `json:"str" example:"foo"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"int": {
						"type": ["integer", "null"],
						"format": "int64",
						"examples": [123]
					},
					"str": {
						"type": ["string", "null"],
						"examples": ["foo"]
					}
				},
				"required": ["int", "str"]
			}`,
		},
		{
			name: "field-nullable",
			input: struct {
				Int *int64 `json:"int" nullable:"true"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"int": {
						"type": ["integer", "null"],
						"format": "int64"
					}
				},
				"required": ["int"]
			}`,
		},
		{
			name: "field-nullable-array",
			input: struct {
				Int []int64 `json:"int" nullable:"true"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"int": {
						"type": ["array", "null"],
						"items": {
							"type": "integer",
							"format": "int64"
						}
					}
				},
				"required": ["int"]
			}`,
		},
		{
			name: "field-non-nullable-array",
			input: struct {
				Int []int64 `json:"int" nullable:"false"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"int": {
						"type": "array",
						"items": {
							"type": "integer",
							"format": "int64"
						}
					}
				},
				"required": ["int"]
			}`,
		},
		{
			name: "field-nullable-struct",
			input: struct {
				Field struct {
					_   struct{} `json:"-" nullable:"true"`
					Foo string   `json:"foo"`
				} `json:"field"`
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false,
				"properties": {
					"field": {
						"$ref": "#/components/schemas/FieldStruct"
					}
				},
				"required": ["field"]
			}`,
		},
		{
			name:  "recursive-embedded-structure",
			input: RecursiveChild{},
			expected: `{
				"additionalProperties":false,
				"properties":{
					"array":{
						"items":{
							"$ref":"#/components/schemas/RecursiveChildLoop"
						},
						"maxItems":1,
						"minItems":1,
						"type":["array", "null"]
					},
					"byRef":{
						"$ref":"#/components/schemas/RecursiveChildKey"
					},
					"byValue":{
						"$ref":"#/components/schemas/RecursiveChildKey"
					},
					"map":{
						"additionalProperties":{
							"$ref":"#/components/schemas/RecursiveChildLoop"
						},
						"type":"object"
					},
					"slice":{
						"items":{
							"$ref":"#/components/schemas/RecursiveChildLoop"
						},
						"type":["array", "null"]}
					},
					"required":["slice","array","map","byValue", "byRef"],
					"type":"object"
				}`,
		},
		{
			name: "panic-bool",
			input: struct {
				Value string `json:"value" readOnly:"bad"`
			}{},
			panics: "invalid bool tag 'readOnly' for field 'Value': bad",
		},
		{
			name: "panic-int",
			input: struct {
				Value string `json:"value" minLength:"bad"`
			}{},
			panics: "invalid int tag 'minLength' for field 'Value': bad (strconv.Atoi: parsing \"bad\": invalid syntax)",
		},
		{
			name: "panic-float",
			input: struct {
				Value int `json:"value" minimum:"bad"`
			}{},
			panics: "invalid float tag 'minimum' for field 'Value': bad (strconv.ParseFloat: parsing \"bad\": invalid syntax)",
		},
		{
			name: "panic-json",
			input: struct {
				Value int `json:"value" default:"bad"`
			}{},
			panics: `invalid integer tag value 'bad' for field 'Value': invalid character 'b' looking for beginning of value`,
		},
		{
			name: "panic-json-bool",
			input: struct {
				Value bool `json:"value" default:"123"`
			}{},
			panics: `invalid boolean tag value '123' for field 'Value': schema is invalid`,
		},
		{
			name: "panic-json-int",
			input: struct {
				Value int `json:"value" default:"true"`
			}{},
			panics: `invalid number tag value 'true' for field 'Value': schema is invalid`,
		},
		{
			name: "panic-json-int2",
			input: struct {
				Value int `json:"value" default:"1.23"`
			}{},
			panics: `invalid integer tag value '1.23' for field 'Value': schema is invalid`,
		},
		{
			name: "panic-json-array",
			input: struct {
				Value []int `json:"value" default:"true"`
			}{},
			panics: `invalid array tag value 'true' for field 'Value': schema is invalid`,
		},
		{
			name: "panic-json-array-value",
			input: struct {
				Value []string `json:"value" default:"[true]"`
			}{},
			panics: `invalid string tag value 'true' for field 'Value[0]': schema is invalid`,
		},
		{
			name: "panic-json-array-value",
			input: struct {
				Value []int `json:"value" default:"[true]"`
			}{},
			panics: `invalid number tag value 'true' for field 'Value[0]': schema is invalid`,
		},
		{
			name: "panic-json-object",
			input: struct {
				Value struct {
					Foo string `json:"foo"`
				} `json:"value" default:"true"`
			}{},
			panics: `invalid object tag value 'true' for field 'Value': schema is invalid`,
		},
		{
			name: "panic-json-object-field",
			input: struct {
				Value struct {
					Foo string `json:"foo"`
				} `json:"value" default:"{\"foo\": true}"`
			}{},
			panics: `invalid string tag value 'true' for field 'Value.foo': schema is invalid`,
		},
		{
			name: "panic-dependent-required",
			input: struct {
				Value1    string `json:"value1,omitempty" dependentRequired:"missing1,missing2"`
				Value2    string `json:"value2,omitempty" dependentRequired:"missing2"`
				Value3    string `json:"value3,omitempty" dependentRequired:"dependent"`
				Dependent string `json:"dependent,omitempty"`
			}{},
			panics: `dependent field 'missing1' for field 'value1' does not exist; dependent field 'missing2' for field 'value1' does not exist; dependent field 'missing2' for field 'value2' does not exist`,
		},
		{
			name: "panic-nullable-struct",
			input: struct {
				Value *struct {
					Foo string `json:"foo"`
				} `json:"value" nullable:"true"`
			}{},
			panics: `nullable is not supported for field 'Value' which is type '#/components/schemas/ValueStruct'`,
		},
		{
			name: "field-custom-length-string-in-slice",
			input: struct {
				Values []TypedStringWithCustomLength `json:"values"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"values":{
						"type":["array", "null"],
						"items":{
							"type":"string",
							"minLength":1,
							"maxLength":10
						}
					}
				},
				"required":["values"],
				"type":"object"
			}`,
		},
		{
			name: "field-custom-length-string",
			input: struct {
				Value TypedStringWithCustomLength `json:"value"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"value":{
						"type":"string",
						"minLength":1,
						"maxLength":10
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
		{
			name: "field-custom-length-string-with-tag",
			input: struct {
				Value TypedStringWithCustomLength `json:"value" maxLength:"20"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"value":{
						"type":"string",
						"minLength":1,
						"maxLength":20
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
		{
			name: "field-ptr-to-custom-length-string",
			input: struct {
				Value *TypedStringWithCustomLength `json:"value"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"value":{
						"type":"string",
						"minLength":1,
						"maxLength":10
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
		{
			name: "field-ptr-to-custom-length-string-with-tag",
			input: struct {
				Value *TypedStringWithCustomLength `json:"value" minLength:"0"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"value":{
						"type":"string",
						"minLength":0,
						"maxLength":10
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
		{
			name: "field-custom-limits-int",
			input: struct {
				Value TypedIntegerWithCustomLimits `json:"value"`
			}{},
			expected: ` {
					"additionalProperties":false,
					"properties":{
						"value":{
							"type":"integer",
							"format":"int64",
							"minimum":1,
							"maximum":10
						}
					},
					"required":["value"],
					"type":"object"
				}`,
		},
		{
			name: "field-custom-limits-int-with-tag",
			input: struct {
				Value TypedIntegerWithCustomLimits `json:"value" minimum:"2"`
			}{},
			expected: ` {
					"additionalProperties":false,
					"properties":{
						"value":{
							"type":"integer",
							"format":"int64",
							"minimum":2,
							"maximum":10
						}
					},
					"required":["value"],
					"type":"object"
				}`,
		},
		{
			name: "field-ptr-to-custom-limits-int",
			input: struct {
				Value *TypedIntegerWithCustomLimits `json:"value"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"value":{
						"format":"int64",
						"type": ["integer", "null"],
						"minimum":1,
						"maximum":10
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
		{
			name: "field-custom-array",
			input: struct {
				Value TypedArrayWithCustomDesc `json:"value"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"value":{
						"description":"custom description",
						"items":{
							"format":"double",
							"type":"number"
						},
						"maxItems":4,
						"minItems":4,
						"type":["array", "null"]
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
		{
			name: "field-ptr-to-custom-array",
			input: struct {
				Value *TypedArrayWithCustomDesc `json:"value"`
			}{},
			expected: ` {
				"additionalProperties":false,
				"properties":{
					"value":{
						"description":"custom description",
						"items":{
							"format":"double",
							"type":"number"
						},
						"maxItems":4,
						"minItems":4,
						"type":["array", "null"]
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
		{
			name:  "schema-transformer-for-ptr",
			input: &CustomSchemaPtr{},
			expected: ` {
				"additionalProperties":false,
				"description":"custom description",
				"properties":{
					"value":{
						"type":"string"
					}
				},
				"required":["value"],
				"type":"object"
			}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)

			if c.panics != "" {
				assert.PanicsWithError(t, c.panics, func() {
					r.Schema(reflect.TypeOf(c.input), false, "")
				})
			} else {
				s := r.Schema(reflect.TypeOf(c.input), false, "")
				b, _ := json.Marshal(s)
				assert.JSONEq(t, c.expected, string(b))
			}
		})
	}
}

type GreetingInput struct {
	ID string `path:"id"`
}

type TestInputSub struct {
	Num int `json:"num" minimum:"1"`
}

type TestInput struct {
	Name string       `json:"name" minLength:"1"`
	Sub  TestInputSub `json:"sub"`
}

type RecursiveInput struct {
	Value *RecursiveInput
}

func TestSchemaOld(t *testing.T) {
	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)

	s := r.Schema(reflect.TypeOf(GreetingInput{}), false, "")
	assert.Equal(t, "object", s.Type)
	assert.Len(t, s.Properties, 1)
	assert.Equal(t, "string", s.Properties["ID"].Type)

	r.Schema(reflect.TypeOf(RecursiveInput{}), false, "")

	s2 := r.Schema(reflect.TypeOf(TestInput{}), false, "")
	pb := huma.NewPathBuffer(make([]byte, 0, 128), 0)
	res := huma.ValidateResult{}
	huma.Validate(r, s2, pb, huma.ModeReadFromServer, map[string]any{
		"name": "foo",
		"sub": map[string]any{
			"num": 1.0,
		},
	}, &res)
	assert.Empty(t, res.Errors)
}

func TestSchemaGenericNaming(t *testing.T) {
	type SchemaGeneric[T any] struct {
		Value T `json:"value"`
	}

	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(SchemaGeneric[int]{}), true, "")

	b, _ := json.Marshal(s)
	assert.JSONEq(t, `{
		"$ref": "#/components/schemas/SchemaGenericInt"
	}`, string(b))
}

func TestSchemaGenericNamingFromModule(t *testing.T) {
	type SchemaGeneric[T any] struct {
		Value T `json:"value"`
	}

	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(SchemaGeneric[time.Time]{}), true, "")

	b, _ := json.Marshal(s)
	assert.JSONEq(t, `{
		"$ref": "#/components/schemas/SchemaGenericTime"
	}`, string(b))
}

type MyDate time.Time

func (d *MyDate) UnmarshalText(data []byte) error {
	t, err := time.Parse(time.RFC3339, string(data))
	if err != nil {
		return err
	}
	*d = MyDate(t)
	return nil
}

var _ encoding.TextUnmarshaler = (*MyDate)(nil)

func TestCustomDateType(t *testing.T) {
	type O struct {
		Date MyDate `json:"date"`
	}

	var o O
	err := json.Unmarshal([]byte(`{"date": "2022-01-01T00:00:00Z"}`), &o)
	require.NoError(t, err)
	assert.Equal(t, MyDate(time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)), o.Date)

	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(o), false, "")
	assert.Equal(t, "string", s.Properties["date"].Type)
}

type OmittableNullable[T any] struct {
	Sent  bool
	Null  bool
	Value T
}

func (o *OmittableNullable[T]) UnmarshalJSON(b []byte) error {
	if len(b) > 0 {
		o.Sent = true
		if bytes.Equal(b, []byte("null")) {
			o.Null = true
			return nil
		}
		return json.Unmarshal(b, &o.Value)
	}
	return nil
}

func (o OmittableNullable[T]) Schema(r huma.Registry) *huma.Schema {
	s := r.Schema(reflect.TypeOf(o.Value), true, "")
	s.Nullable = true
	return s
}

func TestCustomUnmarshalType(t *testing.T) {
	type O struct {
		Field OmittableNullable[int] `json:"field" maximum:"10" example:"5"`
	}

	var o O

	// Confirm the schema is generated properly, including field constraints.
	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(o), false, "")
	assert.Equal(t, "integer", s.Properties["field"].Type, s)
	assert.Equal(t, Ptr(float64(10)), s.Properties["field"].Maximum, s)
	assert.InDelta(t, float64(5), s.Properties["field"].Examples[0], 0, s.Properties["field"])

	// Confirm the field works as expected when loading JSON.
	o = O{}
	err := json.Unmarshal([]byte(`{"field": 123}`), &o)
	require.NoError(t, err)
	assert.True(t, o.Field.Sent)
	assert.False(t, o.Field.Null)
	assert.Equal(t, 123, o.Field.Value)

	o = O{}
	err = json.Unmarshal([]byte(`{"field": null}`), &o)
	require.NoError(t, err)
	assert.True(t, o.Field.Sent)
	assert.True(t, o.Field.Null)
	assert.Equal(t, 0, o.Field.Value)

	o = O{}
	err = json.Unmarshal([]byte(`{}`), &o)
	require.NoError(t, err)
	assert.False(t, o.Field.Sent)
	assert.False(t, o.Field.Null)
	assert.Equal(t, 0, o.Field.Value)
}

func TestMarshalDiscriminator(t *testing.T) {
	s := &huma.Schema{
		OneOf: []*huma.Schema{
			{Type: "object", Properties: map[string]*huma.Schema{
				"type": {Type: "string", Enum: []any{"foo"}},
				"foo":  {Type: "string"},
			}},
			{Type: "object", Properties: map[string]*huma.Schema{
				"type": {Type: "string", Enum: []any{"bar"}},
				"bar":  {Type: "string"},
			}},
		},
		Discriminator: &huma.Discriminator{
			PropertyName: "type",
			Mapping: map[string]string{
				"foo": "#/components/schemas/Foo",
				"bar": "#/components/schemas/Bar",
			},
		},
	}

	b, _ := json.Marshal(s)
	assert.JSONEq(t, `{
		"oneOf": [
			{
				"type": "object",
				"properties": {
					"type": {"type": "string", "enum": ["foo"]},
					"foo": {"type": "string"}
				}
			},
			{
				"type": "object",
				"properties": {
					"type": {"type": "string", "enum": ["bar"]},
					"bar": {"type": "string"}
				}
			}
		],
		"discriminator": {
			"propertyName": "type",
			"mapping": {
				"foo": "#/components/schemas/Foo",
				"bar": "#/components/schemas/Bar"
			}
		}
	}`, string(b))
}

func TestSchemaArrayNotNullable(t *testing.T) {
	huma.DefaultArrayNullable = false
	defer func() {
		huma.DefaultArrayNullable = true
	}()

	type Value struct {
		Field []string `json:"field"`
	}

	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(Value{}), false, "")

	assert.Equal(t, "array", s.Properties["field"].Type)
}

type BenchSub struct {
	Visible bool      `json:"visible" default:"true"`
	Metrics []float64 `json:"metrics" maxItems:"31"`
}

type BenchStruct struct {
	Name   string    `json:"name" minLength:"1"`
	Code   string    `json:"code" pattern:"^[a-z]{3}-[0-9]+$"`
	Count  uint      `json:"count" maximum:"10"`
	Rating float32   `json:"rating" minimum:"0" maximum:"5"`
	Region string    `json:"region,omitempty" enum:"east,west"`
	Labels []string  `json:"labels,omitempty" maxItems:"5" uniqueItems:"true"`
	Sub    *BenchSub `json:"sub,omitempty"`
}

func BenchmarkSchema(b *testing.B) {
	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)

	s2 := r.Schema(reflect.TypeOf(BenchStruct{}), false, "")

	// data, _ := json.MarshalIndent(r.Map(), "", "  ")
	// fmt.Println(string(data))

	input := map[string]interface{}{
		"name":   "foo",
		"code":   "bar-123",
		"count":  8,
		"rating": 3.5,
		"region": "west",
		"labels": []any{"a", "b"},
		"sub": map[string]any{
			"visible": true,
			"metrics": []any{1.0, 2.0, 3.0},
		},
	}
	pb := huma.NewPathBuffer(make([]byte, 0, 128), 0)
	res := huma.ValidateResult{}
	huma.Validate(r, s2, pb, huma.ModeReadFromServer, input, &res)
	assert.Empty(b, res.Errors)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pb.Reset()
		res.Reset()
		huma.Validate(r, s2, pb, huma.ModeReadFromServer, input, &res)
		if len(res.Errors) > 0 {
			b.Fatal(res.Errors)
		}
	}
}

func BenchmarkSchemaErrors(b *testing.B) {
	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)

	s2 := r.Schema(reflect.TypeOf(BenchStruct{}), false, "")

	input := map[string]any{
		"name":   true,
		"code":   "wrong",
		"count":  20,
		"rating": 5.5,
		"region": "error",
		"labels": []any{"dupe", "dupe"},
		"sub": map[string]any{
			"visible":    1,
			"unexpected": 2,
		},
	}
	pb := huma.NewPathBuffer(make([]byte, 0, 128), 0)
	res := huma.ValidateResult{}
	huma.Validate(r, s2, pb, huma.ModeReadFromServer, input, &res)
	assert.NotEmpty(b, res.Errors)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pb.Reset()
		res.Reset()
		huma.Validate(r, s2, pb, huma.ModeReadFromServer, input, &res)
		if len(res.Errors) == 0 {
			b.Fatal("expected error")
		}
	}
}

// Struct that defines schemas for its property, to be reused by a SchemaTransformer
type ExampleInputStruct struct {
	Name    string `json:"name" minLength:"2" example:"Jane Doe"`
	Email   string `json:"email" format:"email" doc:"Contact e-mail address"`
	Age     *int   `json:"age,omitempty" minimum:"0"`
	Comment string `json:"comment,omitempty" maxLength:"256"`
	Pattern string `json:"pattern" pattern:"^[a-z]+$"`
}

// Implements SchemaTransformer interface, reusing parts of the schema from `ExampleInputStruct`
type ExampleUpdateStruct struct {
	Name    *string                   `json:"name"`
	Email   *string                   `json:"email" doc:"Override doc for email"`
	Age     OmittableNullable[int]    `json:"age"`
	Comment OmittableNullable[string] `json:"comment"`
	Pattern string                    `json:"pattern"`
}

func (u *ExampleUpdateStruct) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	inputSchema := r.Schema(reflect.TypeOf((*ExampleInputStruct)(nil)), false, "")
	for propName, schema := range s.Properties {
		propSchema := inputSchema.Properties[propName]
		if schema.Description != "" {
			propSchema.Description = schema.Description
		}
		propSchema.Nullable = schema.Nullable
		s.Properties[propName] = propSchema
	}
	s.Required = []string{} // make everything optional
	return s
}

func TestSchemaTransformer(t *testing.T) {
	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	inputSchema := r.Schema(reflect.TypeOf((*ExampleInputStruct)(nil)), false, "")
	validateSchema := func(s *huma.Schema) {
		if s.Ref != "" {
			s = r.SchemaFromRef(s.Ref)
		}
		assert.Equal(t, inputSchema.Properties["name"].Examples, s.Properties["name"].Examples)
		assert.Equal(t, "Override doc for email", s.Properties["email"].Description)
		assert.Equal(t, inputSchema.Properties["email"].Format, s.Properties["email"].Format)
		assert.Equal(t, inputSchema.Properties["age"].Minimum, s.Properties["age"].Minimum)
		assert.True(t, s.Properties["age"].Nullable)
		assert.Equal(t, inputSchema.Properties["comment"].MaxLength, s.Properties["comment"].MaxLength)
		assert.True(t, s.Properties["comment"].Nullable)
		assert.Equal(t, inputSchema.Properties["pattern"].Pattern, s.Properties["pattern"].Pattern)
	}
	updateSchema1 := r.Schema(reflect.TypeOf(ExampleUpdateStruct{}), false, "")
	validateSchema(updateSchema1)
	updateSchema2 := huma.SchemaFromType(r, reflect.TypeOf(ExampleUpdateStruct{}))
	validateSchema(updateSchema2)
}
