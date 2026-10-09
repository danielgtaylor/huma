package huma

import (
	"encoding"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

// Value paths are independent of display locations, which may contain literal
// dots and brackets. Container diagnostics can also name an absent child.
type validationStep struct {
	name     string
	index    int
	shadowed bool
}

type validationValuePath struct {
	steps []validationStep
	null  bool
}

func (r *ValidateResult) addValue(path *PathBuffer, v any, msg string, steps []validationStep) {
	detail := &ErrorDetail{Message: msg, Location: path.String(), Value: v}
	r.Errors = append(r.Errors, detail)
	if steps != nil {
		if r.valuePaths == nil {
			r.valuePaths = make(map[*ErrorDetail]validationValuePath)
		}
		steps = slices.Clone(steps[1:])
		r.valuePaths[detail] = validationValuePath{steps: steps, null: v == nil}
	}
}

func (r *ValidateResult) addContainer(path *PathBuffer, v any, msg string) {
	steps := path.valuePath
	if len(steps) > 0 {
		steps = steps[:len(steps)-1]
	}
	r.addValue(path, v, msg, steps)
}

func (r *ValidateResult) resolveValues(body reflect.Value, original []byte) {
	for detail, path := range r.valuePaths {
		detail.Value = nil
		if path.null {
			continue
		}
		value := body
		raw := json.RawMessage(original)
		for _, step := range path.steps {
			raw = validationRawChild(raw, step)
			value = validationChild(value, step, raw)
		}
		for value.IsValid() && value.Kind() == reflect.Interface {
			value = value.Elem()
		}
		if !validationPresent(value) {
			continue
		}
		if value.Kind() != reflect.Pointer && !value.CanAddr() {
			copy := reflect.New(value.Type()).Elem()
			copy.Set(value)
			value = copy
		}
		if value.Kind() != reflect.Pointer && value.CanAddr() {
			ptr := value.Addr()
			if (!value.Type().Implements(reflect.TypeFor[json.Marshaler]()) && ptr.Type().Implements(reflect.TypeFor[json.Marshaler]())) ||
				(!value.Type().Implements(reflect.TypeFor[encoding.TextMarshaler]()) && ptr.Type().Implements(reflect.TypeFor[encoding.TextMarshaler]())) {
				value = ptr
			}
		}
		detail.Value = value.Interface()
	}
	r.valuePaths = nil
}

// A valid reflected slot can still lack a representation after custom decoding.
func validationPresent(value reflect.Value) bool {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		value = value.Elem()
	}
	if !value.IsValid() {
		return false
	}
	switch value.Kind() {
	case reflect.Map, reflect.Slice:
		return !value.IsNil()
	default:
		return true
	}
}

func validationKey(m map[string]any, name string) (string, bool, bool) {
	if _, ok := m[name]; ok {
		for key := range m {
			if key != name && strings.EqualFold(key, name) {
				return name, true, true
			}
		}
		return name, true, false
	}
	if !ValidateStrictCasing {
		found := ""
		for key := range m {
			if strings.EqualFold(key, name) {
				if found != "" {
					return found, true, true
				}
				found = key
			}
		}
		return found, found != "", false
	}
	return name, false, false
}

// RawMessage retains number precision and custom decoder lexical state. Walk
// accepted wire keys and indices, never diagnostic location strings.
func validationRawChild(raw json.RawMessage, step validationStep) json.RawMessage {
	if step.index >= 0 {
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil && step.index < len(items) {
			return items[step.index]
		}
		return nil
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(raw, &members) == nil {
		return members[step.name]
	}
	return nil
}

func validationChild(value reflect.Value, step validationStep, raw json.RawMessage) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		value = value.Elem()
	}
	if !value.IsValid() {
		return reflect.Value{}
	}
	if step.index >= 0 {
		if (value.Kind() == reflect.Array || value.Kind() == reflect.Slice) && step.index < value.Len() {
			return value.Index(step.index)
		}
		return reflect.Value{}
	}
	switch value.Kind() {
	case reflect.Map:
		if value.Type().Key().Kind() == reflect.String {
			return value.MapIndex(reflect.ValueOf(step.name).Convert(value.Type().Key()))
		}
	case reflect.Struct:
		if field, ok := validationFieldByName(value.Type(), step.name); ok {
			parentType := value.Type()
			for _, index := range field.index {
				if value.Kind() == reflect.Pointer {
					value = value.Elem()
				}
				if !value.IsValid() {
					return reflect.Value{}
				}
				value = value.Field(index)
			}
			// Restore the selected assignment before descending, but never invent
			// a field that its parent's custom decoder deliberately left absent.
			if step.shadowed && (validationPresent(value) || !reflect.PointerTo(parentType).Implements(reflect.TypeFor[json.Unmarshaler]())) {
				retyped := reflect.New(value.Type())
				if json.Unmarshal(raw, retyped.Interface()) != nil {
					return reflect.Value{}
				}
				value = retyped.Elem()
			}
			return value
		}
	}
	return reflect.Value{}
}

type validationField struct {
	name   string
	index  []int
	tagged bool
}

// Apply JSON's depth and tag dominance rules before looking up a field. An
// ambiguous promoted name has no corresponding decoded field.
func collectValidationFields(t reflect.Type, prefix []int, ancestors []reflect.Type) []validationField {
	if slices.Contains(ancestors, t) {
		return nil
	}
	ancestors = append(slices.Clone(ancestors), t)
	var candidates []validationField
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		base := field.Type
		if base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		if !field.IsExported() && (!field.Anonymous || base.Kind() != reflect.Struct) {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if !validValidationName(name) {
			name = ""
		}
		index := append(slices.Clone(prefix), i)
		if field.Anonymous && name == "" && base.Kind() == reflect.Struct {
			candidates = append(candidates, collectValidationFields(base, index, ancestors)...)
			continue
		}
		tagged := name != ""
		if name == "" {
			name = field.Name
		}
		candidates = append(candidates, validationField{name, index, tagged})
	}
	return candidates
}

func validationFields(t reflect.Type) []validationField {
	candidates := collectValidationFields(t, nil, nil)
	var fields []validationField
	for i, candidate := range candidates {
		dominated := false
		for j, other := range candidates {
			if i == j || candidate.name != other.name {
				continue
			}
			if len(other.index) < len(candidate.index) ||
				(len(other.index) == len(candidate.index) && (other.tagged || !candidate.tagged)) {
				dominated = true
				break
			}
		}
		if !dominated {
			fields = append(fields, candidate)
		}
	}
	// encoding/json restores index order after dominance filtering. The first
	// folded match uses that order, not the JSON names used to group candidates.
	slices.SortFunc(fields, func(a, b validationField) int {
		return slices.Compare(a.index, b.index)
	})
	return fields
}

func validationFieldByName(t reflect.Type, name string) (validationField, bool) {
	fields := validationFields(t)
	for _, folded := range []bool{false, true} {
		for _, field := range fields {
			if field.name == name || folded && strings.EqualFold(field.name, name) {
				return field, true
			}
		}
	}
	return validationField{}, false
}

func validValidationName(name string) bool {
	for _, c := range name {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && !strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", c) {
			return false
		}
	}
	return true
}
