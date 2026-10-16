package huma

import (
	"encoding"
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

type issue680Duration struct {
	time.Duration
}

func (d *issue680Duration) UnmarshalText(data []byte) error {
	v, err := time.ParseDuration(string(data))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

var _ encoding.TextUnmarshaler = (*issue680Duration)(nil)

func TestConvertTypeTextUnmarshalerDefault(t *testing.T) {
	got := convertType("Duration", reflect.TypeOf(issue680Duration{}), "10s")
	d, ok := got.(issue680Duration)
	if !ok {
		t.Fatalf("got %T %#v", got, got)
	}
	if d.Duration != 10*time.Second {
		t.Fatalf("duration=%v", d.Duration)
	}
}

func TestFindDefaultsTextUnmarshaler(t *testing.T) {
	type Input struct {
		Duration issue680Duration `json:"duration" default:"10s"`
	}
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	defaults := findDefaults(r, reflect.TypeOf(Input{}))
	var in Input
	v := reflect.ValueOf(&in).Elem()
	setDefaults(v, defaults)
	if in.Duration.Duration != 10*time.Second {
		t.Fatalf("default not applied: %v", in.Duration)
	}
}

func TestSchemaFromFieldTextUnmarshalerDefault(t *testing.T) {
	type Input struct {
		Duration issue680Duration `json:"duration" default:"10s"`
	}
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	s := SchemaFromType(r, reflect.TypeOf(Input{}))
	def := s.Properties["duration"].Default
	if def != "10s" {
		t.Fatalf("schema default=%#v want %q", def, "10s")
	}
}

func TestConvertTypeTextUnmarshalerPointerDefault(t *testing.T) {
	got := convertType("Duration", reflect.TypeOf((*issue680Duration)(nil)), "10s")
	d, ok := got.(*issue680Duration)
	if !ok || d == nil {
		t.Fatalf("got %T %#v", got, got)
	}
	if d.Duration != 10*time.Second {
		t.Fatalf("duration=%v", d.Duration)
	}
}

func TestConvertTypeTextUnmarshalerDefaultInvalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on invalid duration default")
		}
	}()
	_ = convertType("Duration", reflect.TypeOf(issue680Duration{}), "not-a-duration")
}
