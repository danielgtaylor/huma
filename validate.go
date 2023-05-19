package huma

import (
	"fmt"
	"math"
	"net"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/net/idna"
)

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
// `foo.bar.baz`. It is not thread-safe. Combined with `sync.Pool` it can
// result in zero allocations, and is used for validation. It is significantly
// better than `strings.Builder` and `bytes.Buffer` for this use case.
type PathBuffer struct {
	buf []byte
	off int
}

func (b *PathBuffer) Push(s string) {
	if b.off > 0 {
		b.buf = append(b.buf, '.')
		b.off++
	}
	b.buf = append(b.buf, s...)
	b.off += len(s)
}

func (b *PathBuffer) Pop() {
	for b.off > 0 {
		b.off--
		if b.buf[b.off] == '.' {
			b.buf = b.buf[:b.off]
			return
		}
	}
	b.buf = b.buf[:0]
}

func (b *PathBuffer) Len() int {
	return b.off
}

func (b *PathBuffer) Bytes() []byte {
	return b.buf[:b.off]
}

func (b *PathBuffer) String() string {
	return string(b.buf[:b.off])
}

func (b *PathBuffer) Reset() {
	b.buf = b.buf[:0]
	b.off = 0
}

func NewPathBuffer(buf []byte, offset int) *PathBuffer {
	return &PathBuffer{buf: buf, off: offset}
}

// ValidateResult tracks validation errors.
type ValidateResult struct {
	Errors []error
}

func (r *ValidateResult) Add(path *PathBuffer, v any, msg string) {
	r.Errors = append(r.Errors, &ErrorDetail{
		Message:  msg,
		Location: path.String(),
		Value:    v,
	})
}

func (r *ValidateResult) Addf(path *PathBuffer, v any, format string, args ...any) {
	r.Errors = append(r.Errors, &ErrorDetail{
		Message:  fmt.Sprintf(format, args...),
		Location: path.String(),
		Value:    v,
	})
}

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
	case "idn-hostname":
		if _, err := idna.ToASCII(str); err != nil {
			res.Addf(path, str, "expected string to be RFC 5890 hostname: %v", err)
		}
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
		if _, err := uuid.Parse(str); err != nil {
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

// Validate an input value against a schema, collecting errors in the validation
// result object. If successful, `res.Errors` will be empty. It is suggested
// to use a `sync.Pool` to reuse the PathBuffer and ValidateResult objects,
// making sure to call `Reset()` on them before returning them to the pool.
func Validate(r Registry, s *Schema, path *PathBuffer, mode ValidateMode, v any, res *ValidateResult) {
	// Get the actual schema if this is a reference.
	for s.Ref != "" {
		s = r.SchemaFromRef(s.Ref)
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
		case int:
			num = float64(v)
		case int64:
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
			if len(str) < *s.MinLength {
				res.Addf(path, str, s.msgMinLength)
			}
		}
		if s.MaxLength != nil {
			if len(str) > *s.MaxLength {
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
		arr, ok := v.([]any)
		if !ok {
			res.Add(path, v, "expected array")
			return
		}

		if s.MinItems != nil {
			if len(arr) < *s.MinItems {
				res.Addf(path, v, s.msgMinItems)
			}
		}
		if s.MaxItems != nil {
			if len(arr) > *s.MaxItems {
				res.Addf(path, v, s.msgMaxItems)
			}
		}

		if s.UniqueItems {
			seen := make(map[any]struct{}, len(arr))
			for _, item := range arr {
				if _, ok := seen[item]; ok {
					res.Add(path, v, "expected array items to be unique")
				}
				seen[item] = struct{}{}
			}
		}

		for i, item := range arr {
			path.Push(strconv.Itoa(i))
			Validate(r, s.Items, path, mode, item, res)
			path.Pop()
		}
	case TypeObject:
		if vv, ok := v.(map[string]any); ok {
			handleMapString(r, s, path, mode, vv, res)
			// TODO: handle map[any]any
		} else {
			res.Add(path, v, "expected object")
			return
		}
	}

	if len(s.Enum) > 0 {
		found := false
		for _, e := range s.Enum {
			if e == v {
				found = true
				break
			}
		}
		if !found {
			res.Add(path, v, s.msgEnum)
		}
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
		for v.Ref != "" {
			v = r.SchemaFromRef(v.Ref)
		}

		// We should be permissive by default to enable easy round-trips for the
		// client without needing to remove read-only values.
		// TODO: should we make this configurable?

		// Be stricter for responses, enabling validation of the server if desired.
		if mode == ModeReadFromServer && v.WriteOnly && m[k] != nil && !reflect.ValueOf(m[k]).IsZero() {
			res.Add(path, m[k], "write only property is non-zero")
			continue
		}

		if m[k] == nil {
			if !s.requiredMap[k] {
				continue
			}
			if (mode == ModeWriteToServer && v.ReadOnly) ||
				(mode == ModeReadFromServer && v.WriteOnly) {
				// These are not required for the current mode.
				continue
			}
			res.Add(path, m, s.msgRequired[k])
			continue
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
