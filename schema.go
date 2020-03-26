package huma

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

var (
	timeType      = reflect.TypeOf(time.Time{})
	ipType        = reflect.TypeOf(net.IP{})
	uriType       = reflect.TypeOf(url.URL{})
	byteSliceType = reflect.TypeOf([]byte(nil))
)

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
}

// GenerateSchema creates a JSON schema for a Go type. Struct field tags
// can be used to provide additional metadata such as descriptions and
// validation.
func GenerateSchema(t reflect.Type) (*Schema, error) {
	schema := &Schema{}

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

		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)

			jsonTags := strings.Split(f.Tag.Get("json"), ",")

			name := f.Name
			if len(jsonTags) > 0 {
				name = jsonTags[0]
			}

			s, err := GenerateSchema(f.Type)
			if err != nil {
				return nil, err
			}
			properties[name] = s

			if tag, ok := f.Tag.Lookup("description"); ok {
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
						return nil, err
					}
					s.Enum = append(s.Enum, parsed)
				}
			}

			if tag, ok := f.Tag.Lookup("default"); ok {
				v, err := getTagValue(s, f.Type, tag)
				if err != nil {
					return nil, err
				}

				s.Default = v
			}

			if tag, ok := f.Tag.Lookup("example"); ok {
				v, err := getTagValue(s, f.Type, tag)
				if err != nil {
					return nil, err
				}

				s.Example = v
			}

			if tag, ok := f.Tag.Lookup("minimum"); ok {
				min, err := strconv.ParseFloat(tag, 64)
				if err != nil {
					return nil, err
				}
				s.Minimum = &min
			}

			if tag, ok := f.Tag.Lookup("exclusiveMinimum"); ok {
				min, err := strconv.ParseFloat(tag, 64)
				if err != nil {
					return nil, err
				}
				s.ExclusiveMinimum = &min
			}

			if tag, ok := f.Tag.Lookup("maximum"); ok {
				max, err := strconv.ParseFloat(tag, 64)
				if err != nil {
					return nil, err
				}
				s.Maximum = &max
			}

			if tag, ok := f.Tag.Lookup("exclusiveMaximum"); ok {
				max, err := strconv.ParseFloat(tag, 64)
				if err != nil {
					return nil, err
				}
				s.ExclusiveMaximum = &max
			}

			if tag, ok := f.Tag.Lookup("multipleOf"); ok {
				mof, err := strconv.ParseFloat(tag, 64)
				if err != nil {
					return nil, err
				}
				s.MultipleOf = mof
			}

			if tag, ok := f.Tag.Lookup("minLength"); ok {
				min, err := strconv.ParseUint(tag, 10, 64)
				if err != nil {
					return nil, err
				}
				s.MinLength = &min
			}

			if tag, ok := f.Tag.Lookup("maxLength"); ok {
				max, err := strconv.ParseUint(tag, 10, 64)
				if err != nil {
					return nil, err
				}
				s.MaxLength = &max
			}

			if tag, ok := f.Tag.Lookup("pattern"); ok {
				s.Pattern = tag

				if _, err := regexp.Compile(s.Pattern); err != nil {
					return nil, err
				}
			}

			if tag, ok := f.Tag.Lookup("minItems"); ok {
				min, err := strconv.ParseUint(tag, 10, 64)
				if err != nil {
					return nil, err
				}
				s.MinItems = &min
			}

			if tag, ok := f.Tag.Lookup("maxItems"); ok {
				max, err := strconv.ParseUint(tag, 10, 64)
				if err != nil {
					return nil, err
				}
				s.MaxItems = &max
			}

			if tag, ok := f.Tag.Lookup("uniqueItems"); ok {
				if !(tag == "true" || tag == "false") {
					return nil, fmt.Errorf("%s uniqueItems: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
				}
				s.UniqueItems = tag == "true"
			}

			if tag, ok := f.Tag.Lookup("minProperties"); ok {
				min, err := strconv.ParseUint(tag, 10, 64)
				if err != nil {
					return nil, err
				}
				s.MinProperties = &min
			}

			if tag, ok := f.Tag.Lookup("maxProperties"); ok {
				max, err := strconv.ParseUint(tag, 10, 64)
				if err != nil {
					return nil, err
				}
				s.MaxProperties = &max
			}

			if tag, ok := f.Tag.Lookup("nullable"); ok {
				if !(tag == "true" || tag == "false") {
					return nil, fmt.Errorf("%s nullable: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
				}
				s.Nullable = tag == "true"
			}

			if tag, ok := f.Tag.Lookup("readOnly"); ok {
				if !(tag == "true" || tag == "false") {
					return nil, fmt.Errorf("%s readOnly: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
				}
				s.ReadOnly = tag == "true"
			}

			if tag, ok := f.Tag.Lookup("writeOnly"); ok {
				if !(tag == "true" || tag == "false") {
					return nil, fmt.Errorf("%s writeOnly: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
				}
				s.WriteOnly = tag == "true"
			}

			if tag, ok := f.Tag.Lookup("deprecated"); ok {
				if !(tag == "true" || tag == "false") {
					return nil, fmt.Errorf("%s deprecated: boolean should be true or false: %w", f.Name, ErrSchemaInvalid)
				}
				s.Deprecated = tag == "true"
			}

			optional := false
			for _, tag := range jsonTags[1:] {
				if tag == "omitempty" {
					optional = true
				}
			}
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
		s, err := GenerateSchema(t.Elem())
		if err != nil {
			return nil, err
		}
		schema.AdditionalProperties = s
	case reflect.Slice, reflect.Array:
		schema.Type = "array"
		s, err := GenerateSchema(t.Elem())
		if err != nil {
			return nil, err
		}
		schema.Items = s
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &Schema{
			Type: "integer",
		}, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		// Unsigned integers can't be negative.
		min := 0.0
		return &Schema{
			Type:    "integer",
			Minimum: &min,
		}, nil
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}, nil
	case reflect.Bool:
		return &Schema{Type: "boolean"}, nil
	case reflect.String:
		return &Schema{Type: "string"}, nil
	case reflect.Ptr:
		return GenerateSchema(t.Elem())
	default:
		return nil, fmt.Errorf("unsupported type %s from %s", t.Kind(), t)
	}

	return schema, nil
}
