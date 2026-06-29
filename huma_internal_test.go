package huma

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEveryAlloc exercises findResult.everyAlloc directly. The public input
// path always operates on a freshly-zeroed input value, so slice/map elements
// along a param path are empty and never iterated at runtime. These branches
// exist for parity with every and to gracefully handle param paths that pass
// through slices/maps, so they're covered here with populated containers.
func TestEveryAlloc(t *testing.T) {
	type leaf struct {
		Val string
	}

	t.Run("allocates pointer and keeps it when a value is set", func(t *testing.T) {
		type container struct {
			Ptr *leaf
		}
		var c container
		r := &findResult[int]{}
		set := r.everyAlloc(reflect.ValueOf(&c).Elem(), []int{0, 0}, 1, func(v reflect.Value, _ int) bool {
			v.SetString("set")
			return true
		})
		assert.True(t, set)
		if assert.NotNil(t, c.Ptr) {
			assert.Equal(t, "set", c.Ptr.Val)
		}
	})

	t.Run("rolls an allocated pointer back to nil when nothing is set", func(t *testing.T) {
		type container struct {
			Ptr *leaf
		}
		var c container
		r := &findResult[int]{}
		set := r.everyAlloc(reflect.ValueOf(&c).Elem(), []int{0, 0}, 1, func(v reflect.Value, _ int) bool {
			return false
		})
		assert.False(t, set)
		assert.Nil(t, c.Ptr, "pointer the call allocated should be reset to nil")
	})

	t.Run("reuses an already-allocated pointer without rolling it back", func(t *testing.T) {
		type container struct {
			Ptr *leaf
		}
		c := container{Ptr: &leaf{Val: "existing"}}
		r := &findResult[int]{}
		set := r.everyAlloc(reflect.ValueOf(&c).Elem(), []int{0, 0}, 1, func(v reflect.Value, _ int) bool {
			return false
		})
		assert.False(t, set)
		// We did not allocate the pointer, so it must be left untouched.
		if assert.NotNil(t, c.Ptr) {
			assert.Equal(t, "existing", c.Ptr.Val)
		}
	})

	t.Run("recurses into slice elements", func(t *testing.T) {
		type container struct {
			Items []leaf
		}
		c := container{Items: []leaf{{}, {}}}
		r := &findResult[int]{}
		count := 0
		set := r.everyAlloc(reflect.ValueOf(&c).Elem(), []int{0, 0}, 1, func(v reflect.Value, _ int) bool {
			v.SetString("x")
			count++
			return true
		})
		assert.True(t, set)
		assert.Equal(t, 2, count)
		assert.Equal(t, "x", c.Items[1].Val)
	})

	t.Run("recurses into map elements", func(t *testing.T) {
		type container struct {
			M map[string]leaf
		}
		c := container{M: map[string]leaf{"a": {}, "b": {}}}
		r := &findResult[int]{}
		count := 0
		set := r.everyAlloc(reflect.ValueOf(&c).Elem(), []int{0, 0}, 1, func(v reflect.Value, _ int) bool {
			count++
			return true
		})
		assert.True(t, set)
		assert.Equal(t, 2, count)
	})

	t.Run("panics on an unsupported kind in the path", func(t *testing.T) {
		type container struct {
			N int
		}
		var c container
		r := &findResult[int]{}
		assert.PanicsWithValue(t, "unsupported", func() {
			// Path descends into the int field, which is neither a struct,
			// slice, map, nor pointer.
			r.everyAlloc(reflect.ValueOf(&c).Elem(), []int{0, 0}, 1, func(v reflect.Value, _ int) bool {
				return true
			})
		})
	})
}
