package cbor

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCBORFormatUnmarshal(t *testing.T) {
	type body struct {
		Source string `json:"source,omitempty"`
	}
	tests := []struct {
		name  string
		input []byte
		err   string
		want  body
	}{
		// {"source":"ok"}
		{name: "exact key", input: []byte{0xA1, 0x66, 's', 'o', 'u', 'r', 'c', 'e', 0x62, 'o', 'k'}, want: body{Source: "ok"}},
		// {"Source":"ok"}
		{name: "case variant ignored", input: []byte{0xA1, 0x66, 'S', 'o', 'u', 'r', 'c', 'e', 0x62, 'o', 'k'}},
		// {"source":"a","source":"b"}
		{name: "duplicate key", input: []byte{0xA2, 0x66, 's', 'o', 'u', 'r', 'c', 'e', 0x61, 'a', 0x66, 's', 'o', 'u', 'r', 'c', 'e', 0x61, 'b'}, err: "duplicate map key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var v body
			err := DefaultCBORFormat.Unmarshal(test.input, &v)
			if test.err != "" {
				assert.ErrorContains(t, err, test.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, v)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	data := map[any]any{"hello": "world"}

	buf := &bytes.Buffer{}
	require.NoError(t, DefaultCBORFormat.Marshal(buf, data))

	var v any
	require.NoError(t, DefaultCBORFormat.Unmarshal(buf.Bytes(), &v))

	require.Equal(t, data, v)
}
