package cbor

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	data := map[any]any{"hello": "world"}

	buf := &bytes.Buffer{}
	require.NoError(t, DefaultCBORFormat.Marshal(buf, data))

	var v any
	require.NoError(t, DefaultCBORFormat.Unmarshal(buf.Bytes(), &v))

	require.Equal(t, data, v)
}
