package huma_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/validation"
)

func Ptr[T any](v T) *T {
	return &v
}

func mapTo[A, B any](s []A, f func(A) B) []B {
	r := make([]B, len(s))
	for i, v := range s {
		r[i] = f(v)
	}
	return r
}

var validateTests = []struct {
	name    string
	typ     reflect.Type
	s       *huma.Schema
	input   any
	mode    huma.ValidateMode
	errs    []string
	panic   string
	before  func()
	cleanup func()
}{
	{
		name:  "bool success",
		typ:   reflect.TypeOf(true),
		input: true,
	},
	{
		name:  "expected bool",
		typ:   reflect.TypeOf(true),
		input: 1.23,
		errs:  []string{"expected boolean"},
	},
	{
		name:  "int success",
		typ:   reflect.TypeOf(0),
		input: 0,
	},
	{
		name:  "int from float64 success",
		typ:   reflect.TypeOf(0),
		input: float64(0),
	},
	{
		name:  "int from int8 success",
		typ:   reflect.TypeOf(0),
		input: int8(0),
	},
	{
		name:  "int from int16 success",
		typ:   reflect.TypeOf(0),
		input: int16(0),
	},
	{
		name:  "int from int32 success",
		typ:   reflect.TypeOf(0),
		input: int32(0),
	},
	{
		name:  "int from int64 success",
		typ:   reflect.TypeOf(0),
		input: int64(0),
	},
	{
		name:  "int from uint success",
		typ:   reflect.TypeOf(0),
		input: uint(0),
	},
	{
		name:  "int from uint8 success",
		typ:   reflect.TypeOf(0),
		input: uint8(0),
	},
	{
		name:  "int from uint16 success",
		typ:   reflect.TypeOf(0),
		input: uint16(0),
	},
	{
		name:  "int from uint32 success",
		typ:   reflect.TypeOf(0),
		input: uint32(0),
	},
	{
		name:  "int from uint64 success",
		typ:   reflect.TypeOf(0),
		input: uint64(0),
	},
	{
		name:  "float64 from int success",
		typ:   reflect.TypeOf(0.0),
		input: 0,
	},
	{
		name:  "float64 from float32 success",
		typ:   reflect.TypeOf(0.0),
		input: float32(0),
	},
	{
		name:  "int64 success",
		typ:   reflect.TypeOf(0),
		input: int64(0),
	},
	{
		name:  "expected number int",
		typ:   reflect.TypeOf(0),
		input: "",
		errs:  []string{"expected integer"},
	},
	{
		name:  "expected number int from float64",
		typ:   reflect.TypeOf(0),
		input: float64(1.5),
		errs:  []string{"expected integer"},
	},
	{
		name:  "expected number float64",
		typ:   reflect.TypeOf(float64(0)),
		input: "",
		errs:  []string{"expected number"},
	},
	{
		name: "minimum success",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" minimum:"1"`
		}{}),
		input: map[string]any{"value": 1},
	},
	{
		name: "minimum fail",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" minimum:"1"`
		}{}),
		input: map[string]any{"value": 0},
		errs:  []string{"expected number >= 1"},
	},
	{
		name: "exclusive minimum success",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" exclusiveMinimum:"1"`
		}{}),
		input: map[string]any{"value": 2},
	},
	{
		name: "exclusive minimum fail",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" exclusiveMinimum:"1"`
		}{}),
		input: map[string]any{"value": 1},
		errs:  []string{"expected number > 1"},
	},
	{
		name: "maximum success",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" maximum:"1"`
		}{}),
		input: map[string]any{"value": 1},
	},
	{
		name: "maximum fail",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" maximum:"1"`
		}{}),
		input: map[string]any{"value": 2},
		errs:  []string{"expected number <= 1"},
	},
	{
		name: "exclusive maximum success",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" exclusiveMaximum:"1"`
		}{}),
		input: map[string]any{"value": 0},
	},
	{
		name: "exclusive maximum fail",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" exclusiveMaximum:"1"`
		}{}),
		input: map[string]any{"value": 1},
		errs:  []string{"expected number < 1"},
	},
	{
		name: "multiple of success",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" multipleOf:"5"`
		}{}),
		input: map[string]any{"value": 10},
	},
	{
		name: "multiple of fail",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" multipleOf:"5"`
		}{}),
		input: map[string]any{"value": 2},
		errs:  []string{"expected number to be a multiple of 5"},
	},
	{
		name:  "string success",
		typ:   reflect.TypeOf(""),
		input: "",
	},
	{
		name:  "expected string",
		typ:   reflect.TypeOf(""),
		input: 1,
		errs:  []string{"expected string"},
	},
	{
		name: "min length success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" minLength:"1"`
		}{}),
		input: map[string]any{"value": "a"},
	},
	{
		name: "min length fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" minLength:"1"`
		}{}),
		input: map[string]any{"value": ""},
		errs:  []string{"expected length >= 1"},
	},
	{
		name: "max length success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" maxLength:"1"`
		}{}),
		input: map[string]any{"value": "a"},
	},
	{
		name: "non ascii max length success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" maxLength:"2"`
		}{}),
		input: map[string]any{"value": "аб"},
	},
	{
		name: "max length fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" maxLength:"1"`
		}{}),
		input: map[string]any{"value": "ab"},
		errs:  []string{"expected length <= 1"},
	},
	{
		name: "pattern success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" pattern:"^[a-z]+$"`
		}{}),
		input: map[string]any{"value": "a"},
	},
	{
		name: "pattern fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" pattern:"^[a-z]+$"`
		}{}),
		input: map[string]any{"value": "a1"},
		errs:  []string{"expected string to match pattern ^[a-z]+$"},
	},
	{
		name: "pattern custom message fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" pattern:"^[a-z]+$" patternDescription:"alphabetical"`
		}{}),
		input: map[string]any{"value": "b@2"},
		errs:  []string{"expected string to be alphabetical"},
	},
	{
		name: "pattern invalid",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" pattern:"^[a-"`
		}{}),
		input: map[string]any{"value": "a1"},
		panic: "error parsing regexp",
	},
	{
		name: "datetime success",
		typ: reflect.TypeOf(struct {
			Value time.Time `json:"value"`
		}{}),
		input: map[string]any{"value": []byte("2020-03-07T22:22:06-08:00")},
	},
	{
		name: "datetime string success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"date-time"`
		}{}),
		input: map[string]any{"value": []byte("2020-03-07T22:22:06-08:00")},
	},
	{
		name: "expected datetime",
		typ: reflect.TypeOf(struct {
			Value time.Time `json:"value"`
		}{}),
		input: map[string]any{"value": "bad"},
		errs:  []string{"expected string to be RFC 3339 date-time"},
	},
	{
		name: "date-time-http success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"date-time-http"`
		}{}),
		input: map[string]any{"value": []byte("Mon, 01 Jan 2023 12:00:00 GMT")},
	},
	{
		name: "expected date-time-http",
		typ: reflect.TypeOf(struct {
			Value time.Time `json:"value" format:"date-time-http"`
		}{}),
		input: map[string]any{"value": "bad"},
		errs:  []string{"expected string to be RFC 1123 date-time"},
	},
	{
		name: "date success",
		typ: reflect.TypeOf(struct {
			Value time.Time `json:"value" format:"date"`
		}{}),
		input: map[string]any{"value": "2020-03-07"},
	},
	{
		name: "expected date",
		typ: reflect.TypeOf(struct {
			Value time.Time `json:"value" format:"date"`
		}{}),
		input: map[string]any{"value": "bad"},
		errs:  []string{"expected string to be RFC 3339 date"},
	},
	{
		name: "time success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"time"`
		}{}),
		input: map[string]any{"value": "22:22:06-08:00"},
	},
	{
		name: "expected time",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"time"`
		}{}),
		input: map[string]any{"value": "bad"},
		errs:  []string{"expected string to be RFC 3339 time"},
	},
	{
		name: "email success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"email"`
		}{}),
		input: map[string]any{"value": "alice@example.com"},
	},
	{
		name: "expected email",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"email"`
		}{}),
		input: map[string]any{"value": "alice"},
		errs:  []string{"expected string to be RFC 5322 email: mail: missing '@' or angle-addr"},
	},
	{
		name: "hostname success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"hostname"`
		}{}),
		input: map[string]any{"value": "example.com"},
	},
	{
		name: "expected hostname",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"hostname"`
		}{}),
		input: map[string]any{"value": "%$^"},
		errs:  []string{"expected string to be RFC 5890 hostname"},
	},
	{
		name: "idn-hostname success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"idn-hostname"`
		}{}),
		input: map[string]any{"value": "ëxample.com"},
	},
	// {
	// 	name: "expected idn-hostname",
	// 	typ: reflect.TypeOf(struct {
	// 		Value string `json:"value" format:"idn-hostname"`
	// 	}{}),
	// 	input: map[string]any{"value": "\\"},
	// 	errs:  []string{"expected string to be RFC 5890 hostname"},
	// },
	{
		name: "ipv4 success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"ipv4"`
		}{}),
		input: map[string]any{"value": "127.0.0.1"},
	},
	{
		name: "expected ipv4",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"ipv4"`
		}{}),
		input: map[string]any{"value": "1234"},
		errs:  []string{"expected string to be RFC 2673 ipv4"},
	},
	{
		name: "ipv6 success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"ipv6"`
		}{}),
		input: map[string]any{"value": "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
	},
	{
		name: "expected ipv6",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"ipv6"`
		}{}),
		input: map[string]any{"value": "1234"},
		errs:  []string{"expected string to be RFC 2373 ipv6"},
	},
	{
		name: "uri success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uri"`
		}{}),
		input: map[string]any{"value": "http://example.com"},
	},
	{
		name: "expected uri",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uri"`
		}{}),
		input: map[string]any{"value": ":"},
		errs:  []string{"expected string to be RFC 3986 uri: parse \":\": missing protocol scheme"},
	},
	{
		name: "uuid success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "123e4567-e89b-12d3-a456-426655440000"},
	},
	{
		name: "uuid success prefix",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "urn:uuid:123e4567-e89b-12d3-a456-426655440000"},
	},
	{
		name: "uuid success braces",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "{123e4567-e89b-12d3-a456-426655440000}"},
	},
	{
		name: "expected uuid",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "bad"},
		errs:  []string{"expected string to be RFC 4122 uuid: invalid UUID length: 3"},
	},
	{
		name: "expected uuid prefix",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "urn:test:123e4567-e89b-12d3-a456-426655440000"},
		errs:  []string{"expected string to be RFC 4122 uuid: invalid urn prefix: \"urn:test:\""},
	},
	{
		name: "expected uuid bracket",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "[123e4567-e89b-12d3-a456-426655440000]"},
		errs:  []string{"expected string to be RFC 4122 uuid: invalid bracketed UUID format"},
	},
	{
		name: "expected uuid format",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "12345678gabc1234abcd1234abcd1234"},
		errs:  []string{"expected string to be RFC 4122 uuid: invalid UUID format"},
	},
	{
		name: "expected uuid format",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "12345678gabc1234abcd1234abcd12345678"},
		errs:  []string{"expected string to be RFC 4122 uuid: invalid UUID format"},
	},
	{
		name: "expected uuid format",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uuid"`
		}{}),
		input: map[string]any{"value": "\n23e4567-e89b-12d3-a456-426655440000"},
		errs:  []string{"expected string to be RFC 4122 uuid: invalid UUID format"},
	},
	{
		name: "uritemplate success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uri-template"`
		}{}),
		input: map[string]any{"value": "/items/{item-id}/history"},
	},
	{
		name: "expected uritemplate bad url",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uri-template"`
		}{}),
		input: map[string]any{"value": ":"},
		errs:  []string{"expected string to be RFC 3986 uri: parse \":\": missing protocol scheme"},
	},
	{
		name: "expected uritemplate",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"uri-template"`
		}{}),
		input: map[string]any{"value": "missing{"},
		errs:  []string{"expected string to be RFC 6570 uri-template"},
	},
	{
		name: "jsonpointer success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"json-pointer"`
		}{}),
		input: map[string]any{"value": "/foo/bar"},
	},
	{
		name: "expected jsonpointer",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"json-pointer"`
		}{}),
		input: map[string]any{"value": "bad"},
		errs:  []string{"expected string to be RFC 6901 json-pointer"},
	},
	{
		name: "rel jsonpointer success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"relative-json-pointer"`
		}{}),
		input: map[string]any{"value": "0"},
	},
	{
		name: "expected rel jsonpointer",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"relative-json-pointer"`
		}{}),
		input: map[string]any{"value": "/bad"},
		errs:  []string{"expected string to be RFC 6901 relative-json-pointer"},
	},
	{
		name: "regex success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"regex"`
		}{}),
		input: map[string]any{"value": "^[0-9a-f]+$"},
	},
	{
		name: "expected regex",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" format:"regex"`
		}{}),
		input: map[string]any{"value": "^[bad"},
		errs:  []string{"expected string to be regex: error parsing regexp: missing closing ]: `[bad`"},
	},
	{
		name: "base64 byte success",
		typ: reflect.TypeOf(struct {
			Value []byte `json:"value"`
		}{}),
		input: map[string]any{"value": []byte("ABCD")},
	},
	{
		name: "base64 string success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" encoding:"base64"`
		}{}),
		input: map[string]any{"value": "ABCD"},
	},
	{
		name: "base64 fail",
		typ: reflect.TypeOf(struct {
			Value []byte `json:"value"`
		}{}),
		input: map[string]any{"value": []byte("!")},
		errs:  []string{"expected string to be base64 encoded"},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]any{}),
		input: []any{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]int{}),
		input: []int{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]int8{}),
		input: []int8{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]int16{}),
		input: []int16{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]int32{}),
		input: []int32{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]int64{}),
		input: []int64{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]uint{}),
		input: []uint{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]uint16{}),
		input: []uint16{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]uint32{}),
		input: []uint32{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]uint64{}),
		input: []uint64{1, 2, 3},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]float32{}),
		input: []float32{1.1, 2.3, 3.52},
	},
	{
		name:  "array success",
		typ:   reflect.TypeOf([]float64{}),
		input: []float64{3.1, 2.3, 3.62},
	},
	{
		name:  "expected array",
		typ:   reflect.TypeOf([]any{}),
		input: 1,
		errs:  []string{"expected array"},
	},
	{
		name: "min items success",
		typ: reflect.TypeOf(struct {
			Value []any `json:"value" minItems:"1"`
		}{}),
		input: map[string]any{"value": []any{1}},
	},
	{
		name: "expected min items",
		typ: reflect.TypeOf(struct {
			Value []any `json:"value" minItems:"1"`
		}{}),
		input: map[string]any{"value": []any{}},
		errs:  []string{"expected array length >= 1"},
	},
	{
		name: "max items success",
		typ: reflect.TypeOf(struct {
			Value []any `json:"value" maxItems:"1"`
		}{}),
		input: map[string]any{"value": []any{1}},
	},
	{
		name: "expected max items",
		typ: reflect.TypeOf(struct {
			Value []any `json:"value" maxItems:"1"`
		}{}),
		input: map[string]any{"value": []any{1, 2}},
		errs:  []string{"expected array length <= 1"},
	},
	{
		name: "unique success",
		typ: reflect.TypeOf(struct {
			Value []any `json:"value" uniqueItems:"true"`
		}{}),
		input: map[string]any{"value": []any{1, 2, 3, 4, 5}},
	},
	{
		name: "expected unique",
		typ: reflect.TypeOf(struct {
			Value []any `json:"value" uniqueItems:"true"`
		}{}),
		input: map[string]any{"value": []any{1, 2, 1, 3}},
		errs:  []string{"expected array items to be unique"},
	},
	{
		name:  "map success",
		typ:   reflect.TypeOf(map[string]int{}),
		input: map[string]any{"one": 1, "two": 2},
	},
	{
		name:  "map any success",
		typ:   reflect.TypeOf(map[string]int{}),
		input: map[any]any{"one": 1, "two": 2},
	},
	{
		name:  "map any int success",
		typ:   reflect.TypeOf(map[int]string{}),
		input: map[any]any{1: "one", 2: "two"},
	},
	{
		name:  "expected map item",
		typ:   reflect.TypeOf(map[any]int{}),
		input: map[string]any{"one": 1, "two": true},
		errs:  []string{"expected integer"},
	},
	{
		name:  "expected map any item",
		typ:   reflect.TypeOf(map[any]int{}),
		input: map[any]any{"one": 1, "two": true},
		errs:  []string{"expected integer"},
	},
	{
		name: "map minProps success",
		typ: reflect.TypeOf(struct {
			Value map[string]int `json:"value" minProperties:"1"`
		}{}),
		input: map[string]any{
			"value": map[string]any{"one": 1},
		},
	},
	{
		name: "map any minProps success",
		typ: reflect.TypeOf(struct {
			Value map[any]int `json:"value" minProperties:"1"`
		}{}),
		input: map[any]any{
			"value": map[any]any{"one": 1},
		},
	},
	{
		name: "expected map minProps",
		typ: reflect.TypeOf(struct {
			Value map[string]int `json:"value" minProperties:"1"`
		}{}),
		input: map[string]any{
			"value": map[string]any{},
		},
		errs: []string{"expected object with at least 1 properties"},
	},
	{
		name: "expected map any minProps",
		typ: reflect.TypeOf(struct {
			Value map[any]int `json:"value" minProperties:"1"`
		}{}),
		input: map[any]any{
			"value": map[any]any{},
		},
		errs: []string{"expected object with at least 1 properties"},
	},
	{
		name: "map maxProps success",
		typ: reflect.TypeOf(struct {
			Value map[string]int `json:"value" maxProperties:"1"`
		}{}),
		input: map[string]any{
			"value": map[string]any{"one": 1},
		},
	},
	{
		name: "map any maxProps success",
		typ: reflect.TypeOf(struct {
			Value map[any]int `json:"value" maxProperties:"1"`
		}{}),
		input: map[any]any{
			"value": map[any]any{"one": 1},
		},
	},
	{
		name: "expected map maxProps",
		typ: reflect.TypeOf(struct {
			Value map[string]int `json:"value" maxProperties:"1"`
		}{}),
		input: map[string]any{
			"value": map[string]any{"one": 1, "two": 2},
		},
		errs: []string{"expected object with at most 1 properties"},
	},
	{
		name: "expected map any maxProps",
		typ: reflect.TypeOf(struct {
			Value map[any]int `json:"value" maxProperties:"1"`
		}{}),
		input: map[any]any{
			"value": map[any]any{"one": 1, "two": 2},
		},
		errs: []string{"expected object with at most 1 properties"},
	},
	{
		name: "json.RawMessage success",
		typ: reflect.TypeOf(struct {
			Value json.RawMessage `json:"value"`
		}{}),
		input: map[string]any{"value": map[string]any{"some": []any{"thing"}}},
	},
	{
		name:  "object struct success",
		typ:   reflect.TypeOf(struct{}{}),
		input: map[string]any{},
	},
	{
		name:  "object struct any success",
		typ:   reflect.TypeOf(struct{}{}),
		input: map[any]any{},
	},
	{
		name:  "expected object",
		typ:   reflect.TypeOf(struct{}{}),
		input: true,
		errs:  []string{"expected object"},
	},
	{
		name: "object optional success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value,omitempty"`
		}{}),
		input: map[string]any{},
	},
	{
		name: "object any optional success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value,omitempty"`
		}{}),
		input: map[any]any{},
	},
	{
		name: "readOnly set success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" readOnly:"true"`
		}{}),
		mode:  huma.ModeWriteToServer,
		input: map[string]any{"value": "whoops"},
	},
	{
		name: "readOnly any set success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" readOnly:"true"`
		}{}),
		mode:  huma.ModeWriteToServer,
		input: map[any]any{"value": "whoops"},
	},
	{
		name: "readOnly missing success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" readOnly:"true"`
		}{}),
		mode:  huma.ModeWriteToServer,
		input: map[string]any{},
	},
	{
		name: "readOnly any missing success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" readOnly:"true"`
		}{}),
		mode:  huma.ModeWriteToServer,
		input: map[any]any{},
	},
	{
		name: "readOnly missing fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" readOnly:"true"`
		}{}),
		mode:  huma.ModeReadFromServer,
		input: map[string]any{},
		errs:  []string{"expected required property value to be present"},
	},
	{
		name: "readOnly any missing fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" readOnly:"true"`
		}{}),
		mode:  huma.ModeReadFromServer,
		input: map[any]any{},
		errs:  []string{"expected required property value to be present"},
	},
	{
		name: "writeOnly missing fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" writeOnly:"true"`
		}{}),
		mode:  huma.ModeReadFromServer,
		input: map[string]any{"value": "should not be set"},
		errs:  []string{"write only property is non-zero"},
	},
	{
		name: "writeOnly any missing fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" writeOnly:"true"`
		}{}),
		mode:  huma.ModeReadFromServer,
		input: map[any]any{"value": "should not be set"},
		errs:  []string{"write only property is non-zero"},
	},
	{
		name: "case-insensive success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value"`
		}{}),
		input: map[string]any{"VaLuE": "works"},
	},
	{
		name: "case-insensive fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" maxLength:"3"`
		}{}),
		input: map[string]any{"VaLuE": "fails"},
		errs:  []string{"expected length <= 3"},
	},
	{
		name:    "case-sensive fail",
		before:  func() { huma.ValidateStrictCasing = true },
		cleanup: func() { huma.ValidateStrictCasing = false },
		typ: reflect.TypeOf(struct {
			Value string `json:"value"`
		}{}),
		input: map[string]any{"VaLuE": "fails due to casing"},
		errs: []string{
			"expected required property value to be present",
			"unexpected property",
		},
	},
	{
		name: "unexpected property",
		typ: reflect.TypeOf(struct {
			Value string `json:"value,omitempty"`
		}{}),
		input: map[string]any{"value2": "whoops"},
		errs:  []string{"unexpected property"},
	},
	{
		name: "unexpected property any",
		typ: reflect.TypeOf(struct {
			Value string `json:"value,omitempty"`
		}{}),
		input: map[any]any{123: "whoops"},
		errs:  []string{"unexpected property"},
	},
	{
		name: "nested success",
		typ: reflect.TypeOf(struct {
			Items []struct {
				Value string `json:"value"`
			} `json:"items"`
		}{}),
		input: map[string]any{"items": []any{map[string]any{"value": "hello"}}},
	},
	{
		name: "nested any success",
		typ: reflect.TypeOf(struct {
			Items []struct {
				Value string `json:"value"`
			} `json:"items"`
		}{}),
		input: map[any]any{"items": []any{map[any]any{"value": "hello"}}},
	},
	{
		name: "expected nested",
		typ: reflect.TypeOf(struct {
			Items []struct {
				Value string `json:"value"`
			} `json:"items"`
		}{}),
		input: map[string]any{"items": []any{map[string]any{}}},
		errs:  []string{"expected required property value to be present"},
	},
	{
		name: "expected nested any",
		typ: reflect.TypeOf(struct {
			Items []struct {
				Value string `json:"value"`
			} `json:"items"`
		}{}),
		input: map[any]any{"items": []any{map[any]any{}}},
		errs:  []string{"expected required property value to be present"},
	},
	{
		name: "manual object property required",
		s: &huma.Schema{
			Type:     huma.TypeObject,
			Required: []string{"value"},
			Properties: map[string]*huma.Schema{
				"value": {Type: huma.TypeString},
			},
		},
		input: map[string]any{},
		errs:  []string{"expected required property value to be present"},
	},
	{
		name: "enum string success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" enum:"one,two"`
		}{}),
		input: map[string]any{"value": "one"},
	},
	{
		name: "enum int success",
		typ: reflect.TypeOf(struct {
			Value int `json:"value" enum:"1,5,9"`
		}{}),
		input: map[string]any{"value": 1.0},
	},
	{
		name: "enum uint16 success",
		typ: reflect.TypeOf(struct {
			Value uint16 `json:"value" enum:"1,5,9"`
		}{}),
		input: map[string]any{"value": 1.0},
	},
	{
		name: "enum array success",
		typ: reflect.TypeOf(struct {
			Value []int `json:"value" enum:"1,5,9"`
		}{}),
		input: map[string]any{"value": []any{1.0}},
	},
	{
		name: "expected enum",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" enum:"one,two"`
		}{}),
		input: map[string]any{"value": "three"},
		errs:  []string{"expected value to be one of \"one, two\""},
	},
	{
		name: "optional success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value,omitempty" minLength:"1"`
		}{}),
		input: map[string]any{},
	},
	{
		name: "optional null success",
		typ: reflect.TypeOf(struct {
			Value *string `json:"value,omitempty" minLength:"1"`
		}{}),
		input: map[string]any{"value": nil},
	},
	{
		name: "optional any null success",
		typ: reflect.TypeOf(struct {
			Value *string `json:"value,omitempty" minLength:"1"`
		}{}),
		input: map[any]any{"value": nil},
	},
	{
		name: "optional fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value,omitempty" minLength:"1"`
		}{}),
		input: map[string]any{"value": ""},
		errs:  []string{"expected length >= 1"},
	},
	{
		name: "hidden is optional",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" minLength:"5" hidden:"true"`
		}{}),
		input: map[any]any{},
	},
	{
		name: "hidden success",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" minLength:"5" hidden:"true"`
		}{}),
		input: map[any]any{"value": "abcde"},
	},
	{
		name: "hidden fail",
		typ: reflect.TypeOf(struct {
			Value string `json:"value" minLength:"5" hidden:"true"`
		}{}),
		input: map[any]any{"value": "abc"},
		errs:  []string{"expected length >= 5"},
	},
	{
		name: "dependentRequired empty success",
		typ: reflect.TypeOf(struct {
			Value     string `json:"value,omitempty" dependentRequired:"dependent"`
			Dependent string `json:"dependent,omitempty"`
		}{}),
		input: map[string]any{},
		errs:  nil,
	},
	{
		name: "dependentRequired filled success",
		typ: reflect.TypeOf(struct {
			Value     string `json:"value,omitempty" dependentRequired:"dependent"`
			Dependent string `json:"dependent,omitempty"`
		}{}),
		input: map[string]any{"value": "abc", "dependent": "123"},
		errs:  nil,
	},
	{
		name: "dependentRequired ignored success",
		typ: reflect.TypeOf(struct {
			Value     string `json:"value,omitempty" dependentRequired:""`
			Dependent string `json:"dependent,omitempty"`
		}{}),
		input: map[string]any{},
		errs:  nil,
	},
	{
		name: "dependentRequired failure",
		typ: reflect.TypeOf(struct {
			Value1     string `json:"value1,omitempty" dependentRequired:"dependent1,dependent3"`
			Dependent1 string `json:"dependent1,omitempty"`
			Value2     string `json:"value2,omitempty" dependentRequired:"dependent2,dependent3"`
			Dependent2 string `json:"dependent2,omitempty"`
			Dependent3 string `json:"dependent3,omitempty"`
		}{}),
		input: map[string]any{"value1": "abc", "value2": "123"},
		errs: []string{
			"expected property dependent1 to be present when value1 is present",
			"expected property dependent3 to be present when value1 is present",
			"expected property dependent2 to be present when value2 is present",
			"expected property dependent3 to be present when value2 is present",
		},
	},
	{
		name: "dependentRequired empty success any",
		typ: reflect.TypeOf(struct {
			Value     string `json:"value,omitempty" dependentRequired:"dependent"`
			Dependent string `json:"dependent,omitempty"`
		}{}),
		input: map[any]any{},
		errs:  nil,
	},
	{
		name: "dependentRequired filled success any",
		typ: reflect.TypeOf(struct {
			Value     string `json:"value,omitempty" dependentRequired:"dependent"`
			Dependent string `json:"dependent,omitempty"`
		}{}),
		input: map[any]any{"value": "abc", "dependent": "123"},
		errs:  nil,
	},
	{
		name: "dependentRequired ignored success any",
		typ: reflect.TypeOf(struct {
			Value     string `json:"value,omitempty" dependentRequired:""`
			Dependent string `json:"dependent,omitempty"`
		}{}),
		input: map[any]any{},
		errs:  nil,
	},
	{
		name: "dependentRequired failure any",
		typ: reflect.TypeOf(struct {
			Value1     string `json:"value1,omitempty" dependentRequired:"dependent1,dependent3"`
			Dependent1 string `json:"dependent1,omitempty"`
			Value2     string `json:"value2,omitempty" dependentRequired:"dependent2,dependent3"`
			Dependent2 string `json:"dependent2,omitempty"`
			Dependent3 string `json:"dependent3,omitempty"`
		}{}),
		input: map[any]any{"value1": "abc", "value2": "123"},
		errs: []string{
			"expected property dependent1 to be present when value1 is present",
			"expected property dependent3 to be present when value1 is present",
			"expected property dependent2 to be present when value2 is present",
			"expected property dependent3 to be present when value2 is present",
		},
	},
	{
		name: "oneOf success bool",
		s: &huma.Schema{
			OneOf: []*huma.Schema{
				{Type: huma.TypeBoolean},
				{Type: huma.TypeString},
			},
		},
		input: true,
	},
	{
		name: "oneOf success string",
		s: &huma.Schema{
			OneOf: []*huma.Schema{
				{Type: huma.TypeBoolean},
				{Type: huma.TypeString},
			},
		},
		input: "hello",
	},
	{
		name: "oneOf fail zero",
		s: &huma.Schema{
			OneOf: []*huma.Schema{
				{Type: huma.TypeBoolean},
				{Type: huma.TypeString},
			},
		},
		input: 123,
		errs:  []string{"expected value to match exactly one schema but matched none"},
	},
	{
		name: "oneOf fail multi",
		s: &huma.Schema{
			OneOf: []*huma.Schema{
				{Type: huma.TypeNumber, Minimum: Ptr(float64(5))},
				{Type: huma.TypeNumber, Maximum: Ptr(float64(10))},
			},
		},
		input: 8,
		errs:  []string{"expected value to match exactly one schema but matched multiple"},
	},
	{
		name: "anyOf success",
		s: &huma.Schema{
			AnyOf: []*huma.Schema{
				{Type: huma.TypeNumber, Minimum: Ptr(float64(5))},
				{Type: huma.TypeNumber, Maximum: Ptr(float64(10))},
			},
		},
		input: 8,
	},
	{
		name: "anyOf fail",
		s: &huma.Schema{
			AnyOf: []*huma.Schema{
				{Type: huma.TypeNumber, Minimum: Ptr(float64(5))},
				{Type: huma.TypeNumber, Minimum: Ptr(float64(10))},
			},
		},
		input: 1,
		errs:  []string{"expected value to match at least one schema but matched none"},
	},
	{
		name: "allOf success",
		s: &huma.Schema{
			AllOf: []*huma.Schema{
				{Type: huma.TypeNumber, Minimum: Ptr(float64(5))},
				{Type: huma.TypeNumber, Maximum: Ptr(float64(10))},
			},
		},
		input: 8,
	},
	{
		name: "allOf fail",
		s: &huma.Schema{
			AllOf: []*huma.Schema{
				{Type: huma.TypeNumber, Minimum: Ptr(float64(5))},
				{Type: huma.TypeNumber, Maximum: Ptr(float64(10))},
			},
		},
		input: 12,
		errs:  []string{"expected number <= 10"},
	},
	{
		name: "not success",
		s: &huma.Schema{
			Not: &huma.Schema{Type: huma.TypeNumber},
		},
		input: "hello",
	},
	{
		name: "not fail",
		s: &huma.Schema{
			Not: &huma.Schema{Type: huma.TypeNumber},
		},
		input: 5,
		errs:  []string{"expected value to not match schema"},
	},
	{
		name:  "nullable success",
		s:     &huma.Schema{Type: huma.TypeNumber, Nullable: true},
		input: nil,
	},
	{
		name: "pointer required field success",
		typ: reflect.TypeOf(struct {
			Field *int `json:"field" required:"true" nullable:"true"`
		}{}),
		input: map[string]any{"field": nil},
	},
	{
		name: "pointer required field fail",
		typ: reflect.TypeOf(struct {
			Field *int `json:"field" required:"true" nullable:"true"`
		}{}),
		input: map[string]any{},
		errs:  []string{"expected required property field to be present"},
	},
	{
		name: "discriminator: input expected to be an object",
		s: &huma.Schema{
			Type: huma.TypeObject,
			OneOf: []*huma.Schema{
				{Type: huma.TypeString},
			},
			Discriminator: &huma.Discriminator{
				PropertyName: "inputType",
			},
		},
		input: "test",
		errs:  []string{validation.MsgExpectedObject},
	},
	{
		name: "discriminator: propertyName expected to be present in object",
		s: &huma.Schema{
			Type: huma.TypeObject,
			OneOf: []*huma.Schema{
				{Type: huma.TypeString},
			},
			Properties: map[string]*huma.Schema{
				"inputType": {Type: huma.TypeString},
			},
			Discriminator: &huma.Discriminator{
				PropertyName: "inputType",
			},
		},
		input: map[string]any{"undefined": ""},
		errs:  []string{validation.MsgExpectedPropertyNameInObject},
	},
	{
		name: "discriminator: propertyName expected to be present in any object",
		s: &huma.Schema{
			Type: huma.TypeObject,
			OneOf: []*huma.Schema{
				{Type: huma.TypeString},
			},
			Properties: map[string]*huma.Schema{
				"inputType": {Type: huma.TypeString},
			},
			Discriminator: &huma.Discriminator{
				PropertyName: "inputType",
			},
		},
		input: map[any]any{"undefined": ""},
		errs:  []string{validation.MsgExpectedPropertyNameInObject},
	},
	{
		name: "discriminator: propertyName expected to be string",
		s: &huma.Schema{
			Type: huma.TypeObject,
			OneOf: []*huma.Schema{
				{Type: huma.TypeString},
			},
			Properties: map[string]*huma.Schema{
				"inputType": {Type: huma.TypeString},
			},
			Discriminator: &huma.Discriminator{
				PropertyName: "inputType",
			},
		},
		input: map[string]any{"inputType": 1},
		errs:  []string{validation.MsgExpectedString},
	},
	{
		name: "discriminator: propertyName not explicitly mapped",
		s: &huma.Schema{
			Type: huma.TypeObject,
			OneOf: []*huma.Schema{
				{Type: huma.TypeString},
			},
			Properties: map[string]*huma.Schema{
				"inputType": {Type: huma.TypeString},
			},
			Discriminator: &huma.Discriminator{
				PropertyName: "inputType",
			},
		},
		input: map[string]any{"inputType": "test"},
		errs:  []string{validation.MsgExpectedMatchExactlyOneSchema},
	},
}

func TestValidate(t *testing.T) {
	pb := huma.NewPathBuffer([]byte(""), 0)
	res := &huma.ValidateResult{}

	for _, test := range validateTests {
		t.Run(test.name, func(t *testing.T) {
			if test.before != nil {
				test.before()
			}
			if test.cleanup != nil {
				defer test.cleanup()
			}

			registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)

			var s *huma.Schema
			if test.panic != "" {
				assert.Panics(t, func() {
					registry.Schema(test.typ, true, "TestInput")
				})
				return
			} else {
				if test.s != nil {
					s = test.s
					s.PrecomputeMessages()
				} else {
					s = registry.Schema(test.typ, true, "TestInput")
				}
			}

			pb.Reset()
			res.Reset()

			huma.Validate(registry, s, pb, test.mode, test.input, res)

			assert.Len(t, res.Errors, len(test.errs))
			if len(test.errs) > 0 {
				errs := mapTo(res.Errors, func(e error) string {
					return e.(*huma.ErrorDetail).Message
				})
				schemaJSON, _ := json.MarshalIndent(registry.Map(), "", "  ")
				for _, err := range test.errs {
					assert.Contains(t, errs, err, string(schemaJSON))
				}
			} else {
				assert.Empty(t, res.Errors)
			}
		})
	}
}

func TestValidateCustomFormatter(t *testing.T) {
	originalFormatter := huma.ErrorFormatter
	defer func() {
		huma.ErrorFormatter = originalFormatter
	}()

	huma.ErrorFormatter = func(format string, a ...any) string {
		return fmt.Sprintf("custom: %v", a)
	}

	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := registry.Schema(reflect.TypeOf(struct {
		Value string `json:"value" format:"email"`
	}{}), true, "TestInput")
	pb := huma.NewPathBuffer([]byte(""), 0)
	res := &huma.ValidateResult{}

	huma.Validate(registry, s, pb, huma.ModeReadFromServer, map[string]any{"value": "alice"}, res)
	assert.Len(t, res.Errors, 1)
	assert.Equal(t, "custom: [mail: missing '@' or angle-addr] (value: alice)", res.Errors[0].Error())
}

type TransformDeleteField struct {
	Field1 string `json:"field1"`
	Field2 string `json:"field2"`
}

var _ huma.SchemaTransformer = (*TransformDeleteField)(nil)

func (t *TransformDeleteField) TransformSchema(r huma.Registry, s *huma.Schema) *huma.Schema {
	delete(s.Properties, "field2")
	return s
}

func TestValidateSchemaTransformerDeleteField(t *testing.T) {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := registry.Schema(reflect.TypeOf(&TransformDeleteField{}), true, "TestInput")
	pb := huma.NewPathBuffer([]byte(""), 0)
	res := &huma.ValidateResult{}

	huma.Validate(registry, s, pb, huma.ModeReadFromServer, map[string]any{"field1": "value1"}, res)
	// We should have no errors and no panics.
	assert.Empty(t, res.Errors)
	assert.NotContains(t, s.Properties, "field2")
}

func ExampleModelValidator() {
	// Define a type you want to validate.
	type Model struct {
		Name string `json:"name" maxLength:"5"`
		Age  int    `json:"age" minimum:"25"`
	}

	typ := reflect.TypeOf(Model{})

	// Unmarshal some JSON into an `any` for validation. This input should not
	// validate against the schema for the struct above.
	var val any
	json.Unmarshal([]byte(`{"name": "abcdefg", "age": 1}`), &val)

	// Validate the unmarshaled data against the type and print errors.
	validator := huma.NewModelValidator()
	errs := validator.Validate(typ, val)
	fmt.Println(errs)

	// Try again with valid data!
	json.Unmarshal([]byte(`{"name": "foo", "age": 25}`), &val)
	errs = validator.Validate(typ, val)
	fmt.Println(errs)

	// Output: [expected number >= 25 (age: 1) expected length <= 5 (name: abcdefg)]
	// []
}

var BenchValidatePB *huma.PathBuffer
var BenchValidateRes *huma.ValidateResult

func BenchmarkValidate(b *testing.B) {
	pb := huma.NewPathBuffer([]byte(""), 0)
	res := &huma.ValidateResult{}
	BenchValidatePB = pb
	BenchValidateRes = res

	for _, test := range validateTests {
		if test.panic != "" || len(test.errs) > 0 {
			continue
		}

		b.Run(strings.TrimSuffix(test.name, " success"), func(b *testing.B) {
			registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
			s := registry.Schema(test.typ, false, "TestInput")

			input := test.input
			if s.Type == huma.TypeObject && s.Properties["value"] != nil {
				switch i := input.(type) {
				case map[string]any:
					for k := range i {
						if strings.EqualFold(k, "value") {
							input = i[k]
						}
					}
					s = s.Properties["value"]
				case map[any]any:
					for k := range i {
						if strings.EqualFold(fmt.Sprintf("%v", k), "value") {
							input = i[k]
						}
					}
					s = s.Properties["value"]
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pb.Reset()
				res.Reset()
				huma.Validate(registry, s, pb, test.mode, input, res)
			}
		})
	}
}

type Cat struct {
	Name string `json:"name" minLength:"2" maxLength:"10"`
	Kind string `json:"kind" enum:"cat"`
}

type Dog struct {
	Color string `json:"color" enum:"black,white,brown"`
	Kind  string `json:"kind" enum:"dog"`
}

func Test_validateWithDiscriminator(t *testing.T) {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	catSchema := registry.Schema(reflect.TypeOf(Cat{}), true, "Cat")
	dogSchema := registry.Schema(reflect.TypeOf(Dog{}), true, "Dog")

	s := &huma.Schema{
		Type:        huma.TypeObject,
		Description: "Animal",
		OneOf: []*huma.Schema{
			{Ref: catSchema.Ref},
			{Ref: dogSchema.Ref},
		},
		Discriminator: &huma.Discriminator{
			PropertyName: "kind",
			Mapping: map[string]string{
				"cat": catSchema.Ref,
				"dog": dogSchema.Ref,
			},
		},
	}

	pb := huma.NewPathBuffer([]byte(""), 0)
	res := &huma.ValidateResult{}

	tests := []struct {
		name     string
		input    any
		wantErrs []string
	}{
		{
			name: "cat - minLength case",
			input: map[string]any{
				"kind": "cat",
				"name": "c",
			},
			wantErrs: []string{"expected length >= 2"},
		},
		{
			name: "cat - maxLength case",
			input: map[string]any{
				"kind": "cat",
				"name": "aaaaaaaaaaa",
			},
			wantErrs: []string{"expected length <= 10"},
		},
		{
			name: "cat - invalid schema",
			input: map[string]any{
				"kind": "dog",
				"name": "cat",
			},
			wantErrs: []string{
				"expected required property color to be present",
				"unexpected property",
			},
		},
		{
			name: "cat - any invalid schema",
			input: map[any]any{
				"kind": "dog",
				"name": "cat",
			},
			wantErrs: []string{
				"expected required property color to be present",
				"unexpected property",
			},
		},
		{
			name: "cat - ok",
			input: map[string]any{
				"kind": "cat",
				"name": "meow",
			},
		},
		{
			name: "cat - any ok",
			input: map[any]any{
				"kind": "cat",
				"name": "meow",
			},
		},
		{
			name: "dog - wrong color",
			input: map[string]any{
				"kind":  "dog",
				"color": "red",
			},
			wantErrs: []string{"expected value to be one of \"black, white, brown\""},
		},
		{
			name: "unknown kind",
			input: map[string]any{
				"kind": "unknown",
				"foo":  "bar",
			},
			wantErrs: []string{validation.MsgExpectedMatchExactlyOneSchema},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pb.Reset()
			res.Reset()
			huma.Validate(registry, s, pb, huma.ModeWriteToServer, tc.input, res)
			require.Len(t, res.Errors, len(tc.wantErrs))
			for i, wantErr := range tc.wantErrs {
				assert.Contains(t, res.Errors[i].Error(), wantErr)
			}
		})
	}
}

func TestPathBuffer_WithIndex_ReturnsExpectedString(t *testing.T) {
	pb := huma.NewPathBuffer([]byte(""), 0)
	pb.Push("prefix")

	assert.Equal(t, "prefix[1]", pb.WithIndex(1))
}
