package huma

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"
)

type paramFieldInfo[R any] struct {
	Type      reflect.Type
	IndexPath []int
	Getter    func(R, string) string
	Loc       string
	Schema    *Schema
}

func getParamFields[R, W any](registry Registry, op *Operation, adapter Adapter[R, W], path []int, t reflect.Type, m map[string]*paramFieldInfo[R]) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		fi := append([]int{}, path...)
		fi = append(fi, i)

		if f.Anonymous {
			getParamFields(registry, op, adapter, path, deref(f.Type), m)
			continue
		}

		if f.Name == "Body" {
			continue
		}

		pfi := &paramFieldInfo[R]{
			Type:      f.Type,
			IndexPath: fi,
			Schema:    SchemaFromField(registry, nil, f),
		}

		var example any
		if e := f.Tag.Get("example"); e != "" {
			example = jsonTagValue(f.Type, f.Tag.Get("example"))
		}

		name := ""
		required := false
		if p := f.Tag.Get("path"); p != "" {
			pfi.Loc = "path"
			pfi.Getter = adapter.GetParam
			m[p] = pfi
			name = p
			required = true
		}

		if q := f.Tag.Get("query"); q != "" {
			pfi.Loc = "query"
			pfi.Getter = adapter.GetQuery
			m[q] = pfi
			name = q
		}

		if h := f.Tag.Get("header"); h != "" {
			pfi.Loc = "header"
			pfi.Getter = adapter.GetHeader
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

type resolverInfo struct {
	Paths [][]int
}

func findResolvers(resolverType reflect.Type, t reflect.Type, path []int, info *resolverInfo) {
	// fmt.Println("finding", t)
	t = deref(t)

	if reflect.PtrTo(t).Implements(resolverType) {
		info.Paths = append(info.Paths, path)
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
			findResolvers(resolverType, f.Type, fi, info)
		}
	case reflect.Slice:
		// TODO: signal some way to indicate it's not a field index...
		findResolvers(resolverType, t.Elem(), path, info)
	case reflect.Map:
		// TODO...
		findResolvers(resolverType, t.Elem(), path, info)
	}
}

func writeErr[R, W any](api API[R, W], op *Operation, r R, w W, status int, msg string, errs ...error) {
	var err any = NewError(status, msg, errs...)

	a := api.GetAdapter()
	ct, _ := api.Negotiate(a.GetHeader(r, "Accept"))
	if ctf, ok := err.(ContentTypeFilter); ok {
		ct = ctf.ContentType(ct)
	}

	err, err2 := api.TransformBody(op, strconv.Itoa(status), err)
	if err2 != nil {
		// TODO: implement
		panic(err2)
	}

	// data, ct, _ := api.Marshal(a.GetHeader(r, "Accept"), err)
	a.WriteHeader(w, "Content-Type", ct)
	a.WriteStatus(w, status)
	// a.BodyWriter(w).Write(data)
	api.Marshal(ct, a.BodyWriter(w), err)
}

func getHint(parent reflect.Type, name string, other string) string {
	if parent.Name() != "" {
		return parent.Name() + name
	} else {
		return other
	}
}

func addSchemaField(r Registry, s *Schema) {
	if s.Ref != "" {
		s = r.SchemaFromRef(s.Ref)
	}
	if s.Type == TypeObject {
		if s.Properties == nil {
			s.Properties = map[string]*Schema{}
		}
		s.Properties["$schema"] = &Schema{
			Type:        TypeString,
			Format:      "uri",
			Description: "A URL to the JSON Schema for this object.",
		}
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

func Register[I, O, R, W any](api API[R, W], op Operation, handler func(context.Context, *I) (*O, error)) {
	oapi := api.OpenAPI()
	if oapi.Paths == nil {
		oapi.Paths = map[string]*PathItem{}
	}

	item := oapi.Paths[op.Path]
	if item == nil {
		item = &PathItem{}
		oapi.Paths[op.Path] = item
	}

	registry := oapi.Components.Schemas

	switch op.Method {
	case http.MethodGet:
		item.Get = &op
	case http.MethodPost:
		item.Post = &op
	case http.MethodPut:
		item.Put = &op
	case http.MethodPatch:
		item.Patch = &op
	case http.MethodDelete:
		item.Delete = &op
	case http.MethodHead:
		item.Head = &op
	case http.MethodOptions:
		item.Options = &op
	case http.MethodTrace:
		item.Trace = &op
	default:
		panic(fmt.Sprintf("unknown method %s", op.Method))
	}

	// fmt.Println("get params")
	inputType := reflect.TypeOf((*I)(nil)).Elem()
	if inputType.Kind() != reflect.Struct {
		panic("input must be a struct")
	}
	inputParams := map[string]*paramFieldInfo[R]{}
	getParamFields(registry, &op, api.GetAdapter(), []int{}, inputType, inputParams)
	inputBodyIndex := -1
	var inSchema *Schema
	for i := 0; i < inputType.NumField(); i++ {
		// fmt.Println("get schema")
		f := inputType.Field(i)
		if f.Name == "Body" {
			inputBodyIndex = i
			inSchema = registry.Schema(f.Type, true, getHint(inputType, f.Name, op.OperationID+"Request"))
			addSchemaField(registry, inSchema)
			op.RequestBody = &RequestBody{
				Content: map[string]*MediaType{
					"application/json": {
						Schema: inSchema,
					},
				},
			}
			break
		}
	}
	// fmt.Println("get resolvers")
	var resolvers resolverInfo
	findResolvers(reflect.TypeOf((*Resolver[R])(nil)).Elem(), inputType, []int{}, &resolvers)
	// fmt.Printf("%+v\n", resolvers)

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
			if api.HasSchemaField() {
				addSchemaField(registry, outSchema)
			}
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

			// op.Responses[fmt.Sprintf("%d", status)] = &Response{
			// 	Description: http.StatusText(status),
			// 	Headers:     map[string]*ParamOrHeader{},
			// 	Content: map[string]*MediaType{
			// 		"application/json": {
			// 			Schema: outSchema,
			// 		},
			// 	},
			// }
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
	if api.HasSchemaField() {
		addSchemaField(registry, errSchema)
	}
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

	// Precompute schema links for responses.
	defaultSchemaLink := ""
	schemaLinks := map[int]string{}
	for codeStr, resp := range op.Responses {
		if resp.Content != nil {
			for _, media := range resp.Content {
				if media.Schema != nil && media.Schema.Ref != "" {
					if codeStr == "default" {
						defaultSchemaLink = "<" + api.SchemaLink(media.Schema.Ref) + ">; rel=\"describedBy\""
					} else {
						code, _ := strconv.Atoi(codeStr)
						schemaLinks[code] = "<" + api.SchemaLink(media.Schema.Ref) + ">; rel=\"describedBy\""
					}
				}
			}
		}
	}

	a := api.GetAdapter()

	a.Handle(op.Method, op.Path, func(w W, r R) {
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
			value := p.Getter(r, name)

			pb.Reset()
			pb.Push(p.Loc)
			pb.Push(name)

			if p.Loc == "path" && value == "" {
				// Path params are always required.
				res.Add(pb, "", "required path parameter is missing")
				continue
			}

			if value != "" {
				var pv any

				switch p.Type.Kind() {
				case reflect.String:
					pv = value
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					v, err := strconv.ParseInt(value, 10, 64)
					if err != nil {
						res.Add(pb, value, "invalid integer")
						continue
					}
					pv = v
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					v, err := strconv.ParseUint(value, 10, 64)
					if err != nil {
						res.Add(pb, value, "invalid integer")
						continue
					}
					pv = v
				case reflect.Float32, reflect.Float64:
					v, err := strconv.ParseFloat(value, 64)
					if err != nil {
						res.Add(pb, value, "invalid float")
						continue
					}
					pv = v
				case reflect.Bool:
					v, err := strconv.ParseBool(value)
					if err != nil {
						res.Add(pb, value, "invalid boolean")
						continue
					}
					pv = v
				default:
					if f.Type() == timeType {
						t, err := time.Parse(time.RFC3339, value)
						if err != nil {
							res.Add(pb, value, "invalid time")
							continue
						}
						pv = t
						break
					}
					panic("unsupported param type")
				}

				if !op.SkipValidateParams {
					count := len(res.Errors)
					Validate(oapi.Components.Schemas, p.Schema, pb, pv, res)
					if len(res.Errors) > count {
						continue
					}
				}

				switch p.Type.Kind() {
				case reflect.String:
					f.SetString(value)
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					f.SetInt(pv.(int64))
				case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
					f.SetUint(pv.(uint64))
				case reflect.Float32, reflect.Float64:
					f.SetFloat(pv.(float64))
				case reflect.Bool:
					f.SetBool(pv.(bool))
				default:
					f.Set(reflect.ValueOf(pv))
				}
			}
		}

		// Read input body if defined.
		if inputBodyIndex != -1 {
			body, err := a.GetBody(r)
			if err != nil {
				if api.HasSchemaLink() {
					setLink(a, schemaLinks, defaultSchemaLink, http.StatusInternalServerError, w)
				}
				writeErr(api, &op, r, w, http.StatusInternalServerError, "cannot read request body", err)
				return
			}

			parseErrCount := 0
			if !op.SkipValidateBody {
				// Validate the input. First, parse the body into []any or map[string]any
				// or equivalent, which can be easily validated. Then, convert to the
				// expected struct type to call the handler.
				var parsed any
				if err := api.Unmarshal(a.GetHeader(r, "Content-Type"), body, &parsed); err != nil {
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
					Validate(oapi.Components.Schemas, inSchema, pb, parsed, res)
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
			if err := api.Unmarshal(a.GetHeader(r, "Content-Type"), body, f.Addr().Interface()); err != nil {
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
				// TODO: cache whether there are defaults to set, otherwise skip
				// This is too slow and allocates... boo.
				inSchema.SetDefaults(registry, f)
			}
		}

		for _, path := range resolvers.Paths {
			f := v
			for _, i := range path {
				// TODO: slices, maybe use -1 index?
				f = reflect.Indirect(f).Field(i)
			}
			// TODO: compare interface with reflect.call perf
			//f.Method(0).Call([]reflect.Value{reflect.ValueOf(r)}
			if resolver, ok := f.Addr().Interface().(Resolver[R]); ok {
				if errs := resolver.Resolve(r); len(errs) > 0 {
					res.Errors = append(res.Errors, errs...)
				}
			}
		}

		if len(res.Errors) > 0 {
			if api.HasSchemaLink() {
				setLink(a, schemaLinks, defaultSchemaLink, errStatus, w)
			}
			writeErr(api, &op, r, w, errStatus, "validation failed", res.Errors...)
			return
		}

		var output any
		var err error
		output, err = handler(a.GetContext(r), &input)
		if err != nil {
			status := http.StatusInternalServerError
			if se, ok := err.(StatusError); ok {
				status = se.GetStatus()
			}

			ct, _ := api.Negotiate(a.GetHeader(r, "Accept"))
			if ctf, ok := err.(ContentTypeFilter); ok {
				ct = ctf.ContentType(ct)
			}

			err, err2 := api.TransformBody(&op, strconv.Itoa(status), err)
			if err2 != nil {
				// TODO: implement
				panic(err)
			}

			a.WriteStatus(w, status)
			if api.HasSchemaLink() {
				setLink(a, schemaLinks, defaultSchemaLink, status, w)
			}
			a.WriteHeader(w, "Content-Type", ct)
			api.Marshal(ct, a.BodyWriter(w), err)
			return
		}

		// Serialize output headers
		vo := reflect.ValueOf(output).Elem()
		for header, index := range outHeaders {
			f := vo.Field(index)

			switch f.Kind() {
			case reflect.String:
				a.WriteHeader(w, header, f.String())
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				a.WriteHeader(w, header, strconv.FormatInt(f.Int(), 10))
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				a.WriteHeader(w, header, strconv.FormatUint(f.Uint(), 10))
			case reflect.Float32, reflect.Float64:
				a.WriteHeader(w, header, strconv.FormatFloat(f.Float(), 'f', -1, 64))
			case reflect.Bool:
				a.WriteHeader(w, header, strconv.FormatBool(f.Bool()))
			default:
				if f.Type() == timeType {
					// TODO: enable custom serialization via struct tag.
					a.WriteHeader(w, header, f.Interface().(time.Time).Format(http.TimeFormat))
					continue
				}

				a.WriteHeader(w, header, fmt.Sprintf("%v", f.Interface()))
			}
		}

		if outBodyIndex != -1 {
			// Serialize output body
			body := vo.Field(outBodyIndex).Interface()

			ct, err := api.Negotiate(a.GetHeader(r, "Accept"))
			if err != nil {
				writeErr(api, &op, r, w, http.StatusNotAcceptable, "unable to marshal response", err)
			}
			if ctf, ok := body.(ContentTypeFilter); ok {
				ct = ctf.ContentType(ct)
			}

			body, err = api.TransformBody(&op, strconv.Itoa(op.DefaultStatus), body)
			if err != nil {
				// TODO: implement
				panic(err)
			}

			a.WriteHeader(w, "Content-Type", ct)
			if api.HasSchemaLink() {
				setLink(a, schemaLinks, defaultSchemaLink, op.DefaultStatus, w)
			}
			a.WriteStatus(w, op.DefaultStatus)
			api.Marshal(ct, a.BodyWriter(w), body)
		} else {
			a.WriteStatus(w, op.DefaultStatus)
		}
	})
}

func setLink[R, W any](a Adapter[R, W], schemaLinks map[int]string, defaultSchemaLink string, status int, w W) {
	if link := schemaLinks[status]; link != "" {
		a.AppendHeader(w, "Link", link)
	} else if defaultSchemaLink != "" {
		a.AppendHeader(w, "Link", defaultSchemaLink)
	}
}
