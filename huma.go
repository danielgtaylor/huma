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

type paramFieldInfo struct {
	Type       reflect.Type
	Name       string
	Loc        string
	Required   bool
	Default    string
	TimeFormat string
	Explode    bool
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
			explode = &pfi.Explode
		} else if h := f.Tag.Get("header"); h != "" {
			pfi.Loc = "header"
			name = h
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
}

func findHeaders(t reflect.Type) *findResult[*headerInfo] {
	return findInType(t, nil, func(sf reflect.StructField, i []int) *headerInfo {
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
		return &headerInfo{sf, header, timeFormat}
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

	ignoreAnonymous := false
	if onType != nil {
		if v := onType(t, path); v != zero {
			result.Paths = append(result.Paths, findResultPath[T]{path, v})

			// Found what we were looking for in the type, no need to go deeper.
			// We do still want to potentially process each non-anonymous field,
			// so only skip anonymous ones.
			ignoreAnonymous = true
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
			if ignoreAnonymous && f.Anonymous {
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
				// Always process embedded structs and named fields which are not
				// structs. If `recurseFields` is true then we also process named
				// struct fields recursively.
				visited[t] = struct{}{}
				_findInType(f.Type, fi, result, onType, onField, recurseFields, visited, ignore...)
				delete(visited, t)
			}
		}
	case reflect.Slice:
		_findInType(t.Elem(), path, result, onType, onField, recurseFields, visited, ignore...)
	case reflect.Map:
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

	inputType := reflect.TypeOf((*I)(nil)).Elem()
	if inputType.Kind() != reflect.Struct {
		panic("input must be a struct")
	}
	inputParams := findParams(registry, &op, inputType)
	inputBodyIndex := []int{}
	hasInputBody := false
	if f, ok := inputType.FieldByName("Body"); ok {
		hasInputBody = true
		inputBodyIndex = f.Index
		if op.RequestBody == nil {
			op.RequestBody = &RequestBody{}
		}

		required := f.Type.Kind() != reflect.Ptr && f.Type.Kind() != reflect.Interface
		if f.Tag.Get("required") == "true" {
			required = true
		}

		contentType := "application/json"
		if c := f.Tag.Get("contentType"); c != "" {
			contentType = c
		}
		hint := getHint(inputType, f.Name, op.OperationID+"Request")
		if nameHint := f.Tag.Get("nameHint"); nameHint != "" {
			hint = nameHint
		}
		s := SchemaFromField(registry, f, hint)

		op.RequestBody.Required = required

		if op.RequestBody.Content == nil {
			op.RequestBody.Content = map[string]*MediaType{}
		}
		op.RequestBody.Content[contentType] = &MediaType{Schema: s}

		if op.BodyReadTimeout == 0 {
			// 5 second default
			op.BodyReadTimeout = 5 * time.Second
		}

		if op.MaxBodyBytes == 0 {
			// 1 MB default
			op.MaxBodyBytes = 1024 * 1024
		}
	}
	rawBodyIndex := []int{}
	rawBodyMultipart := false
	rawBodyDecodedMultipart := false
	if f, ok := inputType.FieldByName("RawBody"); ok {
		rawBodyIndex = f.Index
		if op.RequestBody == nil {
			op.RequestBody = &RequestBody{
				Required: true,
			}
		}

		if op.RequestBody.Content == nil {
			op.RequestBody.Content = map[string]*MediaType{}
		}

		contentType := "application/octet-stream"

		if f.Type.String() == "multipart.Form" {
			contentType = "multipart/form-data"
			rawBodyMultipart = true
		}
		if strings.HasPrefix(f.Type.Name(), "MultipartFormFiles") {
			contentType = "multipart/form-data"
			rawBodyDecodedMultipart = true
		}

		if c := f.Tag.Get("contentType"); c != "" {
			contentType = c
		}

		switch contentType {
		case "multipart/form-data":
			if op.RequestBody.Content["multipart/form-data"] != nil {
				break
			}
			if rawBodyMultipart {
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
			}
			if rawBodyDecodedMultipart {
				dataField, ok := f.Type.FieldByName("data")
				if !ok {
					panic("Expected type MultipartFormFiles[T] to have a 'data *T' generic pointer field")
				}
				op.RequestBody.Content["multipart/form-data"] = &MediaType{
					Schema:   multiPartFormFileSchema(dataField.Type.Elem()),
					Encoding: multiPartContentEncoding(dataField.Type.Elem()),
				}
				op.RequestBody.Required = false
			}
		default:
			op.RequestBody.Content[contentType] = &MediaType{
				Schema: &Schema{
					Type:   "string",
					Format: "binary",
				},
			}
		}
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

	resolvers := findResolvers(resolverType, inputType)
	defaults := findDefaults(registry, inputType)

	if op.Responses == nil {
		op.Responses = map[string]*Response{}
	}
	outputType := reflect.TypeOf((*O)(nil)).Elem()
	if outputType.Kind() != reflect.Struct {
		panic("output must be a struct")
	}

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
	outHeaders := findHeaders(outputType)
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
	for _, entry := range outHeaders.Paths {
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

	if len(op.Errors) > 0 && (len(inputParams.Paths) > 0 || hasInputBody) {
		op.Errors = append(op.Errors, http.StatusUnprocessableEntity)
	}
	if len(op.Errors) > 0 {
		op.Errors = append(op.Errors, http.StatusInternalServerError)
	}

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

	if !op.Hidden {
		oapi.AddOperation(&op)
	}

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
			var value string
			switch p.Loc {
			case "path":
				value = ctx.Param(p.Name)
			case "query":
				value = ctx.Query(p.Name)
			case "header":
				value = ctx.Header(p.Name)
			case "cookie":
				if cookies == nil {
					// Only parse the cookie headers once, on-demand.
					cookies = map[string]*http.Cookie{}
					for _, c := range ReadCookies(ctx) {
						cookies[c.Name] = c
					}
				}
				if c, ok := cookies[p.Name]; ok {
					// Special case: http.Cookie type, meaning we want the entire parsed
					// cookie struct, not just the value.
					if f.Type() == cookieType {
						f.Set(reflect.ValueOf(cookies[p.Name]).Elem())
						return
					}

					value = c.Value
				}
			}

			pb.Reset()
			pb.Push(p.Loc)
			pb.Push(p.Name)

			if value == "" && p.Default != "" {
				value = p.Default
			}

			if !op.SkipValidateParams && p.Required && value == "" {
				// Path params are always required.
				res.Add(pb, "", "required "+p.Loc+" parameter is missing")
				return
			}

			if value != "" {
				var pv any

				switch p.Type.Kind() {
				case reflect.String:
					f.SetString(value)
					pv = value
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					v, err := strconv.ParseInt(value, 10, 64)
					if err != nil {
						res.Add(pb, value, "invalid integer")
						return
					}
					f.SetInt(v)
					pv = v
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						res.Add(pb, value, "invalid integer")
						return
					}
					f.SetUint(v)
					pv = v
				case reflect.Float32, reflect.Float64:
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						res.Add(pb, value, "invalid float")
						return
					}
					f.SetFloat(v)
					pv = v
				case reflect.Bool:
					v, err := strconv.ParseBool(value)
					if err != nil {
						res.Add(pb, value, "invalid boolean")
						return
					}
					f.SetBool(v)
					pv = v
				default:
					if f.Type().Kind() == reflect.Slice {
						var values []string
						if p.Explode {
							u := ctx.URL()
							values = (&u).Query()[p.Name]
						} else {
							values = strings.Split(value, ",")
						}
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
							pv = values

						case reflect.Int:
							vs, err := parseArrElement(values, func(s string) (int, error) {
								val, err := strconv.ParseInt(s, 10, strconv.IntSize)
								if err != nil {
									return 0, err
								}
								return int(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Int8:
							vs, err := parseArrElement(values, func(s string) (int8, error) {
								val, err := strconv.ParseInt(s, 10, 8)
								if err != nil {
									return 0, err
								}
								return int8(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Int16:
							vs, err := parseArrElement(values, func(s string) (int16, error) {
								val, err := strconv.ParseInt(s, 10, 16)
								if err != nil {
									return 0, err
								}
								return int16(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Int32:
							vs, err := parseArrElement(values, func(s string) (int32, error) {
								val, err := strconv.ParseInt(s, 10, 32)
								if err != nil {
									return 0, err
								}
								return int32(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Int64:
							vs, err := parseArrElement(values, func(s string) (int64, error) {
								val, err := strconv.ParseInt(s, 10, 64)
								if err != nil {
									return 0, err
								}
								return val, nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Uint:
							vs, err := parseArrElement(values, func(s string) (uint, error) {
								val, err := strconv.ParseUint(s, 10, strconv.IntSize)
								if err != nil {
									return 0, err
								}
								return uint(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Uint16:
							vs, err := parseArrElement(values, func(s string) (uint16, error) {
								val, err := strconv.ParseUint(s, 10, 16)
								if err != nil {
									return 0, err
								}
								return uint16(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Uint32:
							vs, err := parseArrElement(values, func(s string) (uint32, error) {
								val, err := strconv.ParseUint(s, 10, 32)
								if err != nil {
									return 0, err
								}
								return uint32(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Uint64:
							vs, err := parseArrElement(values, func(s string) (uint64, error) {
								val, err := strconv.ParseUint(s, 10, 64)
								if err != nil {
									return 0, err
								}
								return val, nil
							})
							if err != nil {
								res.Add(pb, value, "invalid integer")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Float32:
							vs, err := parseArrElement(values, func(s string) (float32, error) {
								val, err := strconv.ParseFloat(s, 32)
								if err != nil {
									return 0, err
								}
								return float32(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid floating value")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs

						case reflect.Float64:
							vs, err := parseArrElement(values, func(s string) (float64, error) {
								val, err := strconv.ParseFloat(s, 64)
								if err != nil {
									return 0, err
								}
								return float64(val), nil
							})
							if err != nil {
								res.Add(pb, value, "invalid floating value")
								return
							}
							f.Set(reflect.ValueOf(vs))
							pv = vs
						}
						break
					}

					// Special case: time.Time
					if f.Type() == timeType {
						t, err := time.Parse(p.TimeFormat, value)
						if err != nil {
							res.Add(pb, value, "invalid date/time for format "+p.TimeFormat)
							return
						}
						f.Set(reflect.ValueOf(t))
						pv = value
						break
						// Special case: url.URL
					} else if f.Type() == urlType {
						u, err := url.Parse(value)
						if err != nil {
							res.Add(pb, value, "invalid url.URL value")
							return
						}
						f.Set(reflect.ValueOf(*u))
						pv = value
						break
					}

					// Last resort: use the `encoding.TextUnmarshaler` interface.
					if fn, ok := f.Addr().Interface().(encoding.TextUnmarshaler); ok {
						if err := fn.UnmarshalText([]byte(value)); err != nil {
							res.Add(pb, value, "invalid value: "+err.Error())
							return
						}
						pv = value
						break
					}

					panic("unsupported param type " + p.Type.String())
				}

				if !op.SkipValidateParams {
					Validate(oapi.Components.Schemas, p.Schema, pb, ModeWriteToServer, pv, res)
				}
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

			if rawBodyMultipart || rawBodyDecodedMultipart {
				form, err := ctx.GetMultipartForm()
				if err != nil {
					res.Errors = append(res.Errors, &ErrorDetail{
						Location: "body",
						Message:  "cannot read multipart form: " + err.Error(),
					})
				} else {
					f := v
					for _, i := range rawBodyIndex {
						f = f.Field(i)
					}
					if rawBodyMultipart {
						f.Set(reflect.ValueOf(*form))
					} else {
						f.FieldByName("Form").Set(reflect.ValueOf(form))
						r := f.Addr().
							MethodByName("Decode").
							Call([]reflect.Value{
								reflect.ValueOf(op.RequestBody.Content["multipart/form-data"]),
							})
						errs := r[0].Interface().([]error)
						if errs != nil {
							WriteErr(api, ctx, http.StatusUnprocessableEntity, "validation failed", errs...)
							return
						}
					}
				}
			} else {
				buf := bufPool.Get().(*bytes.Buffer)
				reader := ctx.BodyReader()
				if reader == nil {
					reader = bytes.NewReader(nil)
				}
				if closer, ok := reader.(io.Closer); ok {
					defer closer.Close()
				}
				if op.MaxBodyBytes > 0 {
					reader = io.LimitReader(reader, op.MaxBodyBytes)
				}
				count, err := io.Copy(buf, reader)
				if op.MaxBodyBytes > 0 {
					if count == op.MaxBodyBytes {
						buf.Reset()
						bufPool.Put(buf)
						WriteErr(api, ctx, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body is too large limit=%d bytes", op.MaxBodyBytes), res.Errors...)
						return
					}
				}
				if err != nil {
					buf.Reset()
					bufPool.Put(buf)

					if e, ok := err.(net.Error); ok && e.Timeout() {
						WriteErr(api, ctx, http.StatusRequestTimeout, "request body read timeout", res.Errors...)
						return
					}

					WriteErr(api, ctx, http.StatusInternalServerError, "cannot read request body", err)
					return
				}
				body := buf.Bytes()

				if len(rawBodyIndex) > 0 {
					f := v
					for _, i := range rawBodyIndex {
						f = f.Field(i)
					}
					f.SetBytes(body)
				}

				if len(body) == 0 {
					if op.RequestBody != nil && op.RequestBody.Required {
						buf.Reset()
						bufPool.Put(buf)
						WriteErr(api, ctx, http.StatusBadRequest, "request body is required", res.Errors...)
						return
					}
				} else {
					parseErrCount := 0
					if hasInputBody && !op.SkipValidateBody {
						// Validate the input. First, parse the body into []any or map[string]any
						// or equivalent, which can be easily validated. Then, convert to the
						// expected struct type to call the handler.
						var parsed any
						if err := api.Unmarshal(ctx.Header("Content-Type"), body, &parsed); err != nil {
							errStatus = http.StatusBadRequest
							if errors.Is(err, ErrUnknownContentType) {
								errStatus = http.StatusUnsupportedMediaType
							}
							res.Errors = append(res.Errors, &ErrorDetail{
								Location: "body",
								Message:  err.Error(),
								Value:    body,
							})
							parseErrCount++
						} else {
							pb.Reset()
							pb.Push("body")
							count := len(res.Errors)
							Validate(oapi.Components.Schemas, inSchema, pb, ModeWriteToServer, parsed, res)
							parseErrCount = len(res.Errors) - count
							if parseErrCount > 0 {
								errStatus = http.StatusUnprocessableEntity
							}
						}
					}

					if hasInputBody && len(inputBodyIndex) > 0 {
						// We need to get the body into the correct type now that it has been
						// validated. Benchmarks on Go 1.20 show that using `json.Unmarshal` a
						// second time is faster than `mapstructure.Decode` or any of the other
						// common reflection-based approaches when using real-world medium-sized
						// JSON payloads with lots of strings.
						f := v
						for _, index := range inputBodyIndex {
							f = f.Field(index)
						}
						if err := api.Unmarshal(ctx.Header("Content-Type"), body, f.Addr().Interface()); err != nil {
							if parseErrCount == 0 {
								// Hmm, this should have worked... validator missed something?
								res.Errors = append(res.Errors, &ErrorDetail{
									Location: "body",
									Message:  err.Error(),
									Value:    string(body),
								})
							}
						} else {
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
						}
					}

					if len(rawBodyIndex) > 0 {
						// If the raw body is used, then we must wait until *AFTER* the
						// handler has run to return the body byte buffer to the pool, as
						// the handler can read and modify this buffer. The safest way is
						// to just wait until the end of this handler via defer.
						defer bufPool.Put(buf)
						defer buf.Reset()
					} else {
						// No raw body, and the body has already been unmarshalled above, so
						// we can return the buffer to the pool now as we don't need the
						// bytes any more.
						buf.Reset()
						bufPool.Put(buf)
					}
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
	operation := Operation{
		OperationID: GenerateOperationID(method, path, o),
		Summary:     GenerateSummary(method, path, o),
		Method:      method,
		Path:        path,
	}
	for _, oh := range operationHandlers {
		oh(&operation)
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
//		resp := &PostThingOutput{}
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
