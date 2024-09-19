package huma

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
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
	RegisterTypeAlias(t reflect.Type, alias reflect.Type)
}

// DefaultSchemaNamer provides schema names for types. It uses the type name
// when possible, ignoring the package name. If the type is generic, e.g.
// `MyType[SubType]`, then the brackets are removed like `MyTypeSubType`.
// If the type is unnamed, then the name hint is used.
// Note: if you plan to use types with the same name from different packages,
// you should implement your own namer function to prevent issues. Nested
// anonymous types can also present naming issues.
func DefaultSchemaNamer(t reflect.Type, hint string) string {
	name := deref(t).Name()

	if name == "" {
		name = hint
	}

	// Better support for lists, so e.g. `[]int` becomes `ListInt`.
	name = strings.ReplaceAll(name, "[]", "List[")

	result := ""
	for _, part := range strings.FieldsFunc(name, func(r rune) bool {
		// Split on special characters. Note that `,` is used when there are
		// multiple inputs to a generic type.
		return r == '[' || r == ']' || r == '*' || r == ','
	}) {
		// Split fully qualified names like `github.com/foo/bar.Baz` into `Baz`.
		fqn := strings.Split(part, ".")
		base := fqn[len(fqn)-1]

		// Add to result, and uppercase for better scalar support (`int` -> `Int`).
		// Use unicode-aware uppercase to support non-ASCII characters.
		r, size := utf8.DecodeRuneInString(base)
		result += strings.ToUpper(string(r)) + base[size:]
	}
	name = result

	return name
}

type mapRegistry struct {
	prefix  string
	schemas map[string]*Schema
	types   map[string]reflect.Type
	seen    map[reflect.Type]bool
	namer   func(reflect.Type, string) string
	aliases map[reflect.Type]reflect.Type
}

func (r *mapRegistry) Schema(t reflect.Type, allowRef bool, hint string) *Schema {
	origType := t
	t = deref(t)

	// Pointer to array should decay to array
	if t.Kind() == reflect.Array || t.Kind() == reflect.Slice {
		origType = t
	}

	alias, ok := r.aliases[t]
	if ok {
		return r.Schema(alias, allowRef, hint)
	}

	getsRef := t.Kind() == reflect.Struct
	if t == timeType {
		// Special case: time.Time is always a string.
		getsRef = false
	}

	v := reflect.New(t).Interface()
	if _, ok := v.(SchemaProvider); ok {
		// Special case: type provides its own schema
		getsRef = false
	}
	if _, ok := v.(encoding.TextUnmarshaler); ok {
		// Special case: type can be unmarshalled from text so will be a `string`
		// and doesn't need a ref. This simplifies the schema a little bit.
		getsRef = false
	}

	name := r.namer(origType, hint)

	if getsRef {
		if s, ok := r.schemas[name]; ok {
			if _, ok := r.seen[t]; !ok {
				// Name matches but type is different, so we have a dupe.

				panic(fmt.Errorf("duplicate name: %s, new type: %s, existing type: %s", name, t, r.types[name]))
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
	s := SchemaFromType(r, origType)
	if getsRef {
		r.schemas[name] = s
	}

	if getsRef && allowRef {
		return &Schema{Ref: r.prefix + name}
	}
	return s
}

func (r *mapRegistry) SchemaFromRef(ref string) *Schema {
	if !strings.HasPrefix(ref, r.prefix) {
		return nil
	}
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

// RegisterTypeAlias(t, alias) makes the schema generator use the `alias` type instead of `t`.
func (r *mapRegistry) RegisterTypeAlias(t reflect.Type, alias reflect.Type) {
	r.aliases[t] = alias
}

// NewMapRegistry creates a new registry that stores schemas in a map and
// returns references to them using the given prefix.
func NewMapRegistry(prefix string, namer func(t reflect.Type, hint string) string) Registry {
	return &mapRegistry{
		prefix:  prefix,
		schemas: map[string]*Schema{},
		types:   map[string]reflect.Type{},
		seen:    map[reflect.Type]bool{},
		aliases: map[reflect.Type]reflect.Type{},
		namer:   namer,
	}
}
