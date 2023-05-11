package huma

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"
)

type paramFieldInfo struct {
	Type      reflect.Type
	IndexPath []int
	Loc       string
	Default   string
	Schema    *Schema
}

func findParamFields(registry Registry, op *Operation, adapter Adapter, path []int, t reflect.Type, m map[string]*paramFieldInfo) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fi := append([]int{}, path...)
		fi = append(fi, i)

		if f.Anonymous {
			findParamFields(registry, op, adapter, path, deref(f.Type), m)
			continue
		}

		if f.Name == "Body" {
			continue
		}

		pfi := &paramFieldInfo{
			Type:      f.Type,
			IndexPath: fi,
			Schema:    SchemaFromField(registry, nil, f),
		}

		var example any
		if e := f.Tag.Get("example"); e != "" {
			example = jsonTagValue(f.Type, f.Tag.Get("example"))
		}

		if def := f.Tag.Get("default"); def != "" {
			pfi.Default = def
		}

		name := ""
		required := false
		if p := f.Tag.Get("path"); p != "" {
			pfi.Loc = "path"
			m[p] = pfi
			name = p
			required = true
		}

		if q := f.Tag.Get("query"); q != "" {
			pfi.Loc = "query"
			m[q] = pfi
			name = q
		}

		if h := f.Tag.Get("header"); h != "" {
			pfi.Loc = "header"
			m[h] = pfi
			name = h
		}

		op.Parameters = append(op.Parameters, &Param{
			Name:     name,
			In:       pfi.Loc,
			Required: required,
			Schema:   pfi.Schema,
			Example:  example,
		})
	}
}

func findResolvers(resolverType, t reflect.Type) *findResult {
	return findInType(t, func(t reflect.Type, path []int) any {
		if reflect.PtrTo(t).Implements(resolverType) {
			return true
		}
		return nil
	}, nil)
}

func findDefaults(t reflect.Type) *findResult {
	return findInType(t, nil, func(sf reflect.StructField, i []int) any {
		if d := sf.Tag.Get("default"); d != "" {
			return jsonTagValue(sf.Type, d)
		}
		return nil
	})
}

type findResult struct {
	Paths []struct {
		Path  []int
		Value any
	}
}

func (r *findResult) every(current reflect.Value, path []int, v any, f func(reflect.Value, any)) {
	if len(path) == 0 {
		f(current, v)
		return
	}

	switch current.Kind() {
	case reflect.Struct:
		r.every(reflect.Indirect(current.Field(path[0])), path[1:], v, f)
	case reflect.Slice:
		for j := 0; j < current.Len(); j++ {
			r.every(reflect.Indirect(current.Index(j)), path, v, f)
		}
	case reflect.Map:
		for _, k := range current.MapKeys() {
			r.every(reflect.Indirect(current.MapIndex(k)), path, v, f)
		}
	default:
		panic("unsupported")
	}
}

func (r *findResult) Every(v reflect.Value, f func(reflect.Value, any)) {
	for i := range r.Paths {
		r.every(v, r.Paths[i].Path, &r.Paths[i].Value, f)
	}
}

func findInType(t reflect.Type, onType func(reflect.Type, []int) any, onField func(reflect.StructField, []int) any) *findResult {
	result := &findResult{}
	_findInType(t, []int{}, result, onType, onField)
	return result
}

func _findInType(t reflect.Type, path []int, result *findResult, onType func(reflect.Type, []int) any, onField func(reflect.StructField, []int) any) {
	t = deref(t)

	if onType != nil {
		if v := onType(t, path); v != nil {
			result.Paths = append(result.Paths, struct {
				Path  []int
				Value any
			}{path, v})
		}
	}

	switch t.Kind() {
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			fi := append([]int{}, path...)
			fi = append(fi, i)
			if onField != nil {
				if v := onField(f, fi); v != nil {
					result.Paths = append(result.Paths, struct {
						Path  []int
						Value any
					}{fi, v})
				}
			}
			_findInType(f.Type, fi, result, onType, onField)
		}
	case reflect.Slice:
		_findInType(t.Elem(), path, result, onType, onField)
	case reflect.Map:
		_findInType(t.Elem(), path, result, onType, onField)
	}
}

func writeErr(api API, op *Operation, ctx Context, status int, msg string, errs ...error) {
	var err any = NewError(status, msg, errs...)

	ct, _ := api.Negotiate(ctx.GetHeader("Accept"))
	if ctf, ok := err.(ContentTypeFilter); ok {
		ct = ctf.ContentType(ct)
	}

	ctx.WriteHeader("Content-Type", ct)
	ctx.WriteStatus(status)
	api.Marshal(ctx, op, strconv.Itoa(status), ct, err)
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

// Register an operation handler for an API. The handler must be a function that
// takes a context and a pointer to the input struct and returns a pointer to the
// output struct and an error. The input struct must be a struct with fields
// for the request path/query/header parameters and/or body. The output struct
// must be a  struct with fields for the output headers and body of the
// operation, if any.
func Register[I, O any](api API, op Operation, handler func(context.Context, *I) (*O, error)) {
	oapi := api.OpenAPI()
	registry := oapi.Components.Schemas

	// fmt.Println("get params")
	inputType := reflect.TypeOf((*I)(nil)).Elem()
	if inputType.Kind() != reflect.Struct {
		panic("input must be a struct")
	}
	inputParams := map[string]*paramFieldInfo{}
	findParamFields(registry, &op, api.Adapter(), []int{}, inputType, inputParams)
	inputBodyIndex := -1
	var inSchema *Schema
	for i := 0; i < inputType.NumField(); i++ {
		// fmt.Println("get schema")
		f := inputType.Field(i)
		if f.Name == "Body" {
			inputBodyIndex = i
			inSchema = registry.Schema(f.Type, true, getHint(inputType, f.Name, op.OperationID+"Request"))
			// addSchemaField(registry, inSchema)
			op.RequestBody = &RequestBody{
				Content: map[string]*MediaType{
					"application/json": {
						Schema: inSchema,
					},
				},
			}

			if op.MaxBodyBytes == 0 {
				// 1 MB default
				op.MaxBodyBytes = 1024 * 1024
			}

			break
		}
	}
	resolvers := findResolvers(resolverType, inputType)
	defaults := findDefaults(inputType)

	if op.Responses == nil {
		op.Responses = map[string]*Response{}
	}
	// fmt.Println("get output")
	outputType := reflect.TypeOf((*O)(nil)).Elem()
	if outputType.Kind() != reflect.Struct {
		panic("output must be a struct")
	}

	outHeaders := map[string]int{}
	outBodyIndex := -1
	for i := 0; i < outputType.NumField(); i++ {
		f := outputType.Field(i)
		if f.Name == "Body" {
			outBodyIndex = i
			status := op.DefaultStatus
			if status == 0 {
				status = http.StatusOK
			}
			outSchema := registry.Schema(f.Type, true, getHint(outputType, f.Name, op.OperationID+"Response"))
			statusStr := fmt.Sprintf("%d", status)
			if op.Responses[statusStr] == nil {
				op.Responses[statusStr] = &Response{}
			}
			if op.Responses[statusStr].Description == "" {
				op.Responses[statusStr].Description = http.StatusText(status)
			}
			if op.Responses[statusStr].Headers == nil {
				op.Responses[statusStr].Headers = map[string]*Param{}
			}
			if op.Responses[statusStr].Content == nil {
				op.Responses[statusStr].Content = map[string]*MediaType{}
			}
			if _, ok := op.Responses[statusStr].Content["application/json"]; !ok {
				op.Responses[statusStr].Content["application/json"] = &MediaType{}
			}
			op.Responses[statusStr].Content["application/json"].Schema = outSchema
			continue
		}

		header := f.Tag.Get("header")
		if header == "" {
			header = f.Name
		}
		outHeaders[header] = i
	}
	if op.DefaultStatus == 0 {
		if outBodyIndex != -1 {
			op.DefaultStatus = http.StatusOK
		} else {
			op.DefaultStatus = http.StatusNoContent
		}
	}
	for name := range outHeaders {
		op.Responses[fmt.Sprintf("%d", op.DefaultStatus)].Headers[name] = &Param{
			Schema: registry.Schema(outputType.Field(outHeaders[name]).Type, true, getHint(outputType, name, op.OperationID+"Response")),
		}
	}

	if len(op.Errors) > 0 && (len(inputParams) > 0 || inputBodyIndex >= -1) {
		op.Errors = append(op.Errors, http.StatusUnprocessableEntity)
	}
	if len(op.Errors) > 0 {
		op.Errors = append(op.Errors, http.StatusInternalServerError)
	}

	errType := reflect.TypeOf(NewError(0, ""))
	errSchema := registry.Schema(errType, true, getHint(errType, "", "Error"))
	for _, code := range op.Errors {
		op.Responses[fmt.Sprintf("%d", code)] = &Response{
			Description: http.StatusText(code),
			Content: map[string]*MediaType{
				"application/json": {
					Schema: errSchema,
				},
			},
		}
	}
	// TODO: if no op.Errors, set a default response as the error type

	if !op.Hidden {
		oapi.AddOperation(&op)
	}

	a := api.Adapter()

	a.Handle(op.Method, op.Path, func(ctx Context) {
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

		v := reflect.ValueOf(&input).Elem()
		for name, p := range inputParams {
			f := v
			for _, i := range p.IndexPath {
				f = reflect.Indirect(f).Field(i)
			}
			var value string
			switch p.Loc {
			case "path":
				value = ctx.GetParam(name)
			case "query":
				value = ctx.GetQuery(name)
			case "header":
				value = ctx.GetHeader(name)
			}

			pb.Reset()
			pb.Push(p.Loc)
			pb.Push(name)

			if value == "" && p.Default != "" {
				value = p.Default
			}

			if p.Loc == "path" && value == "" {
				// Path params are always required.
				res.Add(pb, "", "required path parameter is missing")
				continue
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
						continue
					}
					f.SetInt(v)
					pv = v
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						res.Add(pb, value, "invalid integer")
						continue
					}
					f.SetUint(v)
					pv = v
				case reflect.Float32, reflect.Float64:
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						res.Add(pb, value, "invalid float")
						continue
					}
					f.SetFloat(v)
					pv = v
				case reflect.Bool:
					v, err := strconv.ParseBool(value)
					if err != nil {
						res.Add(pb, value, "invalid boolean")
						continue
					}
					f.SetBool(v)
					pv = v
				default:
					if f.Type() == timeType {
						t, err := time.Parse(time.RFC3339, value)
						if err != nil {
							res.Add(pb, value, "invalid time")
							continue
						}
						f.Set(reflect.ValueOf(t))
						pv = t
						break
					}
					panic("unsupported param type")
				}

				if !op.SkipValidateParams {
					Validate(oapi.Components.Schemas, p.Schema, pb, ModeWriteToServer, pv, res)
				}
			}
		}

		// Read input body if defined.
		if inputBodyIndex != -1 {
			buf := bufPool.Get().(*bytes.Buffer)
			reader := ctx.GetBodyReader()
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
					writeErr(api, &op, ctx, http.StatusRequestEntityTooLarge, fmt.Sprintf("request body is too large limit=%d bytes", op.MaxBodyBytes))
					return
				}
			}
			if err != nil {
				buf.Reset()
				bufPool.Put(buf)
				writeErr(api, &op, ctx, http.StatusInternalServerError, "cannot read request body", err)
				return
			}
			body := buf.Bytes()

			parseErrCount := 0
			if !op.SkipValidateBody {
				// Validate the input. First, parse the body into []any or map[string]any
				// or equivalent, which can be easily validated. Then, convert to the
				// expected struct type to call the handler.
				var parsed any
				if err := api.Unmarshal(ctx.GetHeader("Content-Type"), body, &parsed); err != nil {
					// TODO: handle not acceptable
					errStatus = http.StatusBadRequest
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

			// We need to get the body into the correct type now that it has been
			// validated. Benchmarks on Go 1.20 show that using `json.Unmarshal` a
			// second time is faster than `mapstructure.Decode` or any of the other
			// common reflection-based approaches when using real-world medium-sized
			// JSON payloads with lots of strings.
			f := v.Field(inputBodyIndex)
			if err := api.Unmarshal(ctx.GetHeader("Content-Type"), body, f.Addr().Interface()); err != nil {
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
						item.Set(reflect.ValueOf(def).Elem().Elem())
					}
				})
			}

			buf.Reset()
			bufPool.Put(buf)
		}

		resolvers.Every(v, func(item reflect.Value, _ any) {
			if resolver, ok := item.Addr().Interface().(Resolver); ok {
				if errs := resolver.Resolve(ctx); len(errs) > 0 {
					res.Errors = append(res.Errors, errs...)
				}
			}
		})

		if len(res.Errors) > 0 {
			writeErr(api, &op, ctx, errStatus, "validation failed", res.Errors...)
			return
		}

		output, err := handler(ctx.GetContext(), &input)
		if err != nil {
			status := http.StatusInternalServerError
			if se, ok := err.(StatusError); ok {
				status = se.GetStatus()
			} else {
				err = NewError(http.StatusInternalServerError, err.Error())
			}

			ct, _ := api.Negotiate(ctx.GetHeader("Accept"))
			if ctf, ok := err.(ContentTypeFilter); ok {
				ct = ctf.ContentType(ct)
			}

			ctx.WriteStatus(status)
			ctx.WriteHeader("Content-Type", ct)
			api.Marshal(ctx, &op, strconv.Itoa(status), ct, err)
			return
		}

		// Serialize output headers
		vo := reflect.ValueOf(output).Elem()
		for header, index := range outHeaders {
			f := vo.Field(index)

			switch f.Kind() {
			case reflect.String:
				ctx.WriteHeader(header, f.String())
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				ctx.WriteHeader(header, strconv.FormatInt(f.Int(), 10))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				ctx.WriteHeader(header, strconv.FormatUint(f.Uint(), 10))
			case reflect.Float32, reflect.Float64:
				ctx.WriteHeader(header, strconv.FormatFloat(f.Float(), 'f', -1, 64))
			case reflect.Bool:
				ctx.WriteHeader(header, strconv.FormatBool(f.Bool()))
			default:
				if f.Type() == timeType {
					// TODO: enable custom serialization via struct tag.
					ctx.WriteHeader(header, f.Interface().(time.Time).Format(http.TimeFormat))
					continue
				}

				ctx.WriteHeader(header, fmt.Sprintf("%v", f.Interface()))
			}
		}

		if outBodyIndex != -1 {
			// Serialize output body
			body := vo.Field(outBodyIndex).Interface()

			ct, err := api.Negotiate(ctx.GetHeader("Accept"))
			if err != nil {
				writeErr(api, &op, ctx, http.StatusNotAcceptable, "unable to marshal response", err)
				return
			}
			if ctf, ok := body.(ContentTypeFilter); ok {
				ct = ctf.ContentType(ct)
			}

			ctx.WriteHeader("Content-Type", ct)
			ctx.WriteStatus(op.DefaultStatus)
			api.Marshal(ctx, &op, strconv.Itoa(op.DefaultStatus), ct, body)
		} else {
			ctx.WriteStatus(op.DefaultStatus)
		}
	})
}
