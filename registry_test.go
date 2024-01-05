package huma

import (
	"reflect"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/examples/protodemo/protodemo"
	"github.com/stretchr/testify/assert"
)

type Output[T any] struct{}

type Embedded[P any] struct{}

type EmbeddedTwo[P, V any] struct{}

type S struct{}

type ü struct{}

type MP4 struct{}

func TestDefaultSchemaNamer(t *testing.T) {
	type Renamed Output[*[]Embedded[protodemo.User]]

	for _, example := range []struct {
		typ  any
		name string
	}{
		{int(0), "Int"},
		{int64(0), "Int64"},
		{S{}, "S"},
		{time.Time{}, "Time"},
		{Output[int]{}, "OutputInt"},
		{Output[*int]{}, "OutputInt"},
		{Output[[]int]{}, "OutputListInt"},
		{Output[[]*int]{}, "OutputListInt"},
		{Output[[][]int]{}, "OutputListListInt"},
		{Output[map[string]int]{}, "OutputMapStringInt"},
		{Output[map[string][]*int]{}, "OutputMapStringListInt"},
		{Output[S]{}, "OutputS"},
		{Output[ü]{}, "OutputÜ"},
		{Output[MP4]{}, "OutputMP4"},
		{Output[Embedded[*protodemo.User]]{}, "OutputEmbeddedUser"},
		{Output[*[]Embedded[protodemo.User]]{}, "OutputListEmbeddedUser"},
		{Output[EmbeddedTwo[[]protodemo.User, **time.Time]]{}, "OutputEmbeddedTwoListUserTime"},
		{Renamed{}, "Renamed"},
	} {
		t.Run(example.name, func(t *testing.T) {
			name := DefaultSchemaNamer(reflect.TypeOf(example.typ), "hint")
			assert.Equal(t, example.name, name)
		})
	}
}
