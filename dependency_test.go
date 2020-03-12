package huma

import (
	"testing"

	"github.com/alecthomas/assert"
)

func TestDependencyNested(t *testing.T) {
	type Dep1 struct{}
	type Dep2 struct{}
	type Dep3 struct{}

	registry := NewDependencyRegistry()
	assert.NoError(t, registry.Add(&Dep1{}))

	assert.NoError(t, registry.Add(func(d1 *Dep1) (*Dep2, error) {
		return &Dep2{}, nil
	}))

	assert.NoError(t, registry.Add(func(d1 *Dep1, d2 *Dep2) (*Dep3, error) {
		return &Dep3{}, nil
	}))
}

func TestDependencyOrder(t *testing.T) {
	type Dep1 struct{}
	type Dep2 struct{}

	registry := NewDependencyRegistry()

	assert.Error(t, registry.Add(func(d2 *Dep2) (*Dep1, error) {
		return &Dep1{}, nil
	}))
}

func TestDependencyNotPointer(t *testing.T) {
	type Dep1 struct{}

	registry := NewDependencyRegistry()

	assert.Error(t, registry.Add(Dep1{}))
	assert.Error(t, registry.Add(func() (Dep1, error) {
		return Dep1{}, nil
	}))
}

func TestDependencyDupe(t *testing.T) {
	type Dep1 struct{}

	registry := NewDependencyRegistry()

	assert.NoError(t, registry.Add(&Dep1{}))
	assert.Error(t, registry.Add(&Dep1{}))
	assert.Error(t, registry.Add(func() (*Dep1, error) {
		return nil, nil
	}))
}
