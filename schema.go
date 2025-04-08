package huma

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2/validation"
)

// ErrSchemaInvalid is sent when there is a problem building the schema.
var ErrSchemaInvalid = errors.New("schema is invalid")

// DefaultArrayNullable controls whether arrays are nullable by default. Set
// this to `false` to make arrays non-nullable by default, but be aware that
// any `nil` slice will still encode as `null` in JSON. See also:
// https://pkg.go.dev/encoding/json#Marshal.
var DefaultArrayNullable = true

// JSON Schema type constants
const (
	TypeBoolean = "boolean"
	TypeInteger = "integer"
	TypeNumber  = "number"
	TypeString  = "string"
	TypeArray   = "array"
	TypeObject  = "object"
)

// Special JSON Schema formats.
var (
	timeType       = reflect.TypeOf(time.Time{})
	ipType         = reflect.TypeOf(net.IP{})
	ipAddrType     = reflect.TypeOf(netip.Addr{})
	urlType        = reflect.TypeOf(url.URL{})
	rawMessageType = reflect.TypeOf(json.RawMessage{})
)

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// Discriminator object when request bodies or response payloads may be one of a
// number of different schemas, can be used to aid in serialization,
// deserialization, and validation. The discriminator is a specific object in a
// schema which is used to inform the consumer of the document of an alternative
// schema based on the value associated with it.
type Discriminator struct {
	// PropertyName in the payload that will hold the discriminator value.
	// REQUIRED.
	PropertyName string `yaml:"propertyName"`

	// Mapping object to hold mappings between payload values and schema names or
	// references.
	Mapping map[string]string `yaml:"mapping,omitempty"`
}

func (d *Discriminator) MarshalJSON() ([]byte, error) {
	return marshalJSON([]jsonFieldInfo{
		{"propertyName", d.PropertyName, omitNever},
		{"mapping", d.Mapping, omitEmpty},
	}, nil)
}

// Schema represents a JSON Schema compatible with OpenAPI 3.1. It is extensible
// with your own custom properties. It supports a subset of the full JSON Schema
// spec, designed specifically for use with Go structs and to enable fast zero
// or near-zero allocation happy-path validation for incoming requests.
//
// Typically you will use a registry and `huma.SchemaFromType` to generate
// schemas for your types. You can then use `huma.Validate` to validate
// incoming requests.
//
//	// Create a registry and register a type.
//	registry := huma.NewMapRegistry("#/prefix", huma.DefaultSchemaNamer)
//	schema := huma.SchemaFromType(registry, reflect.TypeOf(MyType{}))
//
// Note that the registry may create references for your types.
type Schema struct {
	Type                 string              `yaml:"type,omitempty"`
	Nullable             bool                `yaml:"-"`
	Title                string              `yaml:"title,omitempty"`
	Description          string              `yaml:"description,omitempty"`
	Ref                  string              `yaml:"$ref,omitempty"`
	Format               string              `yaml:"format,omitempty"`
	ContentEncoding      string              `yaml:"contentEncoding,omitempty"`
	Default              any                 `yaml:"default,omitempty"`
	Examples             []any               `yaml:"examples,omitempty"`
	Items                *Schema             `yaml:"items,omitempty"`
	AdditionalProperties any                 `yaml:"additionalProperties,omitempty"`
	Properties           map[string]*Schema  `yaml:"properties,omitempty"`
	Enum                 []any               `yaml:"enum,omitempty"`
	Minimum              *float64            `yaml:"minimum,omitempty"`
	ExclusiveMinimum     *float64            `yaml:"exclusiveMinimum,omitempty"`
	Maximum              *float64            `yaml:"maximum,omitempty"`
	ExclusiveMaximum     *float64            `yaml:"exclusiveMaximum,omitempty"`
	MultipleOf           *float64            `yaml:"multipleOf,omitempty"`
	MinLength            *int                `yaml:"minLength,omitempty"`
	MaxLength            *int                `yaml:"maxLength,omitempty"`
	Pattern              string              `yaml:"pattern,omitempty"`
	PatternDescription   string              `yaml:"patternDescription,omitempty"`
	MinItems             *int                `yaml:"minItems,omitempty"`
	MaxItems             *int                `yaml:"maxItems,omitempty"`
	UniqueItems          bool                `yaml:"uniqueItems,omitempty"`
	Required             []string            `yaml:"required,omitempty"`
	MinProperties        *int                `yaml:"minProperties,omitempty"`
	MaxProperties        *int                `yaml:"maxProperties,omitempty"`
	ReadOnly             bool                `yaml:"readOnly,omitempty"`
	WriteOnly            bool                `yaml:"writeOnly,omitempty"`
	Deprecated           bool                `yaml:"deprecated,omitempty"`
	Extensions           map[string]any      `yaml:",inline"`
	DependentRequired    map[string][]string `yaml:"dependentRequired,omitempty"`

	OneOf []*Schema `yaml:"oneOf,omitempty"`
	AnyOf []*Schema `yaml:"anyOf,omitempty"`
	AllOf []*Schema `yaml:"allOf,omitempty"`
	Not   *Schema   `yaml:"not,omitempty"`

	// OpenAPI specific fields
	Discriminator *Discriminator `yaml:"discriminator,omitempty"`

	patternRe     *regexp.Regexp  `yaml:"-"`
	requiredMap   map[string]bool `yaml:"-"`
	propertyNames []string        `yaml:"-"`
	hidden        bool            `yaml:"-"`

	// Precomputed validation messages. These prevent allocations during
	// validation and are known at schema creation time.
	msgEnum              string                       `yaml:"-"`
	msgMinimum           string                       `yaml:"-"`
	msgExclusiveMinimum  string                       `yaml:"-"`
	msgMaximum           string                       `yaml:"-"`
	msgExclusiveMaximum  string                       `yaml:"-"`
	msgMultipleOf        string                       `yaml:"-"`
	msgMinLength         string                       `yaml:"-"`
	msgMaxLength         string                       `yaml:"-"`
	msgPattern           string                       `yaml:"-"`
	msgMinItems          string                       `yaml:"-"`
	msgMaxItems          string                       `yaml:"-"`
	msgMinProperties     string                       `yaml:"-"`
	msgMaxProperties     string                       `yaml:"-"`
	msgRequired          map[string]string            `yaml:"-"`
	msgDependentRequired map[string]map[string]string `yaml:"-"`
}

// MarshalJSON marshals the schema into JSON, respecting the `Extensions` map
// to marshal extensions inline.
func (s *Schema) MarshalJSON() ([]byte, error) {
	var typ any = s.Type
	if s.Nullable {
		typ = []string{s.Type, "null"}
	}

	var contentMediaType string
	if s.Format == "binary" {
		contentMediaType = "application/octet-stream"
	}

	props := s.Properties
	for _, ps := range props {
		if ps.hidden {
			// Copy the map to avoid modifying the original schema.
			props = make(map[string]*Schema, len(s.Properties))
			for k, v := range s.Properties {
				if !v.hidden {
					props[k] = v
				}
			}
			break
		}
	}

	return marshalJSON([]jsonFieldInfo{
		{"type", typ, omitEmpty},
		{"title", s.Title, omitEmpty},
		{"description", s.Description, omitEmpty},
		{"$ref", s.Ref, omitEmpty},
		{"format", s.Format, omitEmpty},
		{"contentMediaType", contentMediaType, omitEmpty},
		{"contentEncoding", s.ContentEncoding, omitEmpty},
		{"default", s.Default, omitNil},
		{"examples", s.Examples, omitEmpty},
		{"items", s.Items, omitEmpty},
		{"additionalProperties", s.AdditionalProperties, omitNil},
		{"properties", props, omitEmpty},
		{"enum", s.Enum, omitEmpty},
		{"minimum", s.Minimum, omitEmpty},
		{"exclusiveMinimum", s.ExclusiveMinimum, omitEmpty},
		{"maximum", s.Maximum, omitEmpty},
		{"exclusiveMaximum", s.ExclusiveMaximum, omitEmpty},
		{"multipleOf", s.MultipleOf, omitEmpty},
		{"minLength", s.MinLength, omitEmpty},
		{"maxLength", s.MaxLength, omitEmpty},
		{"pattern", s.Pattern, omitEmpty},
		{"patternDescription", s.PatternDescription, omitEmpty},
		{"minItems", s.MinItems, omitEmpty},
		{"maxItems", s.MaxItems, omitEmpty},
		{"uniqueItems", s.UniqueItems, omitEmpty},
		{"required", s.Required, omitEmpty},
		{"dependentRequired", s.DependentRequired, omitEmpty},
		{"minProperties", s.MinProperties, omitEmpty},
		{"maxProperties", s.MaxProperties, omitEmpty},
		{"readOnly", s.ReadOnly, omitEmpty},
		{"writeOnly", s.WriteOnly, omitEmpty},
		{"deprecated", s.Deprecated, omitEmpty},
		{"oneOf", s.OneOf, omitEmpty},
		{"anyOf", s.AnyOf, omitEmpty},
		{"allOf", s.AllOf, omitEmpty},
		{"not", s.Not, omitEmpty},
		{"discriminator", s.Discriminator, omitEmpty},
	}, s.Extensions)
}

// PrecomputeMessages tries to precompute as many validation error messages
// as possible so that new strings aren't allocated during request validation.
func (s *Schema) PrecomputeMessages() {
	s.msgEnum = ErrorFormatter(validation.MsgExpectedOneOf, strings.Join(mapTo(s.Enum, func(v any) string {
		return fmt.Sprintf("%v", v)
	}), ", "))
	if s.Minimum != nil {
		s.msgMinimum = ErrorFormatter(validation.MsgExpectedMinimumNumber, *s.Minimum)
	}
	if s.ExclusiveMinimum != nil {
		s.msgExclusiveMinimum = ErrorFormatter(validation.MsgExpectedExclusiveMinimumNumber, *s.ExclusiveMinimum)
	}
	if s.Maximum != nil {
		s.msgMaximum = ErrorFormatter(validation.MsgExpectedMaximumNumber, *s.Maximum)
	}
	if s.ExclusiveMaximum != nil {
		s.msgExclusiveMaximum = ErrorFormatter(validation.MsgExpectedExclusiveMaximumNumber, *s.ExclusiveMaximum)
	}
	if s.MultipleOf != nil {
		s.msgMultipleOf = ErrorFormatter(validation.MsgExpectedNumberBeMultipleOf, *s.MultipleOf)
	}
	if s.MinLength != nil {
		s.msgMinLength = ErrorFormatter(validation.MsgExpectedMinLength, *s.MinLength)
	}
	if s.MaxLength != nil {
		s.msgMaxLength = ErrorFormatter(validation.MsgExpectedMaxLength, *s.MaxLength)
	}
	if s.Pattern != "" {
		s.patternRe = regexp.MustCompile(s.Pattern)
		if s.PatternDescription != "" {
			s.msgPattern = ErrorFormatter(validation.MsgExpectedBePattern, s.PatternDescription)
		} else {
			s.msgPattern = ErrorFormatter(validation.MsgExpectedMatchPattern, s.Pattern)
		}
	}
	if s.MinItems != nil {
		s.msgMinItems = ErrorFormatter(validation.MsgExpectedMinItems, *s.MinItems)
	}
	if s.MaxItems != nil {
		s.msgMaxItems = ErrorFormatter(validation.MsgExpectedMaxItems, *s.MaxItems)
	}
	if s.MinProperties != nil {
		s.msgMinProperties = ErrorFormatter(validation.MsgExpectedMinProperties, *s.MinProperties)
	}
	if s.MaxProperties != nil {
		s.msgMaxProperties = ErrorFormatter(validation.MsgExpectedMaxProperties, *s.MaxProperties)
	}

	if s.Required != nil {
		if s.msgRequired == nil {
			s.msgRequired = map[string]string{}
		}
		for _, name := range s.Required {
			s.msgRequired[name] = ErrorFormatter(validation.MsgExpectedRequiredProperty, name)
		}
	}

	if s.DependentRequired != nil {
		if s.msgDependentRequired == nil {
			s.msgDependentRequired = map[string]map[string]string{}
		}
		for name, dependents := range s.DependentRequired {
			for _, dependent := range dependents {
				if s.msgDependentRequired[name] == nil {
					s.msgDependentRequired[name] = map[string]string{}
				}
				s.msgDependentRequired[name][dependent] = ErrorFormatter(validation.MsgExpectedDependentRequiredProperty, dependent, name)
			}
		}
	}

	s.propertyNames = make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		s.propertyNames = append(s.propertyNames, name)
	}
	sort.Strings(s.propertyNames)

	s.requiredMap = map[string]bool{}
	for _, name := range s.Required {
		s.requiredMap[name] = true
	}

	if s.Items != nil {
		s.Items.PrecomputeMessages()
	}

	for _, prop := range s.Properties {
		prop.PrecomputeMessages()
	}

	for _, sub := range s.OneOf {
		sub.PrecomputeMessages()
	}

	for _, sub := range s.AnyOf {
		sub.PrecomputeMessages()
	}

	for _, sub := range s.AllOf {
		sub.PrecomputeMessages()
	}

	if sub := s.Not; sub != nil {
		sub.PrecomputeMessages()
	}
}

func boolTag(f reflect.StructField, tag string, def bool) bool {
	if v := f.Tag.Get(tag); v != "" {
		switch v {
		case "true":
			return true
		case "false":
			return false
		default:
			panic(fmt.Errorf("invalid bool tag '%s' for field '%s': %v", tag, f.Name, v))
		}
	}
	return def
}

func intTag(f reflect.StructField, tag string, def *int) *int {
	if v := f.Tag.Get(tag); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return &i
		} else {
			panic(fmt.Errorf("invalid int tag '%s' for field '%s': %v (%w)", tag, f.Name, v, err))
		}
	}
	return def
}

func floatTag(f reflect.StructField, tag string, def *float64) *float64 {
	if v := f.Tag.Get(tag); v != "" {
		if i, err := strconv.ParseFloat(v, 64); err == nil {
			return &i
		} else {
			panic(fmt.Errorf("invalid float tag '%s' for field '%s': %v (%w)", tag, f.Name, v, err))
		}
	}
	return def
}

func stringTag(f reflect.StructField, tag string, def string) string {
	if v := f.Tag.Get(tag); v != "" {
		return v
	}
	return def
}

// ensureType panics if the given value does not match the JSON Schema type.
func ensureType(r Registry, fieldName string, s *Schema, value string, v any) {
	if s.Ref != "" {
		s = r.SchemaFromRef(s.Ref)
		if s == nil {
			// We may not have access to this type, e.g. custom schema provided
			// by the user with remote refs. Skip validation.
			return
		}
	}

	switch s.Type {
	case TypeBoolean:
		if _, ok := v.(bool); !ok {
			panic(fmt.Errorf("invalid boolean tag value '%s' for field '%s': %w", value, fieldName, ErrSchemaInvalid))
		}
	case TypeInteger, TypeNumber:
		if _, ok := v.(float64); !ok {
			panic(fmt.Errorf("invalid number tag value '%s' for field '%s': %w", value, fieldName, ErrSchemaInvalid))
		}

		if s.Type == TypeInteger {
			if v.(float64) != float64(int(v.(float64))) {
				panic(fmt.Errorf("invalid integer tag value '%s' for field '%s': %w", value, fieldName, ErrSchemaInvalid))
			}
		}
	case TypeString:
		if _, ok := v.(string); !ok {
			panic(fmt.Errorf("invalid string tag value '%s' for field '%s': %w", value, fieldName, ErrSchemaInvalid))
		}
	case TypeArray:
		if _, ok := v.([]any); !ok {
			panic(fmt.Errorf("invalid array tag value '%s' for field '%s': %w", value, fieldName, ErrSchemaInvalid))
		}

		if s.Items != nil {
			for i, item := range v.([]any) {
				b, _ := json.Marshal(item)
				ensureType(r, fieldName+"["+strconv.Itoa(i)+"]", s.Items, string(b), item)
			}
		}
	case TypeObject:
		if _, ok := v.(map[string]any); !ok {
			panic(fmt.Errorf("invalid object tag value '%s' for field '%s': %w", value, fieldName, ErrSchemaInvalid))
		}

		for name, prop := range s.Properties {
			if val, ok := v.(map[string]any)[name]; ok {
				b, _ := json.Marshal(val)
				ensureType(r, fieldName+"."+name, prop, string(b), val)
			}
		}
	}
}

// convertType panics if the given value does not match or cannot be converted
// to the field's Go type.
func convertType(fieldName string, t reflect.Type, v any) any {
	vv := reflect.ValueOf(v)
	tv := reflect.TypeOf(v)
	if v != nil && tv != t {
		if tv.Kind() == reflect.Slice {
			// Slices can't be cast due to the different layouts. Instead, we make a
			// new instance of the destination slice, and convert each value in
			// the original to the new type.
			tmp := reflect.MakeSlice(t, 0, vv.Len())
			for i := 0; i < vv.Len(); i++ {
				item := vv.Index(i)
				if item.Kind() == reflect.Interface {
					// E.g. []any and we want the underlying type.
					item = item.Elem()
				}
				item = reflect.Indirect(item)
				typ := deref(t.Elem())
				if !item.Type().ConvertibleTo(typ) {
					panic(fmt.Errorf("unable to convert %v to %v for field '%s': %w", item.Interface(), t.Elem(), fieldName, ErrSchemaInvalid))
				}

				value := item.Convert(typ)
				if t.Elem().Kind() == reflect.Ptr {
					// Special case: if the field is a pointer, we need to get a pointer
					// to the converted value.
					ptr := reflect.New(value.Type())
					ptr.Elem().Set(value)
					value = ptr
				}

				tmp = reflect.Append(tmp, value)
			}
			v = tmp.Interface()
		} else if !tv.ConvertibleTo(deref(t)) {
			panic(fmt.Errorf("unable to convert %v to %v for field '%s': %w", tv, t, fieldName, ErrSchemaInvalid))
		}

		converted := reflect.ValueOf(v).Convert(deref(t))
		if t.Kind() == reflect.Ptr {
			// Special case: if the field is a pointer, we need to get a pointer
			// to the converted value.
			tmp := reflect.New(t.Elem())
			tmp.Elem().Set(converted)
			converted = tmp
		}
		v = converted.Interface()
	}
	return v
}

func jsonTagValue(r Registry, fieldName string, s *Schema, value string) any {
	if s.Ref != "" {
		s = r.SchemaFromRef(s.Ref)
		if s == nil {
			return nil
		}
	}

	// Special case: strings don't need quotes.
	if s.Type == TypeString {
		return value
	}

	// Special case: array of strings with comma-separated values and no quotes.
	if s.Type == TypeArray && s.Items != nil && s.Items.Type == TypeString && value[0] != '[' {
		values := []string{}
		for _, s := range strings.Split(value, ",") {
			values = append(values, strings.TrimSpace(s))
		}
		return values
	}

	var v any
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		panic(fmt.Errorf("invalid %s tag value '%s' for field '%s': %w", s.Type, value, fieldName, err))
	}

	ensureType(r, fieldName, s, value, v)

	return v
}

// jsonTag returns a value of the schema's type for the given tag string.
// Uses JSON parsing if the schema is not a string.
func jsonTag(r Registry, f reflect.StructField, s *Schema, name string) any {
	t := f.Type
	if value := f.Tag.Get(name); value != "" {
		return convertType(f.Name, t, jsonTagValue(r, f.Name, s, value))
	}
	return nil
}

// SchemaFromField generates a schema for a given struct field. If the field
// is a struct (or slice/map of structs) then the registry is used to
// potentially get a reference to that type.
//
// This is used by `huma.SchemaFromType` when it encounters a struct, and
// is used to generate schemas for path/query/header parameters.
func SchemaFromField(registry Registry, f reflect.StructField, hint string) *Schema {
	fs := registry.Schema(f.Type, true, hint)
	if fs == nil {
		return fs
	}
	fs.Description = stringTag(f, "doc", fs.Description)
	if fs.Format == "date-time" && f.Tag.Get("header") != "" {
		// Special case: this is a header and uses a different date/time format.
		// Note that it can still be overridden by the `format` or `timeFormat`
		// tags later.
		fs.Format = "date-time-http"
	}
	fs.Format = stringTag(f, "format", fs.Format)
	if timeFmt := f.Tag.Get("timeFormat"); timeFmt != "" {
		switch timeFmt {
		case "2006-01-02":
			fs.Format = "date"
		case "15:04:05":
			fs.Format = "time"
		default:
			fs.Format = timeFmt
		}
	}
	fs.ContentEncoding = stringTag(f, "encoding", fs.ContentEncoding)
	if defaultValue := jsonTag(registry, f, fs, "default"); defaultValue != nil {
		fs.Default = defaultValue
	}

	if value, ok := f.Tag.Lookup("example"); ok {
		if e := jsonTagValue(registry, f.Name, fs, value); e != nil {
			fs.Examples = []any{e}
		}
	}

	if enum := f.Tag.Get("enum"); enum != "" {
		s := fs
		if s.Type == TypeArray {
			s = s.Items
		}
		enumValues := []any{}
		for _, e := range strings.Split(enum, ",") {
			enumValues = append(enumValues, jsonTagValue(registry, f.Name, s, e))
		}
		if fs.Type == TypeArray {
			fs.Items.Enum = enumValues
		} else {
			fs.Enum = enumValues
		}
	}

	fs.Nullable = boolTag(f, "nullable", fs.Nullable)
	if fs.Nullable && fs.Ref != "" && registry.SchemaFromRef(fs.Ref).Type == "object" {
		// Nullability is only supported for scalar types for now. Objects are
		// much more complicated because the `null` type lives within the object
		// definition (requiring multiple copies of the object) or needs to use
		// `anyOf` or `not` which is not supported by all code generators, or is
		// supported poorly & generates hard-to-use code. This is less than ideal
		// but a compromise for now to support some nullability built-in.
		panic(fmt.Errorf("nullable is not supported for field '%s' which is type '%s'", f.Name, fs.Ref))
	}

	fs.Minimum = floatTag(f, "minimum", fs.Minimum)
	fs.ExclusiveMinimum = floatTag(f, "exclusiveMinimum", fs.ExclusiveMinimum)
	fs.Maximum = floatTag(f, "maximum", fs.Maximum)
	fs.ExclusiveMaximum = floatTag(f, "exclusiveMaximum", fs.ExclusiveMaximum)
	fs.MultipleOf = floatTag(f, "multipleOf", fs.MultipleOf)
	fs.MinLength = intTag(f, "minLength", fs.MinLength)
	fs.MaxLength = intTag(f, "maxLength", fs.MaxLength)
	fs.Pattern = stringTag(f, "pattern", fs.Pattern)
	fs.PatternDescription = stringTag(f, "patternDescription", fs.PatternDescription)
	fs.MinItems = intTag(f, "minItems", fs.MinItems)
	fs.MaxItems = intTag(f, "maxItems", fs.MaxItems)
	fs.UniqueItems = boolTag(f, "uniqueItems", fs.UniqueItems)
	fs.MinProperties = intTag(f, "minProperties", fs.MinProperties)
	fs.MaxProperties = intTag(f, "maxProperties", fs.MaxProperties)
	fs.ReadOnly = boolTag(f, "readOnly", fs.ReadOnly)
	fs.WriteOnly = boolTag(f, "writeOnly", fs.WriteOnly)
	fs.Deprecated = boolTag(f, "deprecated", fs.Deprecated)
	fs.PrecomputeMessages()

	fs.hidden = boolTag(f, "hidden", fs.hidden)

	return fs
}

// fieldInfo stores information about a field, which may come from an
// embedded type. The `Parent` stores the field's direct parent.
type fieldInfo struct {
	Parent reflect.Type
	Field  reflect.StructField
}

// getFields performs a breadth-first search for all fields including embedded
// ones. It may return multiple fields with the same name, the first of which
// represents the outermost declaration.
func getFields(typ reflect.Type, visited map[reflect.Type]struct{}) []fieldInfo {
	fields := make([]fieldInfo, 0, typ.NumField())
	var embedded []reflect.StructField

	if _, ok := visited[typ]; ok {
		return fields
	}
	visited[typ] = struct{}{}

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}

		if f.Anonymous {
			embedded = append(embedded, f)
			continue
		}

		fields = append(fields, fieldInfo{typ, f})
	}

	for _, f := range embedded {
		newTyp := f.Type
		for newTyp.Kind() == reflect.Ptr {
			newTyp = newTyp.Elem()
		}
		if newTyp.Kind() == reflect.Struct {
			fields = append(fields, getFields(newTyp, visited)...)
		}
	}

	return fields
}

// SchemaProvider is an interface that can be implemented by types to provide
// a custom schema for themselves, overriding the built-in schema generation.
// This can be used by custom types with their own special serialization rules.
type SchemaProvider interface {
	Schema(r Registry) *Schema
}

// SchemaTransformer is an interface that can be implemented by types
// to transform the generated schema as needed.
// This can be used to leverage the default schema generation for a type,
// and arbitrarily modify parts of it.
type SchemaTransformer interface {
	TransformSchema(r Registry, s *Schema) *Schema
}

// SchemaFromType returns a schema for a given type, using the registry to
// possibly create references for nested structs. The schema that is returned
// can then be passed to `huma.Validate` to efficiently validate incoming
// requests.
//
//	// Create a registry and register a type.
//	registry := huma.NewMapRegistry("#/prefix", huma.DefaultSchemaNamer)
//	schema := huma.SchemaFromType(registry, reflect.TypeOf(MyType{}))
func SchemaFromType(r Registry, t reflect.Type) *Schema {
	s := schemaFromType(r, t)
	t = deref(t)

	// Transform generated schema if type implements SchemaTransformer
	v := reflect.New(t).Interface()
	if st, ok := v.(SchemaTransformer); ok {
		s = st.TransformSchema(r, s)

		// The schema may have been modified, so recompute the error messages.
		s.PrecomputeMessages()
	}
	return s
}

func schemaFromType(r Registry, t reflect.Type) *Schema {
	isPointer := t.Kind() == reflect.Pointer

	s := Schema{}
	t = deref(t)

	v := reflect.New(t).Interface()
	if sp, ok := v.(SchemaProvider); ok {
		// Special case: type provides its own schema. Do not try to generate.
		custom := sp.Schema(r)
		custom.PrecomputeMessages()
		return custom
	}

	// Handle special cases for known stdlib types.
	switch t {
	case timeType:
		return &Schema{Type: TypeString, Nullable: isPointer, Format: "date-time"}
	case urlType:
		return &Schema{Type: TypeString, Nullable: isPointer, Format: "uri"}
	case ipType:
		return &Schema{Type: TypeString, Nullable: isPointer, Format: "ipv4"}
	case ipAddrType:
		return &Schema{Type: TypeString, Nullable: isPointer, Format: "ip"}
	case rawMessageType:
		return &Schema{}
	}

	if _, ok := v.(encoding.TextUnmarshaler); ok {
		// Special case: types that implement encoding.TextUnmarshaler are able to
		// be loaded from plain text, and so should be treated as strings.
		// This behavior can be overridden by implementing `huma.SchemaProvider`
		// and returning a custom schema.
		return &Schema{Type: TypeString, Nullable: isPointer}
	}

	minZero := 0.0
	switch t.Kind() {
	case reflect.Bool:
		s.Type = TypeBoolean
	case reflect.Int:
		s.Type = TypeInteger
		if bits.UintSize == 32 {
			s.Format = "int32"
		} else {
			s.Format = "int64"
		}
	case reflect.Int8, reflect.Int16, reflect.Int32:
		s.Type = TypeInteger
		s.Format = "int32"
	case reflect.Int64:
		s.Type = TypeInteger
		s.Format = "int64"
	case reflect.Uint:
		s.Type = TypeInteger
		if bits.UintSize == 32 {
			s.Format = "int32"
		} else {
			s.Format = "int64"
		}
		s.Minimum = &minZero
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		// Unsigned integers can't be negative.
		s.Type = TypeInteger
		s.Format = "int32"
		s.Minimum = &minZero
	case reflect.Uint64:
		// Unsigned integers can't be negative.
		s.Type = TypeInteger
		s.Format = "int64"
		s.Minimum = &minZero
	case reflect.Float32:
		s.Type = TypeNumber
		s.Format = "float"
	case reflect.Float64:
		s.Type = TypeNumber
		s.Format = "double"
	case reflect.String:
		s.Type = TypeString
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// Special case: []byte will be serialized as a base64 string.
			s.Type = TypeString
			s.ContentEncoding = "base64"
		} else {
			s.Type = TypeArray
			s.Nullable = DefaultArrayNullable
			s.Items = r.Schema(t.Elem(), true, t.Name()+"Item")

			if t.Kind() == reflect.Array {
				l := t.Len()
				s.MinItems = &l
				s.MaxItems = &l
			}
		}
	case reflect.Map:
		s.Type = TypeObject
		s.AdditionalProperties = r.Schema(t.Elem(), true, t.Name()+"Value")
	case reflect.Struct:
		var required []string
		requiredMap := map[string]bool{}
		var propNames []string
		fieldSet := map[string]struct{}{}
		props := map[string]*Schema{}
		dependentRequiredMap := map[string][]string{}
		for _, info := range getFields(t, make(map[reflect.Type]struct{})) {
			f := info.Field

			if _, ok := fieldSet[f.Name]; ok {
				// This field was overridden by an ancestor type, so we
				// should ignore it.
				continue
			}

			fieldSet[f.Name] = struct{}{}

			// Controls whether the field is required or not. All fields start as
			// required, then can be made optional with the `omitempty` JSON tag,
			// `omitzero` JSON tag, or it can be overridden manually via the
			// `required` tag.
			fieldRequired := true

			name := f.Name
			if j := f.Tag.Get("json"); j != "" {
				if n := strings.Split(j, ",")[0]; n != "" {
					name = n
				}
				if strings.Contains(j, "omitempty") {
					fieldRequired = false
				}
				if strings.Contains(j, "omitzero") {
					fieldRequired = false
				}
			}
			if name == "-" {
				// This field is deliberately ignored.
				continue
			}

			if _, ok := f.Tag.Lookup("required"); ok {
				fieldRequired = boolTag(f, "required", false)
			}

			if dr := f.Tag.Get("dependentRequired"); strings.TrimSpace(dr) != "" {
				dependentRequiredMap[name] = strings.Split(dr, ",")
			}

			fs := SchemaFromField(r, f, t.Name()+f.Name+"Struct")
			if fs != nil {
				props[name] = fs
				propNames = append(propNames, name)

				if fs.hidden {
					// This field is deliberately ignored. It may still exist, but won't
					// be documented as a required field.
					fieldRequired = false
				}

				if fieldRequired {
					required = append(required, name)
					requiredMap[name] = true
				}

				// Special case: pointer with omitempty and not manually set to
				// nullable, which will never get `null` sent over the wire.
				if f.Type.Kind() == reflect.Ptr && strings.Contains(f.Tag.Get("json"), "omitempty") && f.Tag.Get("nullable") != "true" {
					fs.Nullable = false
				}
			}
		}
		s.Type = TypeObject

		// Check if the dependent fields exists. If they don't, panic with the correct message.
		var errs []string
		depKeys := make([]string, 0, len(dependentRequiredMap))
		for field := range dependentRequiredMap {
			depKeys = append(depKeys, field)
		}
		sort.Strings(depKeys)
		for _, field := range depKeys {
			dependents := dependentRequiredMap[field]
			for _, dependent := range dependents {
				if _, ok := props[dependent]; ok {
					continue
				}
				errs = append(errs, fmt.Sprintf("dependent field '%s' for field '%s' does not exist", dependent, field))
			}
		}
		if errs != nil {
			panic(errors.New(strings.Join(errs, "; ")))
		}

		additionalProps := false
		if f, ok := t.FieldByName("_"); ok {
			if _, ok = f.Tag.Lookup("additionalProperties"); ok {
				additionalProps = boolTag(f, "additionalProperties", false)
			}

			if _, ok := f.Tag.Lookup("nullable"); ok {
				// Allow overriding nullability per struct.
				s.Nullable = boolTag(f, "nullable", false)
			}
		}
		s.AdditionalProperties = additionalProps

		s.Properties = props
		s.propertyNames = propNames
		s.Required = required
		s.DependentRequired = dependentRequiredMap
		s.requiredMap = requiredMap
		s.PrecomputeMessages()
	case reflect.Interface:
		// Interfaces mean any object.
	default:
		return nil
	}

	switch s.Type {
	case TypeBoolean, TypeInteger, TypeNumber, TypeString:
		// Scalar types which are pointers are nullable by default. This can be
		// overridden via the `nullable:"false"` field tag in structs.
		s.Nullable = isPointer
	}

	return &s
}
