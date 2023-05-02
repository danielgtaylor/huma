package huma

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// ErrSchemaInvalid is sent when there is a problem building the schema.
var ErrSchemaInvalid = errors.New("schema is invalid")

// JSON Schema type constants
const (
	TypeBoolean = "boolean"
	TypeInteger = "integer"
	TypeNumber  = "number"
	TypeString  = "string"
	TypeArray   = "array"
	TypeObject  = "object"
)

var (
	timeType = reflect.TypeOf(time.Time{})
	ipType   = reflect.TypeOf(net.IP{})
	uriType  = reflect.TypeOf(url.URL{})
)

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

type Schema struct {
	Type                 string             `yaml:"type,omitempty"`
	Description          string             `yaml:"description,omitempty"`
	Ref                  string             `yaml:"$ref,omitempty"`
	Format               string             `yaml:"format,omitempty"`
	Default              any                `yaml:"default,omitempty"`
	Example              any                `yaml:"example,omitempty"`
	Items                *Schema            `yaml:"items,omitempty"`
	AdditionalProperties any                `yaml:"additionalProperties,omitempty"`
	Properties           map[string]*Schema `yaml:"properties,omitempty"`
	Enum                 []any              `yaml:"enum,omitempty"`
	Minimum              *float64           `yaml:"minimum,omitempty"`
	ExclusiveMinimum     *float64           `yaml:"exclusiveMinimum,omitempty"`
	Maximum              *float64           `yaml:"maximum,omitempty"`
	ExclusiveMaximum     *float64           `yaml:"exclusiveMaximum,omitempty"`
	MultipleOf           *float64           `yaml:"multipleOf,omitempty"`
	MinLength            *int               `yaml:"minLength,omitempty"`
	MaxLength            *int               `yaml:"maxLength,omitempty"`
	Pattern              *string            `yaml:"pattern,omitempty"`
	MinItems             *int               `yaml:"minItems,omitempty"`
	MaxItems             *int               `yaml:"maxItems,omitempty"`
	UniqueItems          bool               `yaml:"uniqueItems,omitempty"`
	Required             []string           `yaml:"required,omitempty"`
	MinProperties        *int               `yaml:"minProperties,omitempty"`
	MaxProperties        *int               `yaml:"maxProperties,omitempty"`
	Extensions           map[string]any     `yaml:",inline"`

	patternRe   *regexp.Regexp  `yaml:"-"`
	requiredMap map[string]bool `yaml:"-"`

	// Precomputed validation messages
	msgEnum             string            `yaml:"-"`
	msgMinimum          string            `yaml:"-"`
	msgExclusiveMinimum string            `yaml:"-"`
	msgMaximum          string            `yaml:"-"`
	msgExclusiveMaximum string            `yaml:"-"`
	msgMultipleOf       string            `yaml:"-"`
	msgMinLength        string            `yaml:"-"`
	msgMaxLength        string            `yaml:"-"`
	msgPattern          string            `yaml:"-"`
	msgMinItems         string            `yaml:"-"`
	msgMaxItems         string            `yaml:"-"`
	msgMinProperties    string            `yaml:"-"`
	msgMaxProperties    string            `yaml:"-"`
	msgRequired         map[string]string `yaml:"-"`
}

func (s *Schema) MarshalJSON() ([]byte, error) {
	return yaml.MarshalWithOptions(s, yaml.JSON())
}

func intTag(f reflect.StructField, tag string) *int {
	if v := f.Tag.Get(tag); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return &i
		} else {
			panic(err)
		}
	}
	return nil
}

func floatTag(f reflect.StructField, tag string) *float64 {
	if v := f.Tag.Get(tag); v != "" {
		if i, err := strconv.ParseFloat(v, 64); err == nil {
			return &i
		} else {
			panic(err)
		}
	}
	return nil
}

func stringTag(f reflect.StructField, tag string) *string {
	if v := f.Tag.Get(tag); v != "" {
		return &v
	}
	return nil
}

func jsonTagValue(t reflect.Type, value string) any {
	// Special case: strings don't need quotes.
	if t.Kind() == reflect.String {
		return value
	}

	// Special case: array of strings with comma-separated values and no quotes.
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.String && value[0] != '[' {
		values := []string{}
		for _, s := range strings.Split(value, ",") {
			values = append(values, strings.TrimSpace(s))
		}
		return values
	}

	var v any
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		panic(err)
	}

	vv := reflect.ValueOf(v)
	tv := reflect.TypeOf(v)
	if v != nil && tv != t {
		if tv.Kind() == reflect.Slice {
			// Slices can't be cast due to the different layouts. Instead, we make a
			// new instance of the destination slice, and convert each value in
			// the original to the new type.
			tmp := reflect.MakeSlice(t, 0, vv.Len())
			for i := 0; i < vv.Len(); i++ {
				if !vv.Index(i).Elem().Type().ConvertibleTo(t.Elem()) {
					panic(fmt.Errorf("unable to convert %v to %v: %w", vv.Index(i).Interface(), t.Elem(), ErrSchemaInvalid))
				}

				tmp = reflect.Append(tmp, vv.Index(i).Elem().Convert(t.Elem()))
			}
			v = tmp.Interface()
		} else if !tv.ConvertibleTo(t) {
			panic(fmt.Errorf("unable to convert %v to %v: %w", tv, t, ErrSchemaInvalid))
		}

		v = reflect.ValueOf(v).Convert(t).Interface()
	}

	return v
}

// jsonTag returns a value of the schema's type for the given tag string.
// Uses JSON parsing if the schema is not a string.
func jsonTag(f reflect.StructField, name string, multi bool) any {
	t := f.Type
	if value := f.Tag.Get(name); value != "" {
		return jsonTagValue(t, value)
	}
	return nil
}

func SchemaFromField(registry Registry, parent reflect.Type, f reflect.StructField) *Schema {
	parentName := ""
	if parent != nil {
		parentName = parent.Name()
	}
	fs := registry.Schema(f.Type, true, parentName+f.Name+"Struct")
	fs.Description = f.Tag.Get("doc")
	fs.Default = jsonTag(f, "default", false)
	fs.Example = jsonTag(f, "example", false)
	if enum := f.Tag.Get("enum"); enum != "" {
		for _, e := range strings.Split(enum, ",") {
			fs.Enum = append(fs.Enum, jsonTagValue(f.Type, e))
		}
		fs.msgEnum = "expected string to be one of \"" + strings.Join(mapTo(fs.Enum, func(v any) string {
			return fmt.Sprintf("%v", v)
		}), ", ") + "\""
	}
	fs.Minimum = floatTag(f, "minimum")
	if fs.Minimum != nil {
		fs.msgMinimum = fmt.Sprintf("expected number >= %f", *fs.Minimum)
	}
	fs.ExclusiveMinimum = floatTag(f, "exclusiveMinimum")
	if fs.ExclusiveMaximum != nil {
		fs.msgExclusiveMinimum = fmt.Sprintf("expected number < %f", *fs.ExclusiveMaximum)
	}
	fs.Maximum = floatTag(f, "maximum")
	if fs.Maximum != nil {
		fs.msgMaximum = fmt.Sprintf("expected number <= %f", *fs.Maximum)
	}
	fs.ExclusiveMaximum = floatTag(f, "exclusiveMaximum")
	if fs.ExclusiveMaximum != nil {
		fs.msgExclusiveMaximum = fmt.Sprintf("expected number < %f", *fs.ExclusiveMaximum)
	}
	fs.MultipleOf = floatTag(f, "multipleOf")
	if fs.MultipleOf != nil {
		fs.msgMultipleOf = fmt.Sprintf("expected number to be a multiple of %f", *fs.MultipleOf)
	}
	fs.MinLength = intTag(f, "minLength")
	if fs.MinLength != nil {
		fs.msgMinLength = fmt.Sprintf("expected length >= %d", *fs.MinLength)
	}
	fs.MaxLength = intTag(f, "maxLength")
	if fs.MaxLength != nil {
		fs.msgMaxLength = fmt.Sprintf("expected length <= %d", *fs.MaxLength)
	}
	fs.Pattern = stringTag(f, "pattern")
	if fs.Pattern != nil {
		fs.patternRe = regexp.MustCompile(*fs.Pattern)
		fs.msgPattern = "expected string to match pattern " + *fs.Pattern
	}
	fs.MinItems = intTag(f, "minItems")
	if fs.MinItems != nil {
		fs.msgMinItems = fmt.Sprintf("expected array with at least %d items", *fs.MinItems)
	}
	fs.MaxItems = intTag(f, "maxItems")
	if fs.MaxItems != nil {
		fs.msgMaxItems = fmt.Sprintf("expected array with at most %d items", *fs.MaxItems)
	}
	fs.MinProperties = intTag(f, "minProperties")
	if fs.MinProperties != nil {
		fs.msgMinProperties = fmt.Sprintf("expected object with at least %d properties", *fs.MinProperties)
	}
	fs.MaxProperties = intTag(f, "maxProperties")
	if fs.MaxProperties != nil {
		fs.msgMaxProperties = fmt.Sprintf("expected object with at most %d properties", *fs.MaxProperties)
	}

	return fs
}

func SchemaFromType(r Registry, t reflect.Type) *Schema {
	s := Schema{}
	t = deref(t)

	if t == ipType {
		// Special case: IP address.
		return &Schema{Type: TypeString, Format: "ipv4"}
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
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
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
			s.Type = TypeString
		} else {
			s.Type = TypeArray
			s.Items = r.Schema(t.Elem(), true, t.Name()+"Item")
		}
	case reflect.Map:
		s.Type = TypeObject
		s.AdditionalProperties = r.Schema(t.Elem(), true, t.Name()+"Value")
	case reflect.Struct:
		// Handle special cases.
		switch t {
		case timeType:
			return &Schema{Type: TypeString, Format: "date-time"}
		case uriType:
			return &Schema{Type: TypeString, Format: "uri"}
		}

		required := []string{}
		requiredMap := map[string]bool{}
		props := map[string]*Schema{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)

			if !f.IsExported() {
				continue
			}

			name := f.Name
			omit := false
			if j := f.Tag.Get("json"); j != "" && j != "-" {
				name = strings.Split(j, ",")[0]
				if strings.Contains(j, "omitempty") {
					omit = true
				}
			}
			if !omit {
				required = append(required, name)
				requiredMap[name] = true
				if s.msgRequired == nil {
					s.msgRequired = map[string]string{}
				}
				s.msgRequired[name] = "expected required property " + name + " to be present"
			}

			fs := SchemaFromField(r, t, f)
			props[name] = fs
		}
		s.Type = TypeObject
		s.AdditionalProperties = false
		s.Properties = props
		s.Required = required
		s.requiredMap = requiredMap
	}

	return &s
}

// TODO: this is slow. huma.Register should cache and only try to set fields
// with actual default values defined, and we should never parse the field name
// more than once or in hot paths if we can avoid it.
func (s *Schema) SetDefaults(registry Registry, v reflect.Value) {
	if s.Ref != "" {
		s = registry.SchemaFromRef(s.Ref)
	}

	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if f.CanSet() {
				name := v.Type().Field(i).Name
				if j := v.Type().Field(i).Tag.Get("json"); j != "" && j != "-" {
					name = strings.Split(j, ",")[0]
				}

				fs := s.Properties[name]

				if fs != nil && fs.Default != nil && f.IsZero() {
					f.Set(reflect.ValueOf(fs.Default))
				}

				fs.SetDefaults(registry, f)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			s.Items.SetDefaults(registry, v.Index(i))
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			s.AdditionalProperties.(*Schema).SetDefaults(registry, v.MapIndex(k))
		}
	}
}
