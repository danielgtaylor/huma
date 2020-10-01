// Package schema implements OpenAPI 3 compatible JSON Schema which can be
// generated from structs.
package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrSchemaInvalid is sent when there is a problem building the schema.
var ErrSchemaInvalid = errors.New("schema is invalid")

// Mode defines whether the schema is being generated for read or
// write mode. Read-only fields are dropped when in write mode, for example.
type Mode int

const (
	// ModeAll is for general purpose use and includes all fields.
	ModeAll Mode = iota
	// ModeRead is for HTTP HEAD & GET and will hide write-only fields.
	ModeRead
	// ModeWrite is for HTTP POST, PUT, PATCH, DELETE and will hide
	// read-only fields.
	ModeWrite
)

var (
	timeType      = reflect.TypeOf(time.Time{})
	ipType        = reflect.TypeOf(net.IP{})
	uriType       = reflect.TypeOf(url.URL{})
	byteSliceType = reflect.TypeOf([]byte(nil))
)

// I returns a pointer to the given int. Useful helper function for pointer
// schema validators like MaxLength or MinItems.
func I(value uint64) *uint64 {
	return &value
}

// F returns a pointer to the given float64. Useful helper function for pointer
// schema validators like Maximum or Minimum.
func F(value float64) *float64 {
	return &value
}

// getTagValue returns a value of the schema's type for the given tag string.
// Uses JSON parsing if the schema is not a string.
func getTagValue(s *Schema, t reflect.Type, value string) (interface{}, error) {
	if s.Type == "string" {
		return value, nil
	}

	var v interface{}
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		return nil, err
	}

	tv := reflect.TypeOf(v)
	if v != nil && tv != t {
		if !tv.ConvertibleTo(t) {
			return nil, fmt.Errorf("unable to convert %v to %v: %w", tv, t, ErrSchemaInvalid)
		}

		v = reflect.ValueOf(v).Convert(t).Interface()
	}

	return v, nil
}

// Schema represents a JSON Schema which can be generated from Go structs
type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Description          string             `json:"description,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	AdditionalProperties interface{}        `json:"additionalProperties,omitempty"`
	PatternProperties    map[string]*Schema `json:"patternProperties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Format               string             `json:"format,omitempty"`
	Enum                 []interface{}      `json:"enum,omitempty"`
	Default              interface{}        `json:"default,omitempty"`
	Example              interface{}        `json:"example,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	ExclusiveMinimum     *float64           `json:"exclusiveMinimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	ExclusiveMaximum     *float64           `json:"exclusiveMaximum,omitempty"`
	MultipleOf           float64            `json:"multipleOf,omitempty"`
	MinLength            *uint64            `json:"minLength,omitempty"`
	MaxLength            *uint64            `json:"maxLength,omitempty"`
	Pattern              string             `json:"pattern,omitempty"`
	MinItems             *uint64            `json:"minItems,omitempty"`
	MaxItems             *uint64            `json:"maxItems,omitempty"`
	UniqueItems          bool               `json:"uniqueItems,omitempty"`
	MinProperties        *uint64            `json:"minProperties,omitempty"`
	MaxProperties        *uint64            `json:"maxProperties,omitempty"`
	AllOf                []*Schema          `json:"allOf,omitempty"`
	AnyOf                []*Schema          `json:"anyOf,omitempty"`
	OneOf                []*Schema          `json:"oneOf,omitempty"`
	Not                  *Schema            `json:"not,omitempty"`
	Nullable             bool               `json:"nullable,omitempty"`
	ReadOnly             bool               `json:"readOnly,omitempty"`
	WriteOnly            bool               `json:"writeOnly,omitempty"`
	Deprecated           bool               `json:"deprecated,omitempty"`
	ContentEncoding      string             `json:"contentEncoding,omitempty"`
}

// HasValidation returns true if at least one validator is set on the schema.
// This excludes the schema's type but includes most other fields and can be
// used to trigger additional slow validation steps when needed.
func (s *Schema) HasValidation() bool {
	if s.Items != nil || len(s.Properties) > 0 || s.AdditionalProperties != nil || len(s.PatternProperties) > 0 || len(s.Required) > 0 || len(s.Enum) > 0 || s.Minimum != nil || s.ExclusiveMinimum != nil || s.Maximum != nil || s.ExclusiveMaximum != nil || s.MultipleOf != 0 || s.MinLength != nil || s.MaxLength != nil || s.Pattern != "" || s.MinItems != nil || s.MaxItems != nil || s.UniqueItems || s.MinProperties != nil || s.MaxProperties != nil || len(s.AllOf) > 0 || len(s.AnyOf) > 0 || len(s.OneOf) > 0 || s.Not != nil {
		return true
	}

	return false
}

// RemoveProperty removes a property by name from the schema, making sure to
// also remove it from the required property set if present.
func (s *Schema) RemoveProperty(name string) {
	delete(s.Properties, name)
	for i := range s.Required {
		if s.Required[i] == name {
			s.Required[i] = s.Required[len(s.Required)-1]
			s.Required = s.Required[:len(s.Required)-1]
			break
		}
	}
}

// Generate creates a JSON schema for a Go type. Struct field tags
// can be used to provide additional metadata such as descriptions and
// validation.
func Generate(t reflect.Type) (*Schema, error) {
	return GenerateWithMode(t, ModeAll, nil)
}

// getFields performs a breadth-first search for all fields including embedded
// ones. It may return multiple fields with the same name, the first of which
// represents the outer-most declaration.
func getFields(typ reflect.Type) []reflect.StructField {
	fields := make([]reflect.StructField, 0, typ.NumField())
	embedded := []reflect.StructField{}

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Anonymous {
			embedded = append(embedded, f)
			continue
		}

		fields = append(fields, f)
	}

	for _, f := range embedded {
		newTyp := f.Type
		if newTyp.Kind() == reflect.Ptr {
			newTyp = newTyp.Elem()
		}
		fields = append(fields, getFields(newTyp)...)
	}

	return fields
}

// GenerateFromField generates a schema for a single struct field. It returns
// the computed field name, whether it is optional, its schema, and any error
// which may have occurred.
func GenerateFromField(f reflect.StructField, mode Mode) (string, bool, *Schema, error) {
	jsonTags := strings.Split(f.Tag.Get("json"), ",")
	name := strings.ToLower(f.Name)
	if len(jsonTags) > 0 && jsonTags[0] != "" {
		name = jsonTags[0]
	}

	if name == "-" {
		// Skip deliberately filtered out items
		return name, false, nil, nil
	}

	s, err := GenerateWithMode(f.Type, mode, nil)
	if err != nil {
		return name, false, nil, err
	}

	if tag, ok := f.Tag.Lookup("description"); ok {
		s.Description = tag
	}

	if tag, ok := f.Tag.Lookup("doc"); ok {
		s.Description = tag
	}

	if tag, ok := f.Tag.Lookup("format"); ok {
		s.Format = tag
	}

	if tag, ok := f.Tag.Lookup("enum"); ok {
		s.Enum = []interface{}{}
		for _, v := range strings.Split(tag, ",") {
			parsed, err := getTagValue(s, f.Type, v)
			if err != nil {
				return name, false, nil, err
			}
			s.Enum = append(s.Enum, parsed)
		}
	}

	if tag, ok := f.Tag.Lookup("default"); ok {
		v, err := getTagValue(s, f.Type, tag)
		if err != nil {
			return name, false, nil, err
		}

		s.Default = v
	}

	if tag, ok := f.Tag.Lookup("example"); ok {
		v, err := getTagValue(s, f.Type, tag)
		if err != nil {
			return name, false, nil, err
		}

		s.Example = v
	}

	if tag, ok := f.Tag.Lookup("minimum"); ok {
		min, err := strconv.ParseFloat(tag, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.Minimum = &min
	}

	if tag, ok := f.Tag.Lookup("exclusiveMinimum"); ok {
		min, err := strconv.ParseFloat(tag, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.ExclusiveMinimum = &min
	}

	if tag, ok := f.Tag.Lookup("maximum"); ok {
		max, err := strconv.ParseFloat(tag, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.Maximum = &max
	}

	if tag, ok := f.Tag.Lookup("exclusiveMaximum"); ok {
		max, err := strconv.ParseFloat(tag, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.ExclusiveMaximum = &max
	}

	if tag, ok := f.Tag.Lookup("multipleOf"); ok {
		mof, err := strconv.ParseFloat(tag, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.MultipleOf = mof
	}

	if tag, ok := f.Tag.Lookup("minLength"); ok {
		min, err := strconv.ParseUint(tag, 10, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.MinLength = &min
	}

	if tag, ok := f.Tag.Lookup("maxLength"); ok {
		max, err := strconv.ParseUint(tag, 10, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.MaxLength = &max
	}

	if tag, ok := f.Tag.Lookup("pattern"); ok {
		s.Pattern = tag

		if _, err := regexp.Compile(s.Pattern); err != nil {
			return name, false, nil, err
		}
	}

	if tag, ok := f.Tag.Lookup("minItems"); ok {
		min, err := strconv.ParseUint(tag, 10, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.MinItems = &min
	}

	if tag, ok := f.Tag.Lookup("maxItems"); ok {
		max, err := strconv.ParseUint(tag, 10, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.MaxItems = &max
	}

	if tag, ok := f.Tag.Lookup("uniqueItems"); ok {
		if !(tag == "true" || tag == "false") {
			return name, false, nil, fmt.Errorf("%s uniqueItems: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
		}
		s.UniqueItems = tag == "true"
	}

	if tag, ok := f.Tag.Lookup("minProperties"); ok {
		min, err := strconv.ParseUint(tag, 10, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.MinProperties = &min
	}

	if tag, ok := f.Tag.Lookup("maxProperties"); ok {
		max, err := strconv.ParseUint(tag, 10, 64)
		if err != nil {
			return name, false, nil, err
		}
		s.MaxProperties = &max
	}

	if tag, ok := f.Tag.Lookup("nullable"); ok {
		if !(tag == "true" || tag == "false") {
			return name, false, nil, fmt.Errorf("%s nullable: boolean should be true or false but got %s: %w", f.Name, tag, ErrSchemaInvalid)
		}
		s.Nullable = tag == "true"
	}

	if tag, ok := f.Tag.Lookup("readOnly"); ok {
		if !(tag == "true" || tag == "false") {
			return name, false, nil, fmt.Errorf("%s readOnly: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
		}
		s.ReadOnly = tag == "true"
	}

	if tag, ok := f.Tag.Lookup("writeOnly"); ok {
		if !(tag == "true" || tag == "false") {
			return name, false, nil, fmt.Errorf("%s writeOnly: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
		}
		s.WriteOnly = tag == "true"
	}

	if tag, ok := f.Tag.Lookup("deprecated"); ok {
		if !(tag == "true" || tag == "false") {
			return name, false, nil, fmt.Errorf("%s deprecated: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
		}
		s.Deprecated = tag == "true"
	}

	optional := false
	for _, tag := range jsonTags[1:] {
		if tag == "omitempty" {
			optional = true
		}
	}

	return name, optional, s, nil
}

// GenerateWithMode creates a JSON schema for a Go type. Struct field
// tags can be used to provide additional metadata such as descriptions and
// validation. The mode can be all, read, or write. In read or write mode
// any field that is marked as the opposite will be excluded, e.g. a
// write-only field would not be included in read mode. If a schema is given
// as input, add to it, otherwise creates a new schema.
func GenerateWithMode(t reflect.Type, mode Mode, schema *Schema) (*Schema, error) {
	if schema == nil {
		schema = &Schema{}
	}

	if t == ipType {
		// Special case: IP address.
		return &Schema{Type: "string", Format: "ipv4"}, nil
	}

	switch t.Kind() {
	case reflect.Struct:
		// Handle special cases.
		switch t {
		case timeType:
			return &Schema{Type: "string", Format: "date-time"}, nil
		case uriType:
			return &Schema{Type: "string", Format: "uri"}, nil
		}

		properties := make(map[string]*Schema)
		required := make([]string, 0)
		schema.Type = "object"
		schema.AdditionalProperties = false

		for _, f := range getFields(t) {
			name, optional, s, err := GenerateFromField(f, mode)
			if err != nil {
				return nil, err
			}

			if s == nil {
				// Skip deliberately filtered out items
				continue
			}

			if _, ok := properties[name]; ok {
				// Item already exists, ignore it since we process embedded fields
				// after top-level ones.
				continue
			}

			if s.ReadOnly && mode == ModeWrite {
				continue
			}

			if s.WriteOnly && mode == ModeRead {
				continue
			}

			properties[name] = s

			if !optional {
				required = append(required, name)
			}
		}

		if len(properties) > 0 {
			schema.Properties = properties
		}

		if len(required) > 0 {
			schema.Required = required
		}

	case reflect.Map:
		schema.Type = "object"
		s, err := GenerateWithMode(t.Elem(), mode, nil)
		if err != nil {
			return nil, err
		}
		schema.AdditionalProperties = s
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// Special case: `[]byte` should be a Base-64 string.
			schema.Type = "string"
			schema.ContentEncoding = "base64"
		} else {
			schema.Type = "array"
			s, err := GenerateWithMode(t.Elem(), mode, nil)
			if err != nil {
				return nil, err
			}
			schema.Items = s
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		schema.Type = "integer"
		schema.Format = "int32"
	case reflect.Int64:
		schema.Type = "integer"
		schema.Format = "int64"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		// Unsigned integers can't be negative.
		schema.Type = "integer"
		schema.Format = "int32"
		schema.Minimum = F(0.0)
	case reflect.Uint64:
		schema.Type = "integer"
		schema.Format = "int64"
		schema.Minimum = F(0.0)
	case reflect.Float32:
		schema.Type = "number"
		schema.Format = "float"
	case reflect.Float64:
		schema.Type = "number"
		schema.Format = "double"
	case reflect.Bool:
		schema.Type = "boolean"
	case reflect.String:
		schema.Type = "string"
	case reflect.Ptr:
		return GenerateWithMode(t.Elem(), mode, schema)
	case reflect.Interface:
		// Interfaces can be any type.
	default:
		return nil, fmt.Errorf("unsupported type %s from %s", t.Kind(), t)
	}

	return schema, nil
}
