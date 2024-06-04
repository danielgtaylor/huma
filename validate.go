package huma

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
	"unsafe"
)

// ValidateMode describes the direction of validation (server -> client or
// client -> server). It impacts things like how read-only or write-only fields
// are handled.
type ValidateMode int

const (
	// ModeReadFromServer is a read mode (response output) that may ignore or
	// reject write-only fields that are non-zero, as these write-only fields
	// are meant to be sent by the client.
	ModeReadFromServer ValidateMode = iota

	// ModeWriteToServer is a write mode (request input) that may ignore or
	// reject read-only fields that are non-zero, as these are owned by the
	// server and the client should not try to modify them.
	ModeWriteToServer
)

var rxHostname = regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)
var rxURITemplate = regexp.MustCompile("^([^{]*({[^}]*})?)*$")
var rxJSONPointer = regexp.MustCompile("^(?:/(?:[^~/]|~0|~1)*)*$")
var rxRelJSONPointer = regexp.MustCompile("^(?:0|[1-9][0-9]*)(?:#|(?:/(?:[^~/]|~0|~1)*)*)$")
var rxBase64 = regexp.MustCompile(`^[a-zA-Z0-9+/_-]+=*$`)

func mapTo[A, B any](s []A, f func(A) B) []B {
	r := make([]B, len(s))
	for i, v := range s {
		r[i] = f(v)
	}
	return r
}

// PathBuffer is a low-allocation helper for building a path string like
// `foo.bar.baz`. It is not goroutine-safe. Combined with `sync.Pool` it can
// result in zero allocations, and is used for validation. It is significantly
// better than `strings.Builder` and `bytes.Buffer` for this use case.
//
// Path buffers can be converted to strings for use in responses or printing
// using either the `pb.String()` or `pb.With("field")` methods.
//
//	pb := NewPathBuffer([]byte{}, 0)
//	pb.Push("foo")  // foo
//	pb.PushIndex(1) // foo[1]
//	pb.Push("bar")  // foo[1].bar
//	pb.Pop()        // foo[1]
//	pb.Pop()        // foo
type PathBuffer struct {
	buf []byte
	off int
}

// Push an entry onto the path, adding a `.` separator as needed.
//
//	pb.Push("foo") // foo
//	pb.Push("bar") // foo.bar
func (b *PathBuffer) Push(s string) {
	if b.off > 0 {
		b.buf = append(b.buf, '.')
		b.off++
	}
	b.buf = append(b.buf, s...)
	b.off += len(s)
}

// PushIndex pushes an entry onto the path surrounded by `[` and `]`.
//
//	pb.Push("foo")  // foo
//	pb.PushIndex(1) // foo[1]
func (b *PathBuffer) PushIndex(i int) {
	l := len(b.buf)
	b.buf = append(b.buf, '[')
	b.buf = append(b.buf, strconv.Itoa(i)...)
	b.buf = append(b.buf, ']')
	b.off += len(b.buf) - l
}

// Pop the latest entry off the path.
//
//	pb.Push("foo")  // foo
//	pb.PushIndex(1) // foo[1]
//	pb.Push("bar")  // foo[1].bar
//	pb.Pop()        // foo[1]
//	pb.Pop()        // foo
func (b *PathBuffer) Pop() {
	for b.off > 0 {
		b.off--
		if b.buf[b.off] == '.' || b.buf[b.off] == '[' {
			break
		}
	}
	b.buf = b.buf[:b.off]
}

// With is shorthand for push, convert to string, and pop. This is useful
// when you want the location of a field given a path buffer as a prefix.
//
//	pb.Push("foo")
//	pb.With("bar") // returns foo.bar
func (b *PathBuffer) With(s string) string {
	b.Push(s)
	tmp := b.String()
	b.Pop()
	return tmp
}

// Len returns the length of the current path.
func (b *PathBuffer) Len() int {
	return b.off
}

// Bytes returns the underlying slice of bytes of the path.
func (b *PathBuffer) Bytes() []byte {
	return b.buf[:b.off]
}

// String converts the path buffer to a string.
func (b *PathBuffer) String() string {
	return string(b.buf[:b.off])
}

// Reset the path buffer to empty, keeping and reusing the underlying bytes.
func (b *PathBuffer) Reset() {
	b.buf = b.buf[:0]
	b.off = 0
}

// NewPathBuffer creates a new path buffer given an existing byte slice.
// Tip: using `sync.Pool` can significantly reduce buffer allocations.
//
//	pb := NewPathBuffer([]byte{}, 0)
//	pb.Push("foo")
func NewPathBuffer(buf []byte, offset int) *PathBuffer {
	return &PathBuffer{buf: buf, off: offset}
}

// ValidateResult tracks validation errors. It is safe to use for multiple
// validations as long as `Reset()` is called between uses.
type ValidateResult struct {
	Errors []error
}

// Add an error to the validation result at the given path and with the
// given value.
func (r *ValidateResult) Add(path *PathBuffer, v any, msg string) {
	r.Errors = append(r.Errors, &ErrorDetail{
		Message:  msg,
		Location: path.String(),
		Value:    v,
	})
}

// Addf adds an error to the validation result at the given path and with
// the given value, allowing for fmt.Printf-style formatting.
func (r *ValidateResult) Addf(path *PathBuffer, v any, format string, args ...any) {
	r.Errors = append(r.Errors, &ErrorDetail{
		Message:  fmt.Sprintf(format, args...),
		Location: path.String(),
		Value:    v,
	})
}

// Reset the validation error so it can be used again.
func (r *ValidateResult) Reset() {
	r.Errors = r.Errors[:0]
}

func validateFormat(path *PathBuffer, str string, s *Schema, res *ValidateResult) {
	switch s.Format {
	case "date-time":
		found := false
		for _, format := range []string{time.RFC3339, time.RFC3339Nano} {
			if _, err := time.Parse(format, str); err == nil {
				found = true
				break
			}
		}
		if !found {
			res.Add(path, str, "expected string to be RFC 3339 date-time")
		}
	case "date-time-http":
		if _, err := time.Parse(time.RFC1123, str); err != nil {
			res.Add(path, str, "expected string to be RFC 1123 date-time")
		}
	case "date":
		if _, err := time.Parse("2006-01-02", str); err != nil {
			res.Add(path, str, "expected string to be RFC 3339 date")
		}
	case "time":
		if _, err := time.Parse("15:04:05", str); err != nil {
			if _, err := time.Parse("15:04:05Z07:00", str); err != nil {
				res.Add(path, str, "expected string to be RFC 3339 time")
			}
		}
		// TODO: duration
	case "email", "idn-email":
		if _, err := mail.ParseAddress(str); err != nil {
			res.Addf(path, str, "expected string to be RFC 5322 email: %v", err)
		}
	case "hostname":
		if !(rxHostname.MatchString(str) && len(str) < 256) {
			res.Add(path, str, "expected string to be RFC 5890 hostname")
		}
	// TODO: proper idn-hostname support... need to figure out how.
	case "ipv4":
		if ip := net.ParseIP(str); ip == nil || ip.To4() == nil {
			res.Add(path, str, "expected string to be RFC 2673 ipv4")
		}
	case "ipv6":
		if ip := net.ParseIP(str); ip == nil || ip.To16() == nil {
			res.Add(path, str, "expected string to be RFC 2373 ipv6")
		}
	case "uri", "uri-reference", "iri", "iri-reference":
		if _, err := url.Parse(str); err != nil {
			res.Addf(path, str, "expected string to be RFC 3986 uri: %v", err)
		}
		// TODO: check if it's actually a reference?
	case "uuid":
		if err := validateUUID(str); err != nil {
			res.Addf(path, str, "expected string to be RFC 4122 uuid: %v", err)
		}
	case "uri-template":
		u, err := url.Parse(str)
		if err != nil {
			res.Addf(path, str, "expected string to be RFC 3986 uri: %v", err)
			return
		}
		if !rxURITemplate.MatchString(u.Path) {
			res.Add(path, str, "expected string to be RFC 6570 uri-template")
		}
	case "json-pointer":
		if !rxJSONPointer.MatchString(str) {
			res.Add(path, str, "expected string to be RFC 6901 json-pointer")
		}
	case "relative-json-pointer":
		if !rxRelJSONPointer.MatchString(str) {
			res.Add(path, str, "expected string to be RFC 6901 relative-json-pointer")
		}
	case "regex":
		if _, err := regexp.Compile(str); err != nil {
			res.Addf(path, str, "expected string to be regex: %v", err)
		}
	}
}

func validateOneOf(r Registry, s *Schema, path *PathBuffer, mode ValidateMode, v any, res *ValidateResult) {
	found := false
	subRes := &ValidateResult{}
	for _, sub := range s.OneOf {
		Validate(r, sub, path, mode, v, subRes)
		if len(subRes.Errors) == 0 {
			if found {
				res.Add(path, v, "expected value to match exactly one schema but matched multiple")
			}
			found = true
		}
		subRes.Reset()
	}
	if !found {
		res.Add(path, v, "expected value to match exactly one schema but matched none")
	}
}

func validateAnyOf(r Registry, s *Schema, path *PathBuffer, mode ValidateMode, v any, res *ValidateResult) {
	matches := 0
	subRes := &ValidateResult{}
	for _, sub := range s.AnyOf {
		Validate(r, sub, path, mode, v, subRes)
		if len(subRes.Errors) == 0 {
			matches++
		}
		subRes.Reset()
	}

	if matches == 0 {
		res.Add(path, v, "expected value to match at least one schema but matched none")
	}
}

// Validate an input value against a schema, collecting errors in the validation
// result object. If successful, `res.Errors` will be empty. It is suggested
// to use a `sync.Pool` to reuse the PathBuffer and ValidateResult objects,
// making sure to call `Reset()` on them before returning them to the pool.
//
//	registry := huma.NewMapRegistry("#/prefix", huma.DefaultSchemaNamer)
//	schema := huma.SchemaFromType(registry, reflect.TypeOf(MyType{}))
//	pb := huma.NewPathBuffer([]byte(""), 0)
//	res := &huma.ValidateResult{}
//
//	var value any
//	json.Unmarshal([]byte(`{"foo": "bar"}`), &v)
//	huma.Validate(registry, schema, pb, huma.ModeWriteToServer, value, res)
//	for _, err := range res.Errors {
//		fmt.Println(err.Error())
//	}
func Validate(r Registry, s *Schema, path *PathBuffer, mode ValidateMode, v any, res *ValidateResult) {
	// Get the actual schema if this is a reference.
	for s.Ref != "" {
		s = r.SchemaFromRef(s.Ref)
	}

	if s.OneOf != nil {
		validateOneOf(r, s, path, mode, v, res)
	}

	if s.AnyOf != nil {
		validateAnyOf(r, s, path, mode, v, res)
	}

	if s.AllOf != nil {
		for _, sub := range s.AllOf {
			Validate(r, sub, path, mode, v, res)
		}
	}

	if s.Not != nil {
		subRes := &ValidateResult{}
		Validate(r, s.Not, path, mode, v, subRes)
		if len(subRes.Errors) == 0 {
			res.Add(path, v, "expected value to not match schema")
		}
	}

	if s.Nullable && v == nil {
		return
	}

	switch s.Type {
	case TypeBoolean:
		if _, ok := v.(bool); !ok {
			res.Add(path, v, "expected boolean")
			return
		}
	case TypeNumber, TypeInteger:
		var num float64

		switch v := v.(type) {
		case float64:
			num = v
		case float32:
			num = float64(v)
		case int:
			num = float64(v)
		case int8:
			num = float64(v)
		case int16:
			num = float64(v)
		case int32:
			num = float64(v)
		case int64:
			num = float64(v)
		case uint:
			num = float64(v)
		case uint8:
			num = float64(v)
		case uint16:
			num = float64(v)
		case uint32:
			num = float64(v)
		case uint64:
			num = float64(v)
		default:
			res.Add(path, v, "expected number")
			return
		}

		if s.Minimum != nil {
			if num < *s.Minimum {
				res.Addf(path, v, s.msgMinimum)
			}
		}
		if s.ExclusiveMinimum != nil {
			if num <= *s.ExclusiveMinimum {
				res.Addf(path, v, s.msgExclusiveMinimum)
			}
		}
		if s.Maximum != nil {
			if num > *s.Maximum {
				res.Add(path, v, s.msgMaximum)
			}
		}
		if s.ExclusiveMaximum != nil {
			if num >= *s.ExclusiveMaximum {
				res.Addf(path, v, s.msgExclusiveMaximum)
			}
		}
		if s.MultipleOf != nil {
			if math.Mod(num, *s.MultipleOf) != 0 {
				res.Addf(path, v, s.msgMultipleOf)
			}
		}
	case TypeString:
		str, ok := v.(string)
		if !ok {
			if b, ok := v.([]byte); ok {
				str = *(*string)(unsafe.Pointer(&b))
			} else {
				res.Add(path, v, "expected string")
				return
			}
		}

		if s.MinLength != nil {
			if utf8.RuneCountInString(str) < *s.MinLength {
				res.Addf(path, str, s.msgMinLength)
			}
		}
		if s.MaxLength != nil {
			if utf8.RuneCountInString(str) > *s.MaxLength {
				res.Add(path, str, s.msgMaxLength)
			}
		}
		if s.patternRe != nil {
			if !s.patternRe.MatchString(str) {
				res.Add(path, v, s.msgPattern)
			}
		}

		if s.Format != "" {
			validateFormat(path, str, s, res)
		}

		if s.ContentEncoding == "base64" {
			if !rxBase64.MatchString(str) {
				res.Add(path, str, "expected string to be base64 encoded")
			}
		}
	case TypeArray:
		switch arr := v.(type) {
		case []any:
			handleArray(r, s, path, mode, res, arr)
		// Special cases for params which are lists.
		case []string:
			handleArray(r, s, path, mode, res, arr)
		case []int:
			handleArray(r, s, path, mode, res, arr)
		case []int8:
			handleArray(r, s, path, mode, res, arr)
		case []int16:
			handleArray(r, s, path, mode, res, arr)
		case []int32:
			handleArray(r, s, path, mode, res, arr)
		case []int64:
			handleArray(r, s, path, mode, res, arr)
		case []uint:
			handleArray(r, s, path, mode, res, arr)
		case []uint16:
			handleArray(r, s, path, mode, res, arr)
		case []uint32:
			handleArray(r, s, path, mode, res, arr)
		case []uint64:
			handleArray(r, s, path, mode, res, arr)
		case []float32:
			handleArray(r, s, path, mode, res, arr)
		case []float64:
			handleArray(r, s, path, mode, res, arr)
		default:
			res.Add(path, v, "expected array")
			return
		}
	case TypeObject:
		if vv, ok := v.(map[string]any); ok {
			handleMapString(r, s, path, mode, vv, res)
		} else if vv, ok := v.(map[any]any); ok {
			handleMapAny(r, s, path, mode, vv, res)
		} else {
			res.Add(path, v, "expected object")
			return
		}
	}

	if len(s.Enum) > 0 {
		found := false
		vs := fmt.Sprint(v)
		for _, e := range s.Enum {
			if fmt.Sprint(e) == vs {
				found = true
				break
			}
		}
		if !found {
			res.Add(path, v, s.msgEnum)
		}
	}
}

func handleArray[T any](r Registry, s *Schema, path *PathBuffer, mode ValidateMode, res *ValidateResult, arr []T) {
	if s.MinItems != nil {
		if len(arr) < *s.MinItems {
			res.Addf(path, arr, s.msgMinItems)
		}
	}
	if s.MaxItems != nil {
		if len(arr) > *s.MaxItems {
			res.Addf(path, arr, s.msgMaxItems)
		}
	}

	if s.UniqueItems {
		seen := make(map[any]struct{}, len(arr))
		for _, item := range arr {
			if _, ok := seen[item]; ok {
				res.Add(path, arr, "expected array items to be unique")
			}
			seen[item] = struct{}{}
		}
	}

	for i, item := range arr {
		path.PushIndex(i)
		Validate(r, s.Items, path, mode, item, res)
		path.Pop()
	}
}

func handleMapString(r Registry, s *Schema, path *PathBuffer, mode ValidateMode, m map[string]any, res *ValidateResult) {
	if s.MinProperties != nil {
		if len(m) < *s.MinProperties {
			res.Add(path, m, s.msgMinProperties)
		}
	}
	if s.MaxProperties != nil {
		if len(m) > *s.MaxProperties {
			res.Add(path, m, s.msgMaxProperties)
		}
	}

	for _, k := range s.propertyNames {
		v := s.Properties[k]

		// Schemas are generated such that the read/write-only properties are set
		// alongside the `$ref`, if it is present (i.e. for objects). If not,
		// then the read/write-only properties are set directly on the schema and
		// the `for` loop never runs.
		readOnly := v.ReadOnly
		writeOnly := v.WriteOnly
		for v.Ref != "" {
			v = r.SchemaFromRef(v.Ref)
		}

		// We should be permissive by default to enable easy round-trips for the
		// client without needing to remove read-only values.
		// TODO: should we make this configurable?

		// Be stricter for responses, enabling validation of the server if desired.
		if mode == ModeReadFromServer && writeOnly && m[k] != nil && !reflect.ValueOf(m[k]).IsZero() {
			res.Add(path, m[k], "write only property is non-zero")
			continue
		}

		if _, ok := m[k]; !ok {
			if !s.requiredMap[k] {
				continue
			}
			if (mode == ModeWriteToServer && readOnly) ||
				(mode == ModeReadFromServer && writeOnly) {
				// These are not required for the current mode.
				continue
			}
			res.Add(path, m, s.msgRequired[k])
			continue
		}

		if m[k] == nil && (!s.requiredMap[k] || s.Nullable) {
			// This is a non-required field which is null, or a nullable field set
			// to null, so ignore it.
			continue
		}

		if m[k] != nil && s.DependentRequired[k] != nil {
			for _, dependent := range s.DependentRequired[k] {
				if m[dependent] != nil {
					continue
				}

				res.Add(path, m, s.msgDependentRequired[k][dependent])
			}
		}

		path.Push(k)
		Validate(r, v, path, mode, m[k], res)
		path.Pop()
	}

	if addl, ok := s.AdditionalProperties.(bool); ok && !addl {
		for k := range m {
			// No additional properties allowed.
			if _, ok := s.Properties[k]; !ok {
				path.Push(k)
				res.Add(path, m, "unexpected property")
				path.Pop()
			}
		}
	}

	if addl, ok := s.AdditionalProperties.(*Schema); ok {
		// Additional properties are allowed, but must match the given schema.
		for k, v := range m {
			if _, ok := s.Properties[k]; ok {
				continue
			}

			path.Push(k)
			Validate(r, addl, path, mode, v, res)
			path.Pop()
		}
	}
}

func handleMapAny(r Registry, s *Schema, path *PathBuffer, mode ValidateMode, m map[any]any, res *ValidateResult) {
	if s.MinProperties != nil {
		if len(m) < *s.MinProperties {
			res.Add(path, m, s.msgMinProperties)
		}
	}
	if s.MaxProperties != nil {
		if len(m) > *s.MaxProperties {
			res.Add(path, m, s.msgMaxProperties)
		}
	}

	for _, k := range s.propertyNames {
		v := s.Properties[k]

		// Schemas are generated such that the read/write-only properties are set
		// alongside the `$ref`, if it is present (i.e. for objects). If not,
		// then the read/write-only properties are set directly on the schema and
		// the `for` loop never runs.
		readOnly := v.ReadOnly
		writeOnly := v.WriteOnly
		for v.Ref != "" {
			v = r.SchemaFromRef(v.Ref)
		}

		// We should be permissive by default to enable easy round-trips for the
		// client without needing to remove read-only values.
		// TODO: should we make this configurable?

		// Be stricter for responses, enabling validation of the server if desired.
		if mode == ModeReadFromServer && writeOnly && m[k] != nil && !reflect.ValueOf(m[k]).IsZero() {
			res.Add(path, m[k], "write only property is non-zero")
			continue
		}

		if _, ok := m[k]; !ok {
			if !s.requiredMap[k] {
				continue
			}
			if (mode == ModeWriteToServer && readOnly) ||
				(mode == ModeReadFromServer && writeOnly) {
				// These are not required for the current mode.
				continue
			}
			res.Add(path, m, s.msgRequired[k])
			continue
		}

		if m[k] == nil && (!s.requiredMap[k] || s.Nullable) {
			// This is a non-required field which is null, or a nullable field set
			// to null, so ignore it.
			continue
		}

		if m[k] != nil && s.DependentRequired[k] != nil {
			for _, dependent := range s.DependentRequired[k] {
				if m[dependent] != nil {
					continue
				}

				res.Add(path, m, s.msgDependentRequired[k][dependent])
			}
		}

		path.Push(k)
		Validate(r, v, path, mode, m[k], res)
		path.Pop()
	}

	if addl, ok := s.AdditionalProperties.(bool); ok && !addl {
		for k := range m {
			// No additional properties allowed.
			var kStr string
			if s, ok := k.(string); ok {
				kStr = s
			} else {
				kStr = fmt.Sprint(k)
			}
			if _, ok := s.Properties[kStr]; !ok {
				path.Push(kStr)
				res.Add(path, m, "unexpected property")
				path.Pop()
			}
		}
	}

	if addl, ok := s.AdditionalProperties.(*Schema); ok {
		// Additional properties are allowed, but must match the given schema.
		for k, v := range m {
			var kStr string
			if s, ok := k.(string); ok {
				kStr = s
			} else {
				kStr = fmt.Sprint(k)
			}
			path.Push(kStr)
			Validate(r, addl, path, mode, v, res)
			path.Pop()
		}
	}
}

// ModelValidator is a utility for validating e.g. JSON loaded data against a
// Go struct model. It is not goroutine-safe and should not be used in HTTP
// handlers! Schemas are generated on-the-fly on first use and re-used on
// subsequent calls. This utility can be used to easily validate data outside
// of the normal request/response flow, for example on application startup:
//
//	type MyExample struct {
//		Name string `json:"name" maxLength:"5"`
//		Age int `json:"age" minimum:"25"`
//	}
//
//	var value any
//	json.Unmarshal([]byte(`{"name": "abcdefg", "age": 1}`), &value)
//
//	validator := ModelValidator()
//	errs := validator.Validate(reflect.TypeOf(MyExample{}), value)
//	if errs != nil {
//		fmt.Println("Validation error", errs)
//	}
type ModelValidator struct {
	registry Registry
	pb       *PathBuffer
	result   *ValidateResult
}

// NewModelValidator creates a new model validator with all the components
// it needs to create schemas, validate them, and return any errors.
func NewModelValidator() *ModelValidator {
	return &ModelValidator{
		registry: NewMapRegistry("#/components/schemas/", DefaultSchemaNamer),
		pb:       NewPathBuffer([]byte(""), 0),
		result:   &ValidateResult{},
	}
}

// Validate the inputs. The type should be the Go struct with validation field
// tags and the value should be e.g. JSON loaded into an `any`. A list of
// errors is returned if validation failed, otherwise `nil`.
//
//	type MyExample struct {
//		Name string `json:"name" maxLength:"5"`
//		Age int `json:"age" minimum:"25"`
//	}
//
//	var value any
//	json.Unmarshal([]byte(`{"name": "abcdefg", "age": 1}`), &value)
//
//	validator := ModelValidator()
//	errs := validator.Validate(reflect.TypeOf(MyExample{}), value)
//	if errs != nil {
//		fmt.Println("Validation error", errs)
//	}
func (v *ModelValidator) Validate(typ reflect.Type, value any) []error {
	v.pb.Reset()
	v.result.Reset()

	s := v.registry.Schema(typ, true, typ.Name())

	Validate(v.registry, s, v.pb, ModeReadFromServer, value, v.result)

	if len(v.result.Errors) > 0 {
		return v.result.Errors
	}
	return nil
}

// The following is borrowed from the Google UUID package:
// https://github.com/google/uuid/blob/v1.6.0/uuid.go
// Copyright (c) 2009,2014 Google Inc. All rights reserved.
// BSD licensed.

type invalidLengthError struct{ len int }

func (err invalidLengthError) Error() string {
	return fmt.Sprintf("invalid UUID length: %d", err.len)
}

// xvalues returns the value of a byte as a hexadecimal digit or 255.
var xvalues = [256]byte{
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 255, 255, 255, 255, 255, 255,
	255, 10, 11, 12, 13, 14, 15, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 10, 11, 12, 13, 14, 15, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
}

// xtob converts hex characters x1 and x2 into a byte.
//
//nolint:unparam
func xtob(x1, x2 byte) (byte, bool) {
	b1 := xvalues[x1]
	b2 := xvalues[x2]
	return (b1 << 4) | b2, b1 != 255 && b2 != 255
}

// validateUUID returns an error if s is not a properly formatted UUID in one of the following formats:
//
//	xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
//	urn:uuid:xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
//	xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
//	{xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx}
//
// It returns an error if the format is invalid, otherwise nil.
func validateUUID(s string) error {
	switch len(s) {
	// Standard UUID format
	case 36:

	// UUID with "urn:uuid:" prefix
	case 36 + 9:
		if !strings.EqualFold(s[:9], "urn:uuid:") {
			return fmt.Errorf("invalid urn prefix: %q", s[:9])
		}
		s = s[9:]

	// UUID enclosed in braces
	case 36 + 2:
		if s[0] != '{' || s[len(s)-1] != '}' {
			return errors.New("invalid bracketed UUID format")
		}
		s = s[1 : len(s)-1]

	// UUID without hyphens
	case 32:
		for i := 0; i < len(s); i += 2 {
			_, ok := xtob(s[i], s[i+1])
			if !ok {
				return errors.New("invalid UUID format")
			}
		}

	default:
		return invalidLengthError{len(s)}
	}

	// Check for standard UUID format
	if len(s) == 36 {
		if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
			return errors.New("invalid UUID format")
		}
		for _, x := range []int{0, 2, 4, 6, 9, 11, 14, 16, 19, 21, 24, 26, 28, 30, 32, 34} {
			if _, ok := xtob(s[x], s[x+1]); !ok {
				return errors.New("invalid UUID format")
			}
		}
	}

	return nil
}
