package huma

import (
	"testing"

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
