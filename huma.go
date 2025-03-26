// Package huma provides a framework for building REST APIs in Go. It is
// designed to be simple, fast, and easy to use. It is also designed to
// generate OpenAPI 3.1 specifications and JSON Schema documents
// describing the API and providing a quick & easy way to generate
// docs, mocks, SDKs, CLI clients, and more.
//
// https://huma.rocks/
package huma

import (
	"bytes"
	"context"
	"encoding"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2/casing"
)

var errDeadlineUnsupported = fmt.Errorf("%w", http.ErrNotSupported)

var bodyCallbackType = reflect.TypeOf(func(Context) {})
var cookieType = reflect.TypeOf((*http.Cookie)(nil)).Elem()
var fmtStringerType = reflect.TypeOf((*fmt.Stringer)(nil)).Elem()
var stringType = reflect.TypeOf("")

// SetReadDeadline is a utility to set the read deadline on a response writer,
// if possible. If not, it will not incur any allocations (unlike the stdlib
// `http.ResponseController`). This is mostly a convenience function for
// adapters so they can be more efficient.
//
//	huma.SetReadDeadline(w, time.Now().Add(5*time.Second))
func SetReadDeadline(w http.ResponseWriter, deadline time.Time) error {
	for {
		switch t := w.(type) {
		case interface{ SetReadDeadline(time.Time) error }:
			return t.SetReadDeadline(deadline)
		case interface{ Unwrap() http.ResponseWriter }:
			w = t.Unwrap()
		default:
			return errDeadlineUnsupported
		}
	}
}

// StreamResponse is a response that streams data to the client. The body
// function will be called once the response headers have been written and
// the body writer is ready to be written to.
//
//	func handler(ctx context.Context, input *struct{}) (*huma.StreamResponse, error) {
//		return &huma.StreamResponse{
//			Body: func(ctx huma.Context) {
//				ctx.SetHeader("Content-Type", "text/my-type")
//
//				// Write some data to the stream.
//				writer := ctx.BodyWriter()
//				writer.Write([]byte("Hello "))
//
//				// Flush the stream to the client.
//				if f, ok := writer.(http.Flusher); ok {
//					f.Flush()
//				}
//
//				// Write some more...
//				writer.Write([]byte("world!"))
//			}
//		}
//	}
type StreamResponse struct {
	Body func(ctx Context)
}

const styleDeepObject = "deepObject"

type paramFieldInfo struct {
	Type       reflect.Type
	Name       string
	Loc        string
	Required   bool
	Default    string
	TimeFormat string
	Explode    bool
	Style      string
	Schema     *Schema
}

func findParams(registry Registry, op *Operation, t reflect.Type) *findResult[*paramFieldInfo] {
	return findInType(t, nil, func(f reflect.StructField, path []int) *paramFieldInfo {
		if f.Anonymous {
			return nil
		}
		pfi := &paramFieldInfo{
			Type: f.Type,
		}

		if reflect.PointerTo(f.Type).Implements(reflect.TypeFor[ParamWrapper]()) {
			pfi.Type = reflect.New(f.Type).Interface().(ParamWrapper).Receiver().Type()
		}

		if def := f.Tag.Get("default"); def != "" {
			pfi.Default = def
		}

		var name string
		var explode *bool
		if p := f.Tag.Get("path"); p != "" {
			pfi.Loc = "path"
			name = p
			pfi.Required = true
		} else if q := f.Tag.Get("query"); q != "" {
			pfi.Loc = "query"
			split := strings.Split(q, ",")
			name = split[0]
			// If `in` is `query` then `explode` defaults to true. Parsing is *much*
			// easier if we use comma-separated values, so we disable explode by default.
			if slices.Contains(split[1:], "explode") {
				pfi.Explode = true
			}
			if slices.Contains(split[1:], styleDeepObject) {
				pfi.Style = styleDeepObject
			}
			explode = &pfi.Explode
		} else if h := f.Tag.Get("header"); h != "" {
			pfi.Loc = "header"
			name = h
		} else if fo := f.Tag.Get("form"); fo != "" {
			pfi.Loc = "form"
			name = fo
		} else if c := f.Tag.Get("cookie"); c != "" {
			pfi.Loc = "cookie"
			name = c

			if f.Type == cookieType {
				// Special case: this will be parsed from a string input to a
				// `http.Cookie` struct.
				f.Type = stringType
			}
		} else {
			return nil
		}

		if f.Type.Kind() == reflect.Pointer {
			// TODO: support pointers? The problem is that when we dynamically
			// create an instance of the input struct the `params.Every(...)`
			// call cannot set them as the value is `reflect.Invalid` unless
			// dynamically allocated, but we don't know when to allocate until
			// after the `Every` callback has run. Doable, but a bigger change.
			panic("pointers are not supported for path/query/header parameters")
		}

		pfi.Schema = SchemaFromField(registry, f, "")

		var example any
		if e := f.Tag.Get("example"); e != "" {
			example = jsonTagValue(registry, f.Type.Name(), pfi.Schema, f.Tag.Get("example"))
		}
		if example == nil && len(pfi.Schema.Examples) > 0 {
			example = pfi.Schema.Examples[0]
		}

		// While discouraged, make it possible to make query/header params required.
		if r := f.Tag.Get("required"); r == "true" {
			pfi.Required = true
		}

		pfi.Name = name

		if f.Type == timeType {
			timeFormat := time.RFC3339Nano
			if pfi.Loc == "header" {
				timeFormat = http.TimeFormat
			}
			if f := f.Tag.Get("timeFormat"); f != "" {
				timeFormat = f
			}
			pfi.TimeFormat = timeFormat
		}

		if !boolTag(f, "hidden", false) {
			desc := ""
			if pfi.Schema != nil {
				// If the schema has a description, use it. Some tools will not show
				// the description if it is only on the schema.
				desc = pfi.Schema.Description
			}

			// Document the parameter if not hidden.
			op.Parameters = append(op.Parameters, &Param{
				Name:        name,
				Description: desc,
				In:          pfi.Loc,
				Explode:     explode,
				Required:    pfi.Required,
				Schema:      pfi.Schema,
				Example:     example,
				Style:       pfi.Style,
			})
		}

		return pfi
	}, false, "Body")
}

func findResolvers(resolverType, t reflect.Type) *findResult[bool] {
	return findInType(t, func(t reflect.Type, path []int) bool {
		tp := reflect.PointerTo(t)
		if tp.Implements(resolverType) || tp.Implements(resolverWithPathType) {
			return true
		}
		return false
	}, nil, true)
}

func findDefaults(registry Registry, t reflect.Type) *findResult[any] {
	return findInType(t, nil, func(sf reflect.StructField, i []int) any {
		if d := sf.Tag.Get("default"); d != "" {
			if sf.Type.Kind() == reflect.Pointer && sf.Type.Elem().Kind() == reflect.Struct {
				panic("pointers to structs cannot have default values")
			}
			s := registry.Schema(sf.Type, true, "")
			return convertType(sf.Type.Name(), sf.Type, jsonTagValue(registry, sf.Name, s, d))
		}
		return nil
	}, true)
}

type headerInfo struct {
	Field      reflect.StructField
	Name       string
	TimeFormat string
	ShouldSkip bool
}

func findHeaders(t reflect.Type) *findResult[*headerInfo] {
	return findInType(t, func(r reflect.Type, ints []int) *headerInfo {
		if r.Kind() == reflect.Slice && deref(r.Elem()) == cookieType {
			return &headerInfo{ShouldSkip: true}
		}
		return nil
	}, func(sf reflect.StructField, i []int) *headerInfo {
		// Ignore embedded fields
		if sf.Anonymous {
			return nil
		}

		header := sf.Tag.Get("header")
		if header == "" {
			header = sf.Name
		}
		timeFormat := ""
		if sf.Type == timeType {
			timeFormat = http.TimeFormat
			if f := sf.Tag.Get("timeFormat"); f != "" {
				timeFormat = f
			}
		}
		return &headerInfo{sf, header, timeFormat, false}
	}, false, "Status", "Body")
}

type findResultPath[T comparable] struct {
	Path  []int
	Value T
}

type findResult[T comparable] struct {
	Paths []findResultPath[T]
}

func (r *findResult[T]) every(current reflect.Value, path []int, v T, f func(reflect.Value, T)) {
	if len(path) == 0 {
		f(current, v)
		return
	}

	current = reflect.Indirect(current)
	if current.Kind() == reflect.Invalid {
		// Indirect may have resulted in no value, for example an optional field
		// that's a pointer may have been omitted; just ignore it.
		return
	}

	switch current.Kind() {
	case reflect.Struct:
		r.every(current.Field(path[0]), path[1:], v, f)
	case reflect.Slice:
		for j := 0; j < current.Len(); j++ {
			r.every(current.Index(j), path, v, f)
		}
	case reflect.Map:
		for _, k := range current.MapKeys() {
			r.every(current.MapIndex(k), path, v, f)
		}
	default:
		panic("unsupported")
	}
}

func (r *findResult[T]) Every(v reflect.Value, f func(reflect.Value, T)) {
	for i := range r.Paths {
		r.every(v, r.Paths[i].Path, r.Paths[i].Value, f)
	}
}

func jsonName(field reflect.StructField) string {
	name := strings.ToLower(field.Name)
	if jsonName := field.Tag.Get("json"); jsonName != "" {
		name = strings.Split(jsonName, ",")[0]
	}
	return name
}

func (r *findResult[T]) everyPB(current reflect.Value, path []int, pb *PathBuffer, v T, f func(reflect.Value, T)) {
	switch reflect.Indirect(current).Kind() {
	case reflect.Slice, reflect.Map:
		// Ignore these. We only care about the leaf nodes.
	default:
		if len(path) == 0 {
			f(current, v)
			return
		}
	}

	current = reflect.Indirect(current)
	if current.Kind() == reflect.Invalid {
		// Indirect may have resulted in no value, for example an optional field may
		// have been omitted; just ignore it.
		return
	}

	switch current.Kind() {
	case reflect.Struct:
		field := current.Type().Field(path[0])
		pops := 0
		if !field.Anonymous {
			// The path name can come from one of four places: path parameter,
			// query parameter, header parameter, or body field.
			// TODO: pre-compute type/field names? Could save a few allocations.
			pops++
			if path := field.Tag.Get("path"); path != "" && pb.Len() == 0 {
				pb.Push("path")
				pb.Push(path)
				pops++
			} else if query := field.Tag.Get("query"); query != "" && pb.Len() == 0 {
				pb.Push("query")
				pb.Push(query)
				pops++
			} else if header := field.Tag.Get("header"); header != "" && pb.Len() == 0 {
				pb.Push("header")
				pb.Push(header)
				pops++
			} else {
				// The body is _always_ in a field called "Body", which turns into
				// `body` in the path buffer, so we don't need to push it separately
				// like the params fields above.
				pb.Push(jsonName(field))
			}
		}
		r.everyPB(current.Field(path[0]), path[1:], pb, v, f)
		for i := 0; i < pops; i++ {
			pb.Pop()
		}
	case reflect.Slice:
		for j := 0; j < current.Len(); j++ {
			pb.PushIndex(j)
			r.everyPB(current.Index(j), path, pb, v, f)
			pb.Pop()
		}
	case reflect.Map:
		for _, k := range current.MapKeys() {
			if k.Kind() == reflect.String {
				pb.Push(k.String())
			} else {
				pb.Push(fmt.Sprintf("%v", k.Interface()))
			}
			r.everyPB(current.MapIndex(k), path, pb, v, f)
			pb.Pop()
		}
	default:
		panic("unsupported")
	}
}

func (r *findResult[T]) EveryPB(pb *PathBuffer, v reflect.Value, f func(reflect.Value, T)) {
	for i := range r.Paths {
		pb.Reset()
		r.everyPB(v, r.Paths[i].Path, pb, r.Paths[i].Value, f)
	}
}

func findInType[T comparable](t reflect.Type, onType func(reflect.Type, []int) T, onField func(reflect.StructField, []int) T, recurseFields bool, ignore ...string) *findResult[T] {
	result := &findResult[T]{}
	_findInType(t, []int{}, result, onType, onField, recurseFields, make(map[reflect.Type]struct{}), ignore...)
	return result
}

func _findInType[T comparable](t reflect.Type, path []int, result *findResult[T], onType func(reflect.Type, []int) T, onField func(reflect.StructField, []int) T, recurseFields bool, visited map[reflect.Type]struct{}, ignore ...string) {
	t = deref(t)
	zero := reflect.Zero(reflect.TypeOf((*T)(nil)).Elem()).Interface()

	resolved := false
	if onType != nil {
		if v := onType(t, path); v != zero {
			result.Paths = append(result.Paths, findResultPath[T]{path, v})
			resolved = true
		}
	}

	switch t.Kind() {
	case reflect.Struct:
		if _, ok := visited[t]; ok {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if slices.Contains(ignore, f.Name) {
				continue
			}
			// We do still want to potentially process each non-anonymous field,
			// so only skip anonymous ones.
			if resolved && f.Anonymous {
				continue
			}
			fi := append([]int{}, path...)
			fi = append(fi, i)
			if onField != nil {
				if v := onField(f, fi); v != zero {
					result.Paths = append(result.Paths, findResultPath[T]{fi, v})
				}
			}

			if f.Anonymous || recurseFields || deref(f.Type).Kind() != reflect.Struct {
				visited[t] = struct{}{}
				_findInType(f.Type, fi, result, onType, onField, recurseFields, visited, ignore...)
				delete(visited, t)
			}
		}
	case reflect.Slice:
		if resolved {
			return
		}
		_findInType(t.Elem(), path, result, onType, onField, recurseFields, visited, ignore...)
	case reflect.Map:
		if resolved {
			return
		}
		_findInType(t.Elem(), path, result, onType, onField, recurseFields, visited, ignore...)
	}
}

func getHint(parent reflect.Type, name string, other string) string {
	if parent.Name() != "" {
		return parent.Name() + name
	} else {
		return other
	}
}

type validateDeps struct {
	pb  *PathBuffer
	res *ValidateResult
}

var validatePool = sync.Pool{
	New: func() any {
		return &validateDeps{
			pb:  &PathBuffer{buf: make([]byte, 0, 128)},
			res: &ValidateResult{},
		}
	},
}

var bufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 128))
	},
}

func writeResponse(api API, ctx Context, status int, ct string, body any) error {
	if ct == "" {
		// If no content type was provided, try to negotiate one with the client.
		var err error
		ct, err = api.Negotiate(ctx.Header("Accept"))
		if err != nil {
			notAccept := NewErrorWithContext(ctx, http.StatusNotAcceptable, "unable to marshal response", err)
			if e := transformAndWrite(api, ctx, http.StatusNotAcceptable, "application/json", notAccept); e != nil {
				return e
			}
			return err
		}

		if ctf, ok := body.(ContentTypeFilter); ok {
			ct = ctf.ContentType(ct)
		}

		ctx.SetHeader("Content-Type", ct)
	}

	if err := transformAndWrite(api, ctx, status, ct, body); err != nil {
		return err
	}
	return nil
}

func writeResponseWithPanic(api API, ctx Context, status int, ct string, body any) {
	if err := writeResponse(api, ctx, status, ct, body); err != nil {
		panic(err)
	}
}

// transformAndWrite is a utility function to transform and write a response.
// It is best-effort as the status code and headers may have already been sent.
func transformAndWrite(api API, ctx Context, status int, ct string, body any) error {
	// Try to transform and then marshal/write the response.
	// Status code was already sent, so just log the error if something fails,
	// and do our best to stuff it into the body of the response.
	tval, terr := api.Transform(ctx, strconv.Itoa(status), body)
	if terr != nil {
		ctx.BodyWriter().Write([]byte("error transforming response"))
		// When including tval in the panic message, the server may become unresponsive for some time if the value is very large
		// therefore, it has been removed from the panic message
		return fmt.Errorf("error transforming response for %s %s %d: %w", ctx.Operation().Method, ctx.Operation().Path, status, terr)
	}
	ctx.SetStatus(status)
	if status != http.StatusNoContent && status != http.StatusNotModified {
		if merr := api.Marshal(ctx.BodyWriter(), ct, tval); merr != nil {
			if errors.Is(ctx.Context().Err(), context.Canceled) {
				// The client disconnected, so don't bother writing anything. Attempt
				// to set the status in case it'll get logged. Technically this was
				// not a normal successful request.
				ctx.SetStatus(499)
				return nil
			}
			ctx.BodyWriter().Write([]byte("error marshaling response"))
			// When including tval in the panic message, the server may become unresponsive for some time if the value is very large
			// therefore, it has been removed from the panic message
			return fmt.Errorf("error marshaling response for %s %s %d: %w", ctx.Operation().Method, ctx.Operation().Path, status, merr)
		}
	}
	return nil
}

func parseArrElement[T any](values []string, parse func(string) (T, error)) ([]T, error) {
	result := make([]T, 0, len(values))

	for i := 0; i < len(values); i++ {
		v, err := parse(values[i])
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}

	return result, nil
}

// writeHeader is a utility function to write a header value to the response.
// the `write` function should be either `ctx.SetHeader` or `ctx.AppendHeader`.
func writeHeader(write func(string, string), info *headerInfo, f reflect.Value) {
	switch f.Kind() {
	case reflect.String:
		if f.String() == "" {
			// Don't set empty headers.
			return
		}
		write(info.Name, f.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		write(info.Name, strconv.FormatInt(f.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		write(info.Name, strconv.FormatUint(f.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		write(info.Name, strconv.FormatFloat(f.Float(), 'f', -1, 64))
	case reflect.Bool:
		write(info.Name, strconv.FormatBool(f.Bool()))
	default:
		if f.Type() == timeType && !f.Interface().(time.Time).IsZero() {
			write(info.Name, f.Interface().(time.Time).Format(info.TimeFormat))
			return
		}

		// If the field value has a `String() string` method, use it.
		if f.CanAddr() {
			if s, ok := f.Addr().Interface().(fmt.Stringer); ok {
				write(info.Name, s.String())
				return
			}
		}

		write(info.Name, fmt.Sprintf("%v", f.Interface()))
	}
}

// Register an operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header/cookie parameters and/or body. The output
// struct must be a struct with fields for the output headers and body of the
// operation, if any.
//
//	huma.Register(api, huma.Operation{
//		OperationID: "get-greeting",
//		Method:      http.MethodGet,
//		Path:        "/greeting/{name}",
//		Summary:     "Get a greeting",
//	}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
//		if input.Name == "bob" {
//			return nil, huma.Error404NotFound("no greeting for bob")
//		}
//		resp := &GreetingOutput{}
//		resp.MyHeader = "MyValue"
//		resp.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)
//		return resp, nil
//	})
func Register[I, O any](api API, op Operation, handler func(context.Context, *I) (*O, error)) {
	oapi := api.OpenAPI()
	registry := oapi.Components.Schemas

	if op.Method == "" || op.Path == "" {
		panic("method and path must be specified in operation")
	}
	initResponses(&op)

	inputType := reflect.TypeOf((*I)(nil)).Elem()
	if inputType.Kind() != reflect.Struct {
		panic("input must be a struct")
	}
	inputParams, inputBodyIndex, hasInputBody, rawBodyIndex, rbt, inSchema := processInputType(inputType, &op, registry)

	outputType := reflect.TypeOf((*O)(nil)).Elem()
	if outputType.Kind() != reflect.Struct {
		panic("output must be a struct")
	}
	outHeaders, outStatusIndex, outBodyIndex, outBodyFunc := processOutputType(outputType, &op, registry)

	if len(op.Errors) > 0 {
		if len(inputParams.Paths) > 0 || hasInputBody {
			op.Errors = append(op.Errors, http.StatusUnprocessableEntity)
		}
		op.Errors = append(op.Errors, http.StatusInternalServerError)
	}
	defineErrors(&op, registry)

	if documenter, ok := api.(OperationDocumenter); ok {
		// Enables customization of OpenAPI documentation behavior for operations.
		documenter.DocumentOperation(&op)
	} else {
		if !op.Hidden {
			oapi.AddOperation(&op)
		}
	}

	resolvers := findResolvers(resolverType, inputType)
	defaults := findDefaults(registry, inputType)
	a := api.Adapter()
	a.Handle(&op, api.Middlewares().Handler(op.Middlewares.Handler(func(ctx Context) {
		var input I

		// Get the validation dependencies from the shared pool.
		deps := validatePool.Get().(*validateDeps)
		defer func() {
			deps.pb.Reset()
			deps.res.Reset()
			validatePool.Put(deps)
		}()
		pb := deps.pb
		res := deps.res

		errStatus := http.StatusUnprocessableEntity

		var cookies map[string]*http.Cookie

		v := reflect.ValueOf(&input).Elem()
		inputParams.Every(v, func(f reflect.Value, p *paramFieldInfo) {
			f = reflect.Indirect(f)
			if f.Kind() == reflect.Invalid {
				return
			}

			pb.Reset()
			pb.Push(p.Loc)
			pb.Push(p.Name)

			if p.Loc == "cookie" {
				if cookies == nil {
					// Only parse the cookie headers once, on-demand.
					cookies = map[string]*http.Cookie{}
					for _, c := range ReadCookies(ctx) {
						cookies[c.Name] = c
					}
				}
				if c, ok := cookies[p.Name]; ok && f.Type() == cookieType {
					// Special case: http.Cookie type, meaning we want the entire parsed
					// cookie struct, not just the value.
					f.Set(reflect.ValueOf(c).Elem())
					return
				}
			}

			var receiver = f
			if f.Addr().Type().Implements(reflect.TypeFor[ParamWrapper]()) {
				receiver = f.Addr().Interface().(ParamWrapper).Receiver()
			}

			var pv any
			var isSet bool
			if p.Loc == "query" && p.Style == styleDeepObject {
				// Deep object style is a special case where we need to parse the
				// query parameter into a struct. We do this by parsing the query
				// parameter into a map, then iterating over the map and setting
				// the fields on the struct.
				u := ctx.URL()
				value := parseDeepObjectQuery(u.Query(), p.Name)
				isSet = len(value) > 0
				if len(value) == 0 {
					if !op.SkipValidateParams && p.Required {
						res.Add(pb, "", "required "+p.Loc+" parameter is missing")
					}
					return
				}
				pv = setDeepObjectValue(pb, res, receiver, value)
			} else {
				value := getParamValue(*p, ctx, cookies)
				isSet = value != ""
				if value == "" {
					if !op.SkipValidateParams && p.Required {
						// Path params are always required.
						res.Add(pb, "", "required "+p.Loc+" parameter is missing")
					}
					return
				}
				var err error
				pv, err = parseInto(ctx, receiver, value, nil, *p)
				if err != nil {
					res.Add(pb, value, err.Error())
					return
				}
			}

			if f.Addr().Type().Implements(reflect.TypeFor[ParamReactor]()) {
				f.Addr().Interface().(ParamReactor).OnParamSet(isSet, pv)
			}

			if !op.SkipValidateParams {
				Validate(oapi.Components.Schemas, p.Schema, pb, ModeWriteToServer, pv, res)
			}
		})

		// Read input body if defined.
		if hasInputBody || len(rawBodyIndex) > 0 {
			if op.BodyReadTimeout > 0 {
				ctx.SetReadDeadline(time.Now().Add(op.BodyReadTimeout))
			} else if op.BodyReadTimeout < 0 {
				// Disable any server-wide deadline.
				ctx.SetReadDeadline(time.Time{})
			}

			if rbt.isMultipart() {
				// Read form
				form, err := readForm(ctx)

				if err != nil {
					res.Errors = append(res.Errors, err)
				} else {
					var formValueParser func(val reflect.Value)
					if rbt == rbtMultipart {
						formValueParser = func(val reflect.Value) {}
					} else {
						rawBodyF := v.FieldByIndex(rawBodyIndex)
						rawBodyDataF := rawBodyF.FieldByName("data")
						rawBodyDataT := rawBodyDataF.Type()

						rawBodyInputParams := findParams(oapi.Components.Schemas, &op, rawBodyDataT)
						formValueParser = func(val reflect.Value) {
							rawBodyInputParams.Every(val, func(f reflect.Value, p *paramFieldInfo) {
								f = reflect.Indirect(f)
								if f.Kind() == reflect.Invalid {
									return
								}

								pb.Reset()
								pb.Push(p.Loc)
								pb.Push(p.Name)

								value, ok := form.Value[p.Name]
								if !ok {
									_, isFile := form.File[p.Name]
									if !op.SkipValidateParams && p.Required && !isFile {
										res.Add(pb, "", "required "+p.Loc+" parameter is missing")
									}
									return
								}

								// Validation should fail if multiple values are
								// provided but the type of f is not a slice.
								if len(value) > 1 && f.Type().Kind() != reflect.Slice {
									res.Add(pb, value, "expected at most one value, but received multiple values")
									return
								}
								pv, err := parseInto(ctx, f, value[0], value, *p)
								if err != nil {
									res.Add(pb, value, err.Error())
								}

								if !op.SkipValidateParams {
									Validate(oapi.Components.Schemas, p.Schema, pb, ModeWriteToServer, pv, res)
								}
							})
						}
					}

					if cErr := processMultipartMsgBody(form, op, v, rbt, rawBodyIndex, formValueParser); cErr != nil {
						writeErr(api, ctx, cErr, *res)
						return
					}
				}
			} else {
				// Read body
				buf := bufPool.Get().(*bytes.Buffer)
				bufCloser := func() {
					buf.Reset()
					bufPool.Put(buf)
				}
				if cErr := readBody(buf, ctx, op.MaxBodyBytes); cErr != nil {
					bufCloser()
					writeErr(api, ctx, cErr, *res)
					return
				}
				body := buf.Bytes()

				// Store raw body
				if len(rawBodyIndex) > 0 {
					f := v.FieldByIndex(rawBodyIndex)
					f.SetBytes(body)
				}

				// Process body
				unmarshaler := func(data []byte, v any) error { return api.Unmarshal(ctx.Header("Content-Type"), data, v) }
				validator := func(data any, res *ValidateResult) {
					pb.Reset()
					pb.Push("body")
					Validate(oapi.Components.Schemas, inSchema, pb, ModeWriteToServer, data, res)
				}
				processErrStatus, cErr := processRegularMsgBody(body, op, v, hasInputBody, inputBodyIndex, unmarshaler, validator, defaults, res)
				if processErrStatus > 0 {
					errStatus = processErrStatus
				}
				if cErr != nil {
					bufCloser()
					writeErr(api, ctx, cErr, *res)
					return
				}

				// Clean up
				// If the raw body is used, then we must wait until *AFTER* the
				// handler has run to return the body byte buffer to the pool, as
				// the handler can read and modify this buffer. The safest way is
				// to just wait until the end of this handler via defer.
				if len(rawBodyIndex) > 0 {
					defer bufCloser()
				} else {
					bufCloser()
				}
			}
		}

		resolvers.EveryPB(pb, v, func(item reflect.Value, _ bool) {
			item = reflect.Indirect(item)
			if item.Kind() == reflect.Invalid {
				return
			}
			if item.CanAddr() {
				item = item.Addr()
			} else {
				// If the item is non-addressable (example: primitive custom type with
				// a resolver as a map value), then we need to create a new pointer to
				// the value to ensure the resolver can be called, regardless of whether
				// is a value or pointer resolver type.
				// TODO: this is inefficient and could be improved in the future.
				ptr := reflect.New(item.Type())
				elem := ptr.Elem()
				elem.Set(item)
				item = ptr
			}
			var errs []error
			switch resolver := item.Interface().(type) {
			case Resolver:
				errs = resolver.Resolve(ctx)
			case ResolverWithPath:
				errs = resolver.Resolve(ctx, pb)
			default:
				panic("matched resolver cannot be run, please file a bug")
			}
			if len(errs) > 0 {
				res.Errors = append(res.Errors, errs...)
			}
		})

		if len(res.Errors) > 0 {
			for i := len(res.Errors) - 1; i >= 0; i-- {
				// If there are errors, and they provide a status, then update the
				// response status code to match. Otherwise, use the default status
				// code is used. Since these run in order, the last error code wins.
				if s, ok := res.Errors[i].(StatusError); ok {
					errStatus = s.GetStatus()
					break
				}
			}
			WriteErr(api, ctx, errStatus, "validation failed", res.Errors...)
			return
		}

		output, err := handler(ctx.Context(), &input)
		if err != nil {
			var he HeadersError
			if errors.As(err, &he) {
				for k, values := range he.GetHeaders() {
					for _, v := range values {
						ctx.AppendHeader(k, v)
					}
				}
			}

			status := http.StatusInternalServerError

			// handle status error
			var se StatusError
			if errors.As(err, &se) {
				writeResponseWithPanic(api, ctx, se.GetStatus(), "", se)
				return
			}

			se = NewErrorWithContext(ctx, status, "unexpected error occurred", err)
			writeResponseWithPanic(api, ctx, se.GetStatus(), "", se)
			return
		}

		if output == nil {
			// Special case: No err or output, so just set the status code and return.
			// This is a weird case, but it's better than panicking or returning 500.
			ctx.SetStatus(op.DefaultStatus)
			return
		}

		// Serialize output headers
		ct := ""
		vo := reflect.ValueOf(output).Elem()
		outHeaders.Every(vo, func(f reflect.Value, info *headerInfo) {
			f = reflect.Indirect(f)
			if f.Kind() == reflect.Invalid {
				return
			}
			if f.Kind() == reflect.Slice {
				for i := 0; i < f.Len(); i++ {
					writeHeader(ctx.AppendHeader, info, f.Index(i))
				}
			} else {
				if f.Kind() == reflect.String && info.Name == "Content-Type" {
					// Track custom content type. This overrides any content negotiation
					// that would happen when writing the response.
					ct = f.String()
				}
				writeHeader(ctx.SetHeader, info, f)
			}
		})

		status := op.DefaultStatus
		if outStatusIndex != -1 {
			status = int(vo.Field(outStatusIndex).Int())
		}

		if outBodyIndex != -1 {
			// Serialize output body
			body := vo.Field(outBodyIndex).Interface()

			if outBodyFunc {
				body.(func(Context))(ctx)
				return
			}

			if b, ok := body.([]byte); ok {
				ctx.SetStatus(status)
				ctx.BodyWriter().Write(b)
				return
			}

			writeResponseWithPanic(api, ctx, status, ct, body)
		} else {
			ctx.SetStatus(status)
		}
	})))
}

func parseDeepObjectQuery(query url.Values, name string) map[string]string {
	result := make(map[string]string)

	for key, values := range query {
		if strings.Contains(key, "[") {
			// Nested object
			keys := strings.Split(key, "[")
			if keys[0] != name {
				continue
			}
			k := strings.Trim(keys[1], "]")
			result[k] = values[0]
		}
	}
	return result
}

func setDeepObjectValue(pb *PathBuffer, res *ValidateResult, f reflect.Value, data map[string]string) map[string]any {
	t := f.Type()
	result := make(map[string]any)
	switch t.Kind() {
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic("unsupported map key type")
		}
		f.Set(reflect.MakeMap(t))
		for k, v := range data {
			key := reflect.New(t.Key()).Elem()
			key.SetString(k)
			value := reflect.New(t.Elem()).Elem()
			if err := setFieldValue(value, v); err != nil {
				pb.Push(k)
				res.Add(pb, v, err.Error())
				pb.Pop()
			} else {
				f.SetMapIndex(key, value)
				result[k] = value.Interface()
			}
		}
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			// Get the field name
			fieldName := field.Name

			if name := jsonName(field); name != "" {
				fieldName = name
			}

			fv := f.Field(i)
			if val, ok := data[fieldName]; ok {
				if err := setFieldValue(fv, val); err != nil {
					pb.Push(fieldName)
					res.Add(pb, val, err.Error())
					pb.Pop()
				} else {
					result[fieldName] = fv.Interface()
				}
			} else {
				if val := field.Tag.Get("default"); val != "" {
					setFieldValue(fv, val)
					result[fieldName] = fv.Interface()
				}
			}
		}
	}
	return result
}

func setFieldValue(f reflect.Value, value string) error {
	switch f.Kind() {
	case reflect.String:
		f.SetString(value)
	case reflect.Interface:
		f.Set(reflect.ValueOf(value))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return errors.New("invalid integer")
		}
		f.SetInt(v)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return errors.New("invalid integer")
		}
		f.SetUint(v)
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return errors.New("invalid float")
		}
		f.SetFloat(v)
	case reflect.Bool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return errors.New("invalid boolean")
		}
		f.SetBool(v)
	default:
		return errors.New("unsupported type")
	}
	return nil
}

// ParamWrapper is an interface that can be implemented by a wrapping type
// to expose a field into which request parameters may be parsed.
// Must have pointer receiver.
// Example:
//
//	type OptionalParam[T any] struct {
//		Value T
//		IsSet bool
//	}
//	func (o *OptionalParam[T]) Receiver() reflect.Value {
//		return reflect.ValueOf(o).Elem().Field(0)
//	}
type ParamWrapper interface {
	Receiver() reflect.Value
}

// ParamReactor is an interface that can be implemented to react to request
// parameters being set on the field. Must have pointer receiver.
// Intended to be combined with ParamWrapper interface.
//
// First argument is a boolean indicating if the parameter was set in the request.
// Second argument is the parsed value from Huma.
//
// Example:
//
//	func (o *OptionalParam[T]) OnParamSet(isSet bool, parsed any) {
//		 o.IsSet = isSet
//	}
type ParamReactor interface {
	OnParamSet(isSet bool, parsed any)
}

// initResponses initializes Responses if it was unset.
func initResponses(op *Operation) {
	if op.Responses == nil {
		op.Responses = map[string]*Response{}
	}
}

// processInputType validates the input type, extracts expected requests and
// defines them on the operation op.
func processInputType(inputType reflect.Type, op *Operation, registry Registry) (*findResult[*paramFieldInfo], []int, bool, []int, rawBodyType, *Schema) {
	inputParams := findParams(registry, op, inputType)
	inputBodyIndex := []int{}
	hasInputBody := false
	if f, ok := inputType.FieldByName("Body"); ok {
		hasInputBody = true
		inputBodyIndex = f.Index
		initRequestBody(op)
		setRequestBodyFromBody(op, registry, f, inputType)
		ensureBodyReadTimeout(op)
		ensureMaxBodyBytes(op)
	}
	rawBodyIndex := []int{}
	var rbt rawBodyType
	if f, ok := inputType.FieldByName("RawBody"); ok {
		rawBodyIndex = f.Index
		initRequestBody(op, setRequestBodyRequired)
		rbt = setRequestBodyFromRawBody(op, registry, f)
	}

	if op.RequestBody != nil {
		for _, mediatype := range op.RequestBody.Content {
			if mediatype.Schema != nil {
				// Ensure all schema validation errors are set up properly as some
				// parts of the schema may have been user-supplied.
				mediatype.Schema.PrecomputeMessages()
			}
		}
	}

	var inSchema *Schema
	if op.RequestBody != nil && op.RequestBody.Content != nil && op.RequestBody.Content["application/json"] != nil && op.RequestBody.Content["application/json"].Schema != nil {
		hasInputBody = true
		inSchema = op.RequestBody.Content["application/json"].Schema
	}
	return inputParams, inputBodyIndex, hasInputBody, rawBodyIndex, rbt, inSchema
}

// ensureMaxBodyBytes sets the MaxBodyBytes to a default value if it was unset.
func ensureMaxBodyBytes(op *Operation) {
	if op.MaxBodyBytes == 0 {
		// 1 MB default
		op.MaxBodyBytes = 1024 * 1024
	}
}

// ensureBodyReadTimeout sets the BodyReadTimeout to a default value if it was unset.
func ensureBodyReadTimeout(op *Operation) {
	if op.BodyReadTimeout == 0 {
		// 5 second default
		op.BodyReadTimeout = 5 * time.Second
	}
}

// setRequestBodyFromBody configures op.RequestBody from the Body field.
func setRequestBodyFromBody(op *Operation, registry Registry, fBody reflect.StructField, inputType reflect.Type) {
	if fBody.Tag.Get("required") == "true" || (fBody.Type.Kind() != reflect.Ptr && fBody.Type.Kind() != reflect.Interface) {
		setRequestBodyRequired(op.RequestBody)
	}
	contentType := "application/json"
	if c := fBody.Tag.Get("contentType"); c != "" {
		contentType = c
	}
	if op.RequestBody.Content[contentType] == nil {
		op.RequestBody.Content[contentType] = &MediaType{}
	}
	if op.RequestBody.Content[contentType].Schema == nil {
		hint := getHint(inputType, fBody.Name, op.OperationID+"Request")
		if nameHint := fBody.Tag.Get("nameHint"); nameHint != "" {
			hint = nameHint
		}
		s := SchemaFromField(registry, fBody, hint)
		op.RequestBody.Content[contentType].Schema = s
	}
}

type rawBodyType int

const (
	rbtMultipart rawBodyType = iota + 1
	rbtMultipartDecoded
	rbtOther
)

func (r rawBodyType) isMultipart() bool {
	return r == rbtMultipart || r == rbtMultipartDecoded
}

// setRequestBodyFromRawBody configures op.RequestBody from the RawBody field.
func setRequestBodyFromRawBody(op *Operation, r Registry, fRawBody reflect.StructField) rawBodyType {
	rbt := rbtOther
	contentType := "application/octet-stream"
	if fRawBody.Type.String() == "multipart.Form" {
		contentType = "multipart/form-data"
		rbt = rbtMultipart
	}
	if strings.HasPrefix(fRawBody.Type.Name(), "MultipartFormFiles") {
		contentType = "multipart/form-data"
		rbt = rbtMultipartDecoded
	}
	if c := fRawBody.Tag.Get("contentType"); c != "" {
		contentType = c
	}

	if contentType != "multipart/form-data" {
		op.RequestBody.Content[contentType] = &MediaType{
			Schema: &Schema{
				Type:   "string",
				Format: "binary",
			},
		}
		return rbt
	}
	if op.RequestBody.Content["multipart/form-data"] != nil {
		return rbt
	}

	switch rbt {
	case rbtMultipart:
		op.RequestBody.Content["multipart/form-data"] = &MediaType{
			Schema: &Schema{
				Type: "object",
				Properties: map[string]*Schema{
					"name": {
						Type:        "string",
						Description: "general purpose name for multipart form value",
					},
					"filename": {
						Type:        "string",
						Format:      "binary",
						Description: "filename of the file being uploaded",
					},
				},
			},
		}
	case rbtMultipartDecoded:
		dataField, ok := fRawBody.Type.FieldByName("data")
		if !ok {
			panic("Expected type MultipartFormFiles[T] to have a 'data *T' generic pointer field")
		}
		op.RequestBody.Content["multipart/form-data"] = &MediaType{
			Schema:   multiPartFormFileSchema(r, dataField.Type.Elem()),
			Encoding: multiPartContentEncoding(dataField.Type.Elem()),
		}
		op.RequestBody.Required = false
	}
	return rbt
}

// initRequestBody initializes an empty RequestBody and its Content map.
func initRequestBody(op *Operation, rbOpts ...func(*RequestBody)) {
	if op.RequestBody == nil {
		op.RequestBody = &RequestBody{}
	}
	if op.RequestBody.Content == nil {
		op.RequestBody.Content = map[string]*MediaType{}
	}
	for _, opt := range rbOpts {
		opt(op.RequestBody)
	}
}

func setRequestBodyRequired(rb *RequestBody) {
	rb.Required = true
}

// processOutputType validates the output type, extracts possible responses and
// defines them on the operation op.
func processOutputType(outputType reflect.Type, op *Operation, registry Registry) (*findResult[*headerInfo], int, int, bool) {
	outStatusIndex := -1
	if f, ok := outputType.FieldByName("Status"); ok {
		outStatusIndex = f.Index[0]
		if f.Type.Kind() != reflect.Int {
			panic("status field must be an int")
		}
		// TODO: enum tag?
		// TODO: register each of the possible responses with the right model
		//       and headers down below.
	}
	outBodyIndex := -1
	outBodyFunc := false
	if f, ok := outputType.FieldByName("Body"); ok {
		outBodyIndex = f.Index[0]
		if f.Type.Kind() == reflect.Func {
			outBodyFunc = true

			if f.Type != bodyCallbackType {
				panic("body field must be a function with signature func(huma.Context)")
			}
		}
		status := op.DefaultStatus
		if status == 0 {
			status = http.StatusOK
		}
		statusStr := strconv.Itoa(status)
		if op.Responses[statusStr] == nil {
			op.Responses[statusStr] = &Response{}
		}
		if op.Responses[statusStr].Description == "" {
			op.Responses[statusStr].Description = http.StatusText(status)
		}
		if op.Responses[statusStr].Headers == nil {
			op.Responses[statusStr].Headers = map[string]*Param{}
		}
		if !outBodyFunc {
			hint := getHint(outputType, f.Name, op.OperationID+"Response")
			if nameHint := f.Tag.Get("nameHint"); nameHint != "" {
				hint = nameHint
			}
			outSchema := SchemaFromField(registry, f, hint)
			if op.Responses[statusStr].Content == nil {
				op.Responses[statusStr].Content = map[string]*MediaType{}
			}
			// Check if the field's type implements ContentTypeFilter
			contentType := "application/json"
			if reflect.PointerTo(f.Type).Implements(reflect.TypeFor[ContentTypeFilter]()) {
				instance := reflect.New(f.Type).Interface().(ContentTypeFilter)
				contentType = instance.ContentType(contentType)
			}
			if len(op.Responses[statusStr].Content) == 0 {
				op.Responses[statusStr].Content[contentType] = &MediaType{}
			}
			if op.Responses[statusStr].Content[contentType] != nil && op.Responses[statusStr].Content[contentType].Schema == nil {
				op.Responses[statusStr].Content[contentType].Schema = outSchema
			}
		}
	}
	if op.DefaultStatus == 0 {
		if outBodyIndex != -1 {
			op.DefaultStatus = http.StatusOK
		} else {
			op.DefaultStatus = http.StatusNoContent
		}
	}
	defaultStatusStr := strconv.Itoa(op.DefaultStatus)
	if op.Responses[defaultStatusStr] == nil {
		op.Responses[defaultStatusStr] = &Response{
			Description: http.StatusText(op.DefaultStatus),
		}
	}
	outHeaders := findHeaders(outputType)
	for _, entry := range outHeaders.Paths {
		if entry.Value.ShouldSkip {
			continue
		}

		// Document the header's name and type.
		if op.Responses[defaultStatusStr].Headers == nil {
			op.Responses[defaultStatusStr].Headers = map[string]*Param{}
		}
		v := entry.Value
		f := v.Field
		if f.Type.Kind() == reflect.Slice {
			f.Type = deref(f.Type.Elem())
		}
		if reflect.PointerTo(f.Type).Implements(fmtStringerType) {
			// Special case: this field will be written as a string by calling
			// `.String()` on the value.
			f.Type = stringType
		}
		op.Responses[defaultStatusStr].Headers[v.Name] = &Header{
			// We need to generate the schema from the field to get validation info
			// like min/max and enums. Useful to let the client know possible values.
			Schema: SchemaFromField(registry, f, getHint(outputType, f.Name, op.OperationID+defaultStatusStr+v.Name)),
		}
	}
	return outHeaders, outStatusIndex, outBodyIndex, outBodyFunc
}

// defineErrors extracts possible error responses and defines them on the
// operation op.
func defineErrors(op *Operation, registry Registry) {
	exampleErr := NewError(0, "")
	errContentType := "application/json"
	if ctf, ok := exampleErr.(ContentTypeFilter); ok {
		errContentType = ctf.ContentType(errContentType)
	}
	errType := deref(reflect.TypeOf(exampleErr))
	errSchema := registry.Schema(errType, true, getHint(errType, "", "Error"))
	for _, code := range op.Errors {
		op.Responses[strconv.Itoa(code)] = &Response{
			Description: http.StatusText(code),
			Content: map[string]*MediaType{
				errContentType: {
					Schema: errSchema,
				},
			},
		}
	}
	if len(op.Responses) <= 1 && len(op.Errors) == 0 {
		// No errors are defined, so set a default response.
		op.Responses["default"] = &Response{
			Description: "Error",
			Content: map[string]*MediaType{
				errContentType: {
					Schema: errSchema,
				},
			},
		}
	}
}

// getParamValue extracts the requested parameter from the relevant
// context or cookie source. If unset, the function returns the default value
// for this parameter.
func getParamValue(p paramFieldInfo, ctx Context, cookies map[string]*http.Cookie) string {
	var value string
	switch p.Loc {
	case "path":
		value = ctx.Param(p.Name)
	case "query":
		value = ctx.Query(p.Name)
	case "header":
		value = ctx.Header(p.Name)
	case "cookie":
		if c, ok := cookies[p.Name]; ok {
			value = c.Value
		}
	}
	if value == "" {
		value = p.Default
	}
	return value
}

var errUnparsable = errors.New("unparsable value")

// parseInto converts the string value into the expected type using the
// parameter field information p and sets the result on f.
func parseInto(ctx Context, f reflect.Value, value string, preSplit []string, p paramFieldInfo) (any, error) {
	// built-in types
	switch p.Type.Kind() {
	case reflect.String:
		f.SetString(value)
		return value, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.SetInt(v)
		return v, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.SetUint(v)
		return v, nil
	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, errors.New("invalid float")
		}
		f.SetFloat(v)
		return v, nil
	case reflect.Bool:
		v, err := strconv.ParseBool(value)
		if err != nil {
			return nil, errors.New("invalid boolean")
		}
		f.SetBool(v)
		return v, nil
	case reflect.Slice:
		var values []string
		if preSplit != nil {
			values = preSplit
		} else {
			if p.Explode {
				u := ctx.URL()
				values = (&u).Query()[p.Name]
			} else {
				values = strings.Split(value, ",")
			}
		}
		pv, err := parseSliceInto(f, values)
		if err != nil {
			if errors.Is(err, errUnparsable) {
				break
			}
			return nil, err
		}
		return pv, nil
	}

	// special types
	switch f.Type() {
	case timeType: // Special case: time.Time
		// return nil, errors.New(value)
		t, err := time.Parse(p.TimeFormat, value)
		if err != nil {
			return nil, errors.New("invalid date/time for format " + p.TimeFormat)
		}
		f.Set(reflect.ValueOf(t))
		return value, nil
	case urlType: // Special case: url.URL
		u, err := url.Parse(value)
		if err != nil {
			return nil, errors.New("invalid url.URL value")
		}
		f.Set(reflect.ValueOf(*u))
		return value, nil
	}

	// Last resort: use the `encoding.TextUnmarshaler` interface.
	if fn, ok := f.Addr().Interface().(encoding.TextUnmarshaler); ok {
		if err := fn.UnmarshalText([]byte(value)); err != nil {
			return nil, errors.New("invalid value: " + err.Error())
		}
		return value, nil
	}

	panic("unsupported param type " + p.Type.String())
}

// parseSliceInto converts a slice of string values into the expected type of f
// and sets the result on f.
func parseSliceInto(f reflect.Value, values []string) (any, error) {
	switch f.Type().Elem().Kind() {

	case reflect.String:
		if f.Type() == reflect.TypeOf(values) {
			f.Set(reflect.ValueOf(values))
		} else {
			// Change element type to support slice of string subtypes (enums)
			enumValues := reflect.New(f.Type()).Elem()
			for _, val := range values {
				enumVal := reflect.New(f.Type().Elem()).Elem()
				enumVal.SetString(val)
				enumValues.Set(reflect.Append(enumValues, enumVal))
			}
			f.Set(enumValues)
		}
		return values, nil

	case reflect.Int:
		vs, err := parseArrElement(values, func(s string) (int, error) {
			val, err := strconv.ParseInt(s, 10, strconv.IntSize)
			if err != nil {
				return 0, err
			}
			return int(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Int8:
		vs, err := parseArrElement(values, func(s string) (int8, error) {
			val, err := strconv.ParseInt(s, 10, 8)
			if err != nil {
				return 0, err
			}
			return int8(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Int16:
		vs, err := parseArrElement(values, func(s string) (int16, error) {
			val, err := strconv.ParseInt(s, 10, 16)
			if err != nil {
				return 0, err
			}
			return int16(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Int32:
		vs, err := parseArrElement(values, func(s string) (int32, error) {
			val, err := strconv.ParseInt(s, 10, 32)
			if err != nil {
				return 0, err
			}
			return int32(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Int64:
		vs, err := parseArrElement(values, func(s string) (int64, error) {
			val, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return 0, err
			}
			return val, nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Uint:
		vs, err := parseArrElement(values, func(s string) (uint, error) {
			val, err := strconv.ParseUint(s, 10, strconv.IntSize)
			if err != nil {
				return 0, err
			}
			return uint(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Uint8:
		vs, err := parseArrElement(values, func(s string) (uint8, error) {
			val, err := strconv.ParseUint(s, 10, 8)
			if err != nil {
				return 0, err
			}
			return uint8(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Uint16:
		vs, err := parseArrElement(values, func(s string) (uint16, error) {
			val, err := strconv.ParseUint(s, 10, 16)
			if err != nil {
				return 0, err
			}
			return uint16(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Uint32:
		vs, err := parseArrElement(values, func(s string) (uint32, error) {
			val, err := strconv.ParseUint(s, 10, 32)
			if err != nil {
				return 0, err
			}
			return uint32(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Uint64:
		vs, err := parseArrElement(values, func(s string) (uint64, error) {
			val, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				return 0, err
			}
			return val, nil
		})
		if err != nil {
			return nil, errors.New("invalid integer")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Float32:
		vs, err := parseArrElement(values, func(s string) (float32, error) {
			val, err := strconv.ParseFloat(s, 32)
			if err != nil {
				return 0, err
			}
			return float32(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid floating value")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil

	case reflect.Float64:
		vs, err := parseArrElement(values, func(s string) (float64, error) {
			val, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return 0, err
			}
			return float64(val), nil
		})
		if err != nil {
			return nil, errors.New("invalid floating value")
		}
		f.Set(reflect.ValueOf(vs))
		return vs, nil
	}
	return nil, errUnparsable
}

type contextError struct {
	Code int
	Msg  string
	Errs []error
}

func (e *contextError) Error() string {
	return e.Msg
}

func writeErr(api API, ctx Context, cErr *contextError, res ValidateResult) {
	if cErr.Errs != nil {
		WriteErr(api, ctx, cErr.Code, cErr.Msg, cErr.Errs...)
	} else {
		WriteErr(api, ctx, cErr.Code, cErr.Msg, res.Errors...)
	}
}

func processMultipartMsgBody(form *multipart.Form, op Operation, v reflect.Value, rbt rawBodyType, rawBodyIndex []int, formValueParser func(val reflect.Value)) *contextError {
	f := v.FieldByIndex(rawBodyIndex)
	switch rbt {
	case rbtMultipart:
		// f is of type multipart.Form
		f.Set(reflect.ValueOf(*form))
	case rbtMultipartDecoded:
		// f is of type MultipartFormFiles[T]
		f.FieldByName("Form").Set(reflect.ValueOf(form))
		r := f.Addr().
			MethodByName("Decode").
			Call(
				[]reflect.Value{
					reflect.ValueOf(op.RequestBody.Content["multipart/form-data"]),
					reflect.ValueOf(formValueParser),
				})
		errs := r[0].Interface().([]error)
		if errs != nil {
			return &contextError{Code: http.StatusUnprocessableEntity, Msg: "validation failed", Errs: errs}
		}
	}
	return nil
}

func readForm(ctx Context) (*multipart.Form, *ErrorDetail) {
	form, err := ctx.GetMultipartForm()
	if err != nil {
		return form, &ErrorDetail{
			Location: "body",
			Message:  "cannot read multipart form: " + err.Error(),
		}
	}
	return form, nil
}

type intoUnmarshaler = func(data []byte, v any) error

// processRegularMsgBody parses the raw body with unmarshaler and validates it
// with validator. Validation errors are documented in res and the
// corresponding error code is returned. If no errors were found, the return
// value is -1.
func processRegularMsgBody(body []byte, op Operation, v reflect.Value, hasInputBody bool, inputBodyIndex []int, unmarshaler intoUnmarshaler, validator func(data any, res *ValidateResult), defaults *findResult[any], res *ValidateResult) (int, *contextError) {
	errStatus := -1
	// Check preconditions
	if len(body) == 0 {
		if op.RequestBody != nil && op.RequestBody.Required {
			return errStatus, &contextError{Code: http.StatusBadRequest, Msg: "request body is required"}
		}
		return errStatus, nil
	}
	if !hasInputBody {
		return errStatus, nil
	}

	// Validate
	isValid := true
	if !op.SkipValidateBody {
		validateErrStatus := validateBody(body, unmarshaler, validator, res)
		errStatus = validateErrStatus
		if errStatus > 0 {
			isValid = false
		}
	}

	// Parse into value
	if len(inputBodyIndex) > 0 {
		if err := parseBodyInto(v, inputBodyIndex, unmarshaler, body, defaults); err != nil && isValid {
			// Hmm, this should have worked... validator missed something?
			res.Errors = append(res.Errors, err)
		}
	}
	return errStatus, nil
}

// validateBody parses the raw body with u and validates it with the validator.
// Any errors are documented in res and the corresponding error code is
// returned. If no errors were found, the return value is -1.
func validateBody(body []byte, u intoUnmarshaler, validator func(data any, res *ValidateResult), res *ValidateResult) int {
	errStatus := -1
	// Validate the input. First, parse the body into []any or map[string]any
	// or equivalent, which can be easily validated. Then, convert to the
	// expected struct type to call the handler.
	var parsed any
	if err := u(body, &parsed); err != nil {
		errStatus = http.StatusBadRequest
		if errors.Is(err, ErrUnknownContentType) {
			errStatus = http.StatusUnsupportedMediaType
		}

		res.Errors = append(res.Errors, &ErrorDetail{
			Location: "body",
			Message:  err.Error(),
			Value:    string(body),
		})
	} else {
		preValidationErrCount := len(res.Errors)
		validator(parsed, res)
		if len(res.Errors)-preValidationErrCount > 0 {
			errStatus = http.StatusUnprocessableEntity
		}
	}
	return errStatus
}

// parseBodyInto parses the raw body with u and populates the result in v at
// index bodyIndex. Afterwards, it sets default values on v for all fields that
// were not populated with body.
func parseBodyInto(v reflect.Value, bodyIndex []int, u intoUnmarshaler, body []byte, defaults *findResult[any]) *ErrorDetail {
	// We need to get the body into the correct type now that it has been
	// validated. Benchmarks on Go 1.20 show that using `json.Unmarshal` a
	// second time is faster than `mapstructure.Decode` or any of the other
	// common reflection-based approaches when using real-world medium-sized
	// JSON payloads with lots of strings.
	f := v.FieldByIndex(bodyIndex)
	if err := u(body, f.Addr().Interface()); err != nil {
		return &ErrorDetail{
			Location: "body",
			Message:  err.Error(),
			Value:    string(body),
		}
	}
	// Set defaults for any fields that were not in the input.
	defaults.Every(v, func(item reflect.Value, def any) {
		if item.IsZero() {
			if item.Kind() == reflect.Pointer {
				item.Set(reflect.New(item.Type().Elem()))
				item = item.Elem()
			}
			item.Set(reflect.Indirect(reflect.ValueOf(def)))
		}
	})
	return nil
}

// readBody reads the message body from ctx into buf, respecting the
func readBody(buf io.Writer, ctx Context, maxBytes int64) *contextError {
	reader := ctx.BodyReader()
	if reader == nil {
		reader = bytes.NewReader(nil)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}
	if maxBytes > 0 {
		reader = io.LimitReader(reader, maxBytes)
	}
	count, err := io.Copy(buf, reader)
	if maxBytes > 0 {
		if count == maxBytes {
			return &contextError{Code: http.StatusRequestEntityTooLarge, Msg: fmt.Sprintf("request body is too large limit=%d bytes", maxBytes)}
		}
	}
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return &contextError{Code: http.StatusRequestTimeout, Msg: "request body read timeout"}
		}

		return &contextError{Code: http.StatusInternalServerError, Msg: "cannot read request body", Errs: []error{err}}
	}
	return nil
}

// AutoRegister auto-detects operation registration methods and registers them
// with the given API. Any method named `Register...` will be called and
// passed the API as the only argument. Since registration happens at
// service startup, no errors are returned and methods should panic on error.
//
//	type ItemsHandler struct {}
//
//	func (s *ItemsHandler) RegisterListItems(api API) {
//		huma.Register(api, huma.Operation{
//			OperationID: "ListItems",
//			Method: http.MethodGet,
//			Path: "/items",
//		}, s.ListItems)
//	}
//
//	func main() {
//		router := chi.NewMux()
//		config := huma.DefaultConfig("My Service", "1.0.0")
//		api := huma.NewExampleAPI(router, config)
//
//		itemsHandler := &ItemsHandler{}
//		huma.AutoRegister(api, itemsHandler)
//	}
func AutoRegister(api API, server any) {
	args := []reflect.Value{reflect.ValueOf(server), reflect.ValueOf(api)}

	t := reflect.TypeOf(server)
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		if strings.HasPrefix(m.Name, "Register") && len(m.Name) > 8 {
			m.Func.Call(args)
		}
	}
}

var reRemoveIDs = regexp.MustCompile(`\{([^}]+)\}`)

// GenerateOperationID generates an operation ID from the method, path,
// and response type. The operation ID is used to uniquely identify an
// operation in the OpenAPI spec. The generated ID is kebab-cased and
// includes the method and path, with any path parameters replaced by
// their names.
//
// Examples:
//
//   - GET /things` -> `list-things
//   - GET /things/{thing-id} -> get-things-by-thing-id
//   - PUT /things/{thingId}/favorite -> put-things-by-thing-id-favorite
//
// This function can be overridden to provide custom operation IDs.
var GenerateOperationID = func(method, path string, response any) string {
	action := method
	t := deref(reflect.TypeOf(response))
	if t.Kind() != reflect.Struct {
		panic("Response type must be a struct")
	}
	body, hasBody := t.FieldByName("Body")
	if hasBody && method == http.MethodGet && deref(body.Type).Kind() == reflect.Slice {
		// Special case: GET with a slice response body is a list operation.
		action = "list"
	}
	return casing.Kebab(action + "-" + reRemoveIDs.ReplaceAllString(path, "by-$1"))
}

// GenerateSummary generates an operation summary from the method, path,
// and response type. The summary is used to describe an operation in the
// OpenAPI spec. The generated summary is capitalized and includes the
// method and path, with any path parameters replaced by their names.
//
// Examples:
//
//   - GET /things` -> `List things`
//   - GET /things/{thing-id} -> `Get things by thing id`
//   - PUT /things/{thingId}/favorite -> `Put things by thing id favorite`
//
// This function can be overridden to provide custom operation summaries.
var GenerateSummary = func(method, path string, response any) string {
	action := method
	t := deref(reflect.TypeOf(response))
	if t.Kind() != reflect.Struct {
		panic("Response type must be a struct")
	}
	body, hasBody := t.FieldByName("Body")
	if hasBody && method == http.MethodGet && deref(body.Type).Kind() == reflect.Slice {
		// Special case: GET with a slice response body is a list operation.
		action = "list"
	}
	path = reRemoveIDs.ReplaceAllString(path, "by-$1")
	phrase := strings.ReplaceAll(casing.Kebab(strings.ToLower(action)+" "+path, strings.ToLower, casing.Initialism), "-", " ")
	return strings.ToUpper(phrase[:1]) + phrase[1:]
}

func OperationTags(tags ...string) func(o *Operation) {
	return func(o *Operation) {
		o.Tags = tags
	}
}

func convenience[I, O any](api API, method, path string, handler func(context.Context, *I) (*O, error), operationHandlers ...func(o *Operation)) {
	var o *O
	opID := GenerateOperationID(method, path, o)
	opSummary := GenerateSummary(method, path, o)
	operation := Operation{
		OperationID: opID,
		Summary:     opSummary,
		Method:      method,
		Path:        path,
		Metadata:    map[string]any{},
	}
	for _, oh := range operationHandlers {
		oh(&operation)
	}
	// If not modified, hint that these were auto-generated!
	if operation.OperationID == opID {
		operation.Metadata["_convenience_id"] = opID
		operation.Metadata["_convenience_id_out"] = o
	}
	if operation.Summary == opSummary {
		operation.Metadata["_convenience_summary"] = opSummary
		operation.Metadata["_convenience_summary_out"] = o
	}
	Register(api, operation, handler)
}

// Get HTTP operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header/cookie parameters and/or body. The output
// struct must be a struct with fields for the output headers and body of the
// operation, if any.
//
//	huma.Get(api, "/things", func(ctx context.Context, input *struct{
//		Body []Thing
//	}) (*ListThingOutput, error) {
//		// TODO: list things from DB...
//		resp := &ListThingOutput{}
//		resp.Body = []Thing{{ID: "1", Name: "Thing 1"}}
//		return resp, nil
//	})
//
// This is a convenience wrapper around `huma.Register`.
func Get[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), operationHandlers ...func(o *Operation)) {
	convenience(api, http.MethodGet, path, handler, operationHandlers...)
}

// Post HTTP operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header/cookie parameters and/or body. The output
// struct must be a struct with fields for the output headers and body of the
// operation, if any.
//
//	huma.Post(api, "/things", func(ctx context.Context, input *struct{
//		Body Thing
//	}) (*PostThingOutput, error) {
//		// TODO: save thing to DB...
//		resp := &PostThingOutput{}
//		resp.Location = "/things/" + input.Body.ID
//		return resp, nil
//	})
//
// This is a convenience wrapper around `huma.Register`.
func Post[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), operationHandlers ...func(o *Operation)) {
	convenience(api, http.MethodPost, path, handler, operationHandlers...)
}

// Head HTTP operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header/cookie parameters. The output struct must be a
// struct with fields for the output headers of the operation, if any.
//
//	huma.Head(api, "/things/{thing-id}", func(ctx context.Context, input *struct{
//		ID string `path:"thing-id"`
//		Header string `header:"X-My-Header"`
//	}) (*HeadThingOutput, error) {
//		// TODO: get info from DB...
//		resp := &HeadThingOutput{}
//		return resp, nil
//	})
//
// This is a convenience wrapper around `huma.Register`.
func Head[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), operationHandlers ...func(o *Operation)) {
	convenience(api, http.MethodHead, path, handler, operationHandlers...)
}

// Put HTTP operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header/cookie parameters and/or body. The output
// struct must be a struct with fields for the output headers and body of the
// operation, if any.
//
//	huma.Put(api, "/things/{thing-id}", func(ctx context.Context, input *struct{
//		ID string `path:"thing-id"`
//		Body Thing
//	}) (*PutThingOutput, error) {
//		// TODO: save thing to DB...
//		resp := &PutThingOutput{}
//		return resp, nil
//	})
//
// This is a convenience wrapper around `huma.Register`.
func Put[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), operationHandlers ...func(o *Operation)) {
	convenience(api, http.MethodPut, path, handler, operationHandlers...)
}

// Patch HTTP operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header/cookie parameters and/or body. The output
// struct must be a struct with fields for the output headers and body of the
// operation, if any.
//
//	huma.Patch(api, "/things/{thing-id}", func(ctx context.Context, input *struct{
//		ID string `path:"thing-id"`
//		Body ThingPatch
//	}) (*PatchThingOutput, error) {
//		// TODO: save thing to DB...
//		resp := &PutThingOutput{}
//		return resp, nil
//	})
//
// This is a convenience wrapper around `huma.Register`.
func Patch[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), operationHandlers ...func(o *Operation)) {
	convenience(api, http.MethodPatch, path, handler, operationHandlers...)
}

// Delete HTTP operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header/cookie parameters and/or body. The output
// struct must be a struct with fields for the output headers and body of the
// operation, if any.
//
//	huma.Delete(api, "/things/{thing-id}", func(ctx context.Context, input *struct{
//		ID string `path:"thing-id"`
//	}) (*struct{}, error) {
//		// TODO: remove thing from DB...
//		return nil, nil
//	})
//
// This is a convenience wrapper around `huma.Register`.
func Delete[I, O any](api API, path string, handler func(context.Context, *I) (*O, error), operationHandlers ...func(o *Operation)) {
	convenience(api, http.MethodDelete, path, handler, operationHandlers...)
}
