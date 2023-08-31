package huma

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSchema(t *testing.T) {
	bitSize := fmt.Sprint(bits.UintSize)

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
			panics: `invalid tag for field 'Value': invalid character 'b' looking for beginning of value`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

			if c.panics != "" {
				assert.PanicsWithValue(t, c.panics, func() {
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
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

	s := r.Schema(reflect.TypeOf(GreetingInput{}), false, "")
	// fmt.Printf("%+v\n", s)
	assert.Equal(t, "object", s.Type)
	assert.Equal(t, 1, len(s.Properties))
	assert.Equal(t, "string", s.Properties["ID"].Type)

	r.Schema(reflect.TypeOf(RecursiveInput{}), false, "")

	s2 := r.Schema(reflect.TypeOf(TestInput{}), false, "")
	pb := NewPathBuffer(make([]byte, 0, 128), 0)
	res := ValidateResult{}
	Validate(r, s2, pb, ModeReadFromServer, map[string]any{
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

	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	s := r.Schema(reflect.TypeOf(SchemaGeneric[int]{}), true, "")

	b, _ := json.Marshal(s)
	assert.JSONEq(t, `{
		"$ref": "#/components/schemas/SchemaGenericint"
	}`, string(b))
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
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

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
	pb := NewPathBuffer(make([]byte, 0, 128), 0)
	res := ValidateResult{}
	Validate(r, s2, pb, ModeReadFromServer, input, &res)
	assert.Empty(b, res.Errors)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pb.Reset()
		res.Reset()
		Validate(r, s2, pb, ModeReadFromServer, input, &res)
		if len(res.Errors) > 0 {
			b.Fatal(res.Errors)
		}
	}
}

func BenchmarkSchemaErrors(b *testing.B) {
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

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
	pb := NewPathBuffer(make([]byte, 0, 128), 0)
	res := ValidateResult{}
	Validate(r, s2, pb, ModeReadFromServer, input, &res)
	assert.NotEmpty(b, res.Errors)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pb.Reset()
		res.Reset()
		Validate(r, s2, pb, ModeReadFromServer, input, &res)
		if len(res.Errors) == 0 {
			b.Fatal("expected error")
		}
	}
}
