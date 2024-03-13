package huma_test

import (
	"bytes"
	"encoding/json"
	"math/bits"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
)

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
			name:     "url",
			input:    &url.URL{},
			expected: `{"type": "string", "format": "uri"}`,
		},
		{
			name:     "ip",
			input:    net.IPv4(127, 0, 0, 1),
			expected: `{"type": "string", "format": "ipv4"}`,
		},
		{
			name:     "bytes",
			input:    []byte("test"),
			expected: `{"type": "string", "contentEncoding": "base64"}`,
		},
		{
			name:     "array",
			input:    [2]int{1, 2},
			expected: `{"type": "array", "items": {"type": "integer", "format": "int64"}, "minItems": 2, "maxItems": 2}`,
		},
		{
			name:     "slice",
			input:    []int{1, 2, 3},
			expected: `{"type": "array", "items": {"type": "integer", "format": "int64"}}`,
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
						"type": "array",
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
						"type": "array",
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
						"type": "array",
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
						"type": "array",
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
			name: "field-enum-custom",
			input: struct {
				Value OmittableNullable[string] `json:"value,omitempty" enum:"foo,bar"`
			}{},
			expected: `{
				"type": "object",
				"properties": {
					"value": {
						"type": "string",
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
				// Filtered out from JSON tag
				Value1 string `json:"-"`
				// Filtered because it's private
				value2 string
				// Filtered due to being an unsupported type
				Value3 func()
			}{},
			expected: `{
				"type": "object",
				"additionalProperties": false
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
				"required": ["int", "str"],
				"properties": {
					"int": {
						"type": "integer",
						"format": "int64",
						"examples": [123]
					},
					"str": {
						"type": "string",
						"examples": ["foo"]
					}
				}
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
	// fmt.Printf("%+v\n", s)
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

	// b, _ := json.MarshalIndent(r.Map(), "", "  ")
	// fmt.Println(string(b))
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
	return r.Schema(reflect.TypeOf(o.Value), true, "")
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
	err := json.Unmarshal([]byte(`{"field": 123}`), &o) //nolint:musttag
	require.NoError(t, err)
	assert.True(t, o.Field.Sent)
	assert.False(t, o.Field.Null)
	assert.Equal(t, 123, o.Field.Value)

	o = O{}
	err = json.Unmarshal([]byte(`{"field": null}`), &o) //nolint:musttag
	require.NoError(t, err)
	assert.True(t, o.Field.Sent)
	assert.True(t, o.Field.Null)
	assert.Equal(t, 0, o.Field.Value)

	o = O{}
	err = json.Unmarshal([]byte(`{}`), &o) //nolint:musttag
	require.NoError(t, err)
	assert.False(t, o.Field.Sent)
	assert.False(t, o.Field.Null)
	assert.Equal(t, 0, o.Field.Value)
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
