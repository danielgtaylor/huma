package huma

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A path that arrives at a collection must step into its elements. Anything
// else means the path was recorded wrong, e.g. a kind was added to
// `_findInType` without marking its elements.
func TestCollectionPath(t *testing.T) {
	assert.Equal(t, []int{2}, collectionPath([]int{collectionElem, 2}))
	assert.Panics(t, func() { collectionPath([]int{2}) })
	assert.Panics(t, func() { collectionPath(nil) })
}

// optParam is a minimal ParamWrapper used to check that pointer dereferencing
// in parseParamLocation happens before the wrapper is detected.
type optParam struct {
	Value int
	IsSet bool
}

func (o *optParam) Receiver() reflect.Value {
	return reflect.ValueOf(o).Elem().Field(0)
}

// Pointer params are dereferenced at registration so that the rest of the param
// machinery sees the type values are parsed into. The order matters: the deref
// must happen before the ParamWrapper and http.Cookie special cases.
func TestParseParamLocationPointer(t *testing.T) {
	registry := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

	for _, tc := range []struct {
		name      string
		field     reflect.StructField
		isPointer bool
		typ       reflect.Type
	}{
		{"string", reflect.StructField{Name: "P", Type: reflect.TypeFor[string](), Tag: `query:"p"`}, false, stringType},
		{"ptr-string", reflect.StructField{Name: "P", Type: reflect.TypeFor[*string](), Tag: `query:"p"`}, true, stringType},
		{"ptr-time", reflect.StructField{Name: "P", Type: reflect.TypeFor[*time.Time](), Tag: `query:"p"`}, true, timeType},
		{"ptr-slice", reflect.StructField{Name: "P", Type: reflect.TypeFor[*[]string](), Tag: `query:"p"`}, true, reflect.TypeFor[[]string]()},
		// http.Cookie is parsed from the cookie's string value.
		{"cookie", reflect.StructField{Name: "P", Type: reflect.TypeFor[http.Cookie](), Tag: `cookie:"p"`}, false, stringType},
		{"ptr-cookie", reflect.StructField{Name: "P", Type: reflect.TypeFor[*http.Cookie](), Tag: `cookie:"p"`}, true, stringType},
		// The wrapper exposes an int receiver, so that is what gets parsed.
		{"ptr-wrapper", reflect.StructField{Name: "P", Type: reflect.TypeFor[*optParam](), Tag: `query:"p"`}, true, reflect.TypeFor[int]()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pl, ok := parseParamLocation(tc.field, registry)
			require.True(t, ok)
			assert.Equal(t, tc.isPointer, pl.pfi.IsPointer)
			assert.Equal(t, tc.typ, pl.pfi.Type)
		})
	}
}

// Untagged pointer fields are not parameters and must not be rejected.
func TestParseParamLocationIgnoresUntagged(t *testing.T) {
	registry := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	_, ok := parseParamLocation(reflect.StructField{Name: "P", Type: reflect.TypeFor[**string]()}, registry)
	assert.False(t, ok)
}
