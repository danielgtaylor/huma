package huma

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Registry creates and stores schemas and their references, and supports
// marshalling to JSON/YAML for use as an OpenAPI #/components/schemas object.
// Behavior is implementation-dependent, but the design allows for recursive
// schemas to exist while being flexible enough to support other use cases
// like only inline objects (no refs) or always using refs for structs.
type Registry interface {
	Schema(t reflect.Type, allowRef bool, hint string) *Schema
	SchemaFromRef(ref string) *Schema
	TypeFromRef(ref string) reflect.Type
	Map() map[string]*Schema
	MarshalJSON() ([]byte, error)
	MarshalYAML() (interface{}, error)
}

// DefaultSchemaNamer provides schema names for types. It uses the type name
// when possible, ignoring the package name. If the type is unnamed, then
// the name hint is used.
// Note: if you plan to use types with the same name from different packages,
// you should implement your own namer function to prevent issues. Nested
// anonymous types can also present naming issues.
func DefaultSchemaNamer(t reflect.Type, hint string) string {
	name := deref(t).Name()
	if name == "" {
		name = hint
	}
	return name
}

type mapRegistry struct {
	prefix  string
	schemas map[string]*Schema
	types   map[string]reflect.Type
	seen    map[reflect.Type]bool
	namer   func(reflect.Type, string) string
}

func (r *mapRegistry) Schema(t reflect.Type, allowRef bool, hint string) *Schema {
	t = deref(t)
	getsRef := t.Kind() == reflect.Struct

	name := r.namer(t, hint)

	if getsRef {
		if s, ok := r.schemas[name]; ok {
			if _, ok := r.seen[t]; !ok {
				// Name matches but type is different, so we have a dupe.
				panic(fmt.Errorf("duplicate name %s does not match existing type. New type %s, have existing types %+v", name, t, r.seen))
			}
			if allowRef {
				return &Schema{Ref: r.prefix + name}
			}
			return s
		}
	}

	// First, register the type so refs can be created above for recursive types.
	if getsRef {
		r.schemas[name] = &Schema{}
		r.types[name] = t
		r.seen[t] = true
	}
	s := SchemaFromType(r, t)
	if getsRef {
		r.schemas[name] = s
	}

	if getsRef && allowRef {
		return &Schema{Ref: r.prefix + name}
	}
	return s
}

func (r *mapRegistry) SchemaFromRef(ref string) *Schema {
	return r.schemas[ref[len(r.prefix):]]
}

func (r *mapRegistry) TypeFromRef(ref string) reflect.Type {
	return r.types[ref[len(r.prefix):]]
}

func (r *mapRegistry) Map() map[string]*Schema {
	return r.schemas
}

func (r *mapRegistry) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.schemas)
}

func (r *mapRegistry) MarshalYAML() (interface{}, error) {
	return r.schemas, nil
}

// NewMapRegistry creates a new registry that stores schemas in a map and
// returns references to them using the given prefix.
func NewMapRegistry(prefix string, namer func(t reflect.Type, hint string) string) Registry {
	return &mapRegistry{
		prefix:  prefix,
		schemas: map[string]*Schema{},
		types:   map[string]reflect.Type{},
		seen:    map[reflect.Type]bool{},
		namer:   namer,
	}
}
