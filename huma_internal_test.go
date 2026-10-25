package huma

import (
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// A path that arrives at a collection must step into its elements. Anything
// else means the path was recorded wrong, e.g. a kind was added to
// `_findInType` without marking its elements.
func TestCollectionPath(t *testing.T) {
	assert.Equal(t, []int{2}, collectionPath([]int{collectionElem, 2}))
	assert.Panics(t, func() { collectionPath([]int{2}) })
	assert.Panics(t, func() { collectionPath(nil) })
}

type textDuration struct {
	time.Duration
}

func (d *textDuration) UnmarshalText(data []byte) error {
	v, err := time.ParseDuration(string(data))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func TestConvertTypeTextUnmarshaler(t *testing.T) {
	assert.Equal(t, textDuration{10 * time.Second}, convertType("D", reflect.TypeFor[textDuration](), "10s"))
	assert.Equal(t, &textDuration{10 * time.Second}, convertType("D", reflect.TypeFor[*textDuration](), "10s"))
	assert.Panics(t, func() { convertType("D", reflect.TypeFor[textDuration](), "bogus") })

	// net.IP is a []byte, so a plain conversion would store the ASCII bytes.
	assert.Equal(t, net.ParseIP("127.0.0.1"), convertType("IP", reflect.TypeFor[net.IP](), "127.0.0.1"))
}

func TestTextUnmarshalerDefault(t *testing.T) {
	type Input struct {
		D textDuration `json:"d" default:"10s"`
	}
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	assert.Equal(t, "10s", SchemaFromType(r, reflect.TypeFor[Input]()).Properties["d"].Default)

	var in Input
	setDefaults(reflect.ValueOf(&in).Elem(), findDefaults(r, reflect.TypeFor[Input]()))
	assert.Equal(t, 10*time.Second, in.D.Duration)

	type Invalid struct {
		D textDuration `json:"d" default:"bogus"`
	}
	assert.Panics(t, func() { SchemaFromType(r, reflect.TypeFor[Invalid]()) })
}
