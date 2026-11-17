package huma_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
)

func TestDefaultJSONFormatUnmarshal(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		err    string
		target func() any
	}{
		{name: "scalar", input: `1`},
		{name: "nested", input: `{"a":{"b":[{"c":1},{"c":2}]},"b":[1,{"a":1}]}`},
		{name: "same key in sibling objects", input: `[{"a":1},{"a":2}]`},
		{name: "case variants are distinct", input: `{"a":1,"A":2}`},
		{name: "colons and escaped quotes in strings", input: `{"a:b":"c:\\\"d:","e":["f:"]}`},
		{name: "duplicate top-level key", input: `{"a":1,"a":2}`, err: "duplicate object key"},
		{name: "duplicate nested key", input: `{"inner":{"a":9},"inner":{}}`, err: "duplicate object key"},
		{name: "duplicate in array item", input: `[{"a":1,"a":1}]`, err: "duplicate object key"},
		{name: "empty object", input: `{}`},
		{name: "escaped backslash before closing quote", input: `{"\\":1,"a":2}`},
		{name: "json text inside a string", input: `{"a":"{\"a\":1,\"a\":2}"}`},
		{name: "keys colliding after utf-8 replacement", input: "{\"\xff\":1,\"\xfe\":2}", err: "duplicate object key"},
		{name: "typed target not checked", input: `{"a":1,"a":2}`, target: func() any { return &map[string]int{} }},
		{name: "invalid json", input: `{"a":`, err: "unexpected end of JSON input"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var v any
			target := any(&v)
			if test.target != nil {
				target = test.target()
			}
			err := huma.DefaultJSONFormat.Unmarshal([]byte(test.input), target)
			if test.err == "" {
				require.NoError(t, err)
				return
			}
			assert.EqualError(t, err, test.err)
		})
	}
}
