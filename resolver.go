package huma

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/schema"
	"github.com/go-chi/chi"
	"github.com/xeipuuv/gojsonschema"
)

var timeType = reflect.TypeOf(time.Time{})
var readerType = reflect.TypeOf((*io.Reader)(nil)).Elem()

// Resolver provides a way to resolve input values from a request or to post-
// process input values in some way, including additional validation beyond
// what is possible with JSON Schema alone. If any errors are added to the
// context, then the client will get a 400 Bad Request response.
type Resolver interface {
	Resolve(ctx Context, r *http.Request)
}

// Checks if data validates against the given schema. Returns false on failure.
func validAgainstSchema(ctx *hcontext, label string, schema *schema.Schema, data []byte) bool {
	defer func() {
		// Catch panics from the `gojsonschema` library.
		if err := recover(); err != nil {
			ctx.AddError(&ErrorDetail{
				Message:  fmt.Errorf("unable to validate against schema: %w", err.(error)).Error(),
				Location: label,
				Value:    string(data),
			})

			// TODO: log error?
		}
	}()

	// TODO: load and pre-cache schemas once per operation
	loader := gojsonschema.NewGoLoader(schema)
	doc := gojsonschema.NewBytesLoader(data)
	s, err := gojsonschema.NewSchema(loader)
	if err != nil {
		panic(err)
	}
	result, err := s.Validate(doc)
	if err != nil {
		panic(err)
	}

	if !result.Valid() {
		for _, desc := range result.Errors() {
			// Note: some descriptions start with the context location so we trim
			// those off to prevent duplicating data. (e.g. see the enum error)
			ctx.AddError(&ErrorDetail{
				Message:  strings.TrimLeft(desc.Description(), desc.Context().String()+" "),
				Location: label + strings.TrimLeft(desc.Field(), "(root)"),
				Value:    desc.Value(),
			})
		}
		return false
	}

	return true
}

// parseParamValue parses and returns a value from its string representation
// based on the given type/format info.
func parseParamValue(ctx Context, location string, name string, typ reflect.Type, timeFormat string, pstr string) interface{} {
	var pv interface{}
	switch typ.Kind() {
	case reflect.Bool:
		converted, err := strconv.ParseBool(pstr)
		if err != nil {
			ctx.AddError(&ErrorDetail{
				Message:  "cannot parse boolean",
				Location: location + "." + name,
				Value:    pstr,
			})
			return nil
		}
		pv = converted
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		converted, err := strconv.Atoi(pstr)
		if err != nil {
			ctx.AddError(&ErrorDetail{
				Message:  "cannot parse integer",
				Location: location + "." + name,
				Value:    pstr,
			})
			return nil
		}
		pv = reflect.ValueOf(converted).Convert(typ).Interface()
	case reflect.Float32:
		converted, err := strconv.ParseFloat(pstr, 32)
		if err != nil {
			ctx.AddError(&ErrorDetail{
				Message:  "cannot parse float",
				Location: location + "." + name,
				Value:    pstr,
			})
			return nil
		}
		pv = float32(converted)
	case reflect.Float64:
		converted, err := strconv.ParseFloat(pstr, 64)
		if err != nil {
			ctx.AddError(&ErrorDetail{
				Message:  "cannot parse float",
				Location: location + "." + name,
				Value:    pstr,
			})
			return nil
		}
		pv = converted
	case reflect.Slice:
		if len(pstr) > 1 && pstr[0] == '[' {
			pstr = pstr[1 : len(pstr)-1]
		}
		slice := reflect.MakeSlice(typ, 0, 0)
		for i, item := range strings.Split(pstr, ",") {
			if itemValue := parseParamValue(ctx, fmt.Sprintf("%s[%d]", location, i), name, typ.Elem(), timeFormat, item); itemValue != nil {
				slice = reflect.Append(slice, reflect.ValueOf(itemValue))
			} else {
				// Keep going to check other array items for vailidity.
				continue
			}
		}
		pv = slice.Interface()
	default:
		if typ == timeType {
			dt, err := time.Parse(timeFormat, pstr)
			if err != nil {
				ctx.AddError(&ErrorDetail{
					Message:  "cannot parse time",
					Location: location + "." + name,
					Value:    pstr,
				})
				return nil
			}
			pv = dt
		} else {
			pv = pstr
		}
	}

	return pv
}

func setFields(ctx *hcontext, req *http.Request, input reflect.Value, t reflect.Type) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if input.Kind() == reflect.Ptr {
		input = input.Elem()
	}

	if t.Kind() != reflect.Struct {
		panic("not a struct")
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		inField := input.Field(i)

		if f.Anonymous {
			// Embedded struct
			setFields(ctx, req, inField, f.Type)
			continue
		}

		if _, ok := f.Tag.Lookup("body"); ok || f.Name == "Body" {
			// Special case: body field is a reader for streaming
			if f.Type == readerType {
				inField.Set(reflect.ValueOf(req.Body))
				continue
			}

			// Check if a content-length has been sent. If it's too big then there
			// is no need to waste time reading.
			if length := req.Header.Get("Content-Length"); length != "" {
				if l, err := strconv.ParseInt(length, 10, 64); err == nil {
					if l > ctx.op.maxBodyBytes {
						ctx.AddError(&ErrorDetail{
							Message:  fmt.Sprintf("Request body too large, limit = %d bytes", ctx.op.maxBodyBytes),
							Location: "body",
							Value:    length,
						})
						continue
					}
				}
			}

			// Load the body (read/unmarshal).
			data, err := ioutil.ReadAll(req.Body)
			if err != nil {
				if strings.Contains(err.Error(), "request body too large") {
					ctx.AddError(&ErrorDetail{
						Message:  fmt.Sprintf("Request body too large, limit = %d bytes", ctx.op.maxBodyBytes),
						Location: "body",
					})
				} else if e, ok := err.(net.Error); ok && e.Timeout() {
					ctx.AddError(&ErrorDetail{
						Message:  fmt.Sprintf("Request body took too long to read: timed out after %v", ctx.op.bodyReadTimeout),
						Location: "body",
					})
				} else {
					panic(err)
				}
				continue
			}

			if ctx.op.requestSchema != nil && ctx.op.requestSchema.HasValidation() {
				if !validAgainstSchema(ctx, "body.", ctx.op.requestSchema, data) {
					continue
				}
			}

			err = json.Unmarshal(data, inField.Addr().Interface())
			if err != nil {
				panic(err)
			}
			continue
		}

		var pv string
		var pname string
		var location string
		timeFormat := time.RFC3339Nano
		if v, ok := f.Tag.Lookup("default"); ok {
			pv = v
		}

		if name, ok := f.Tag.Lookup("path"); ok {
			pname = name
			location = "path"
			if v := chi.URLParam(req, name); v != "" {
				pv = v
			}
		}

		if name, ok := f.Tag.Lookup("query"); ok {
			pname = name
			location = "query"
			if v := req.URL.Query().Get(name); v != "" {
				pv = v
			}
		}

		if name, ok := f.Tag.Lookup("header"); ok {
			pname = name
			location = "header"
			// TODO: get combined rather than first header?
			if v := req.Header.Get(name); v != "" {
				pv = v
			}

			// Some headers have special time formats that aren't ISO8601/RFC3339.
			lowerName := strings.ToLower(name)
			if lowerName == "if-modified-since" || lowerName == "if-unmodified-since" {
				timeFormat = http.TimeFormat
			}
		}

		if pv != "" {
			// Parse value into the right type.
			parsed := parseParamValue(ctx, location, pname, f.Type, timeFormat, pv)
			if parsed == nil {
				// At least one error, just keep going trying to parse other fields.
				continue
			}

			if oap, ok := ctx.op.params[pname]; ok {
				s := oap.Schema
				if s.HasValidation() {
					data := pv
					if s.Type == "string" {
						// Strings are special in that we don't expect users to provide them
						// with quotes, so wrap them here for the parser that does the
						// validation step below.
						data = `"` + data + `"`
					} else if s.Type == "array" {
						// Array type needs to have `[` and `]` added.
						if s.Items.Type == "string" {
							// Same as above, quote each item.
							data = `"` + strings.Join(strings.Split(data, ","), `","`) + `"`
						}
						if len(data) > 0 && data[0] != '[' {
							data = "[" + data + "]"
						}
					}

					if !validAgainstSchema(ctx, location+"."+pname, s, []byte(data)) {
						continue
					}
				}
			}

			inField.Set(reflect.ValueOf(parsed))
		}
	}

	// Resolve after all other fields are set so the resolver can use them,
	// and also so that any embedded structs are resolved first.
	if input.CanInterface() {
		if resolver, ok := input.Addr().Interface().(Resolver); ok {
			resolver.Resolve(ctx, req)
		}
	}
}

// getParamInfo recursively gets info about params from an input struct. It
// returns a map of parameter name => parameter object.
func getParamInfo(t reflect.Type) map[string]oaParam {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		panic("not a struct")
	}

	params := map[string]oaParam{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		if f.Anonymous {
			// Embedded struct
			for k, v := range getParamInfo(f.Type) {
				params[k] = v
			}
			continue
		}

		p := oaParam{}

		if name, ok := f.Tag.Lookup("path"); ok {
			p.Name = name
			p.In = inPath
			p.Required = true
		}

		if name, ok := f.Tag.Lookup("query"); ok {
			p.Name = name
			p.In = inQuery
			p.Explode = new(bool)
		}

		if name, ok := f.Tag.Lookup("header"); ok {
			p.Name = name
			p.In = inHeader
		}

		if p.Name == "" {
			// This is not a known param. May be filled in later by a resolver so
			// we shouldn't touch it. Skip!
			continue
		}

		if doc, ok := f.Tag.Lookup("doc"); ok {
			p.Description = doc
		}

		if deprecated, ok := f.Tag.Lookup("deprecated"); ok {
			p.Deprecated = deprecated == "true"
		}

		if internal, ok := f.Tag.Lookup("internal"); ok {
			p.Internal = internal == "true"
		}

		_, _, s, err := schema.GenerateFromField(f, schema.ModeRead)
		if err != nil {
			panic(err)
		}
		p.Schema = s

		params[p.Name] = p
	}

	return params
}
