package huma

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/danielgtaylor/huma/schema"
)

// OperationInfo describes an operation. It contains useful information for
// logging, metrics, auditing, etc.
type OperationInfo struct {
	ID          string
	URITemplate string
	Summary     string
	Tags        []string
}

// GetOperationInfo returns information about the current Huma operation. This
// will only be populated *after* routing has been handled, meaning *after*
// `next.ServeHTTP(w, r)` has been called in your middleware.
func GetOperationInfo(ctx context.Context) *OperationInfo {
	if oi := ctx.Value(opIDContextKey); oi != nil {
		return oi.(*OperationInfo)
	}

	return &OperationInfo{
		ID:   "unknown",
		Tags: []string{},
	}
}

type request struct {
	override bool
	model    reflect.Type
	schema   *schema.Schema
}

// Operation represents an operation (an HTTP verb, e.g. GET / PUT) against
// a resource attached to a router.
type Operation struct {
	resource           *Resource
	method             string
	id                 string
	summary            string
	description        string
	params             map[string]oaParam
	paramsOrder        []string
	defaultContentType string
	requests           map[string]*request
	responses          []Response
	maxBodyBytes       int64
	bodyReadTimeout    time.Duration
	deprecated         bool
}

func newOperation(resource *Resource, method, id, docs string, responses []Response) *Operation {
	summary, desc := splitDocs(docs)
	return &Operation{
		resource:    resource,
		method:      method,
		id:          id,
		summary:     summary,
		description: desc,
		requests:    map[string]*request{},
		responses:   responses,
		// 1 MiB body limit by default
		maxBodyBytes: 1024 * 1024,
		// 15 second timeout by default
		bodyReadTimeout: resource.router.defaultBodyReadTimeout,
		deprecated:      false,
	}
}

func (o *Operation) toOpenAPI(components *oaComponents) *gabs.Container {
	doc := gabs.New()

	doc.Set(o.id, "operationId")
	if o.summary != "" {
		doc.Set(o.summary, "summary")
	}
	if o.description != "" {
		doc.Set(o.description, "description")
	}
	if o.deprecated {
		doc.Set(o.deprecated, "deprecated")
	}

	// Request params
	for _, paramKey := range o.paramsOrder {
		param := o.params[paramKey]
		if param.Internal {
			// Skip documenting internal-only params.
			continue
		}

		doc.ArrayAppend(param, "parameters")
	}

	// Request body
	for ct, request := range o.requests {
		ref := ""
		if request.override {
			ref = components.AddExistingSchema(request.schema, o.id+"-request", !o.resource.router.disableSchemaProperty)
		} else {
			// Regenerate with ModeAll so the same model can be used for both the
			// input and output when possible.
			ref = components.AddSchema(request.model, schema.ModeAll, o.id+"-request", !o.resource.router.disableSchemaProperty)
		}
		doc.Set(ref, "requestBody", "content", ct, "schema", "$ref")
	}

	// responses
	for i, resp := range o.responses {
		status := fmt.Sprintf("%v", resp.status)
		if resp.status == 0 {
			status = "default"
		}
		doc.Set(resp.description, "responses", status, "description")

		headers := resp.headers
		for _, name := range headers {
			// TODO: get header description from shared registry
			//header := headerMap[name]
			header := name
			doc.Set(header, "responses", status, "headers", name, "description")

			typ := "string"
			for _, param := range o.params {
				if param.In == inHeader && param.Name == name {
					if param.Schema.Type != "" {
						typ = param.Schema.Type
					}
					break
				}
			}
			doc.Set(typ, "responses", status, "headers", name, "schema", "type")
		}

		if resp.model != nil {
			ref := components.AddSchema(resp.model, schema.ModeAll, o.id+"-response", !o.resource.router.disableSchemaProperty)
			o.responses[i].modelRef = ref
			doc.Set(ref, "responses", status, "content", resp.contentType, "schema", "$ref")
		}
	}

	return doc
}

func (o *Operation) requestForContentType(ct string) (string, *request) {
	req := o.requests[ct]
	if req == nil {
		ct = o.defaultContentType
		req = o.requests[ct]
	}
	return ct, req
}

// MaxBodyBytes sets the max number of bytes that the request body size may be
// before the request is cancelled. The default is 1MiB.
func (o *Operation) MaxBodyBytes(size int64) {
	o.maxBodyBytes = size
}

// NoMaxBody removes the body byte limit, which is 1MiB by default. Use this
// if you expect to stream the input request or need to handle very large
// request bodies.
func (o *Operation) NoMaxBody() {
	o.maxBodyBytes = 0
}

// BodyReadTimeout sets the amount of time a request can spend reading the
// body, after which it times out and the request is cancelled. The default
// is 15 seconds.
func (o *Operation) BodyReadTimeout(duration time.Duration) {
	o.bodyReadTimeout = duration
}

// NoBodyReadTimeout removes the body read timeout, which is 15 seconds by
// default. Use this if you expect to stream the input request or need to
// handle very large request bodies.
func (o *Operation) NoBodyReadTimeout() {
	o.bodyReadTimeout = 0
}

// RequestSchema allows overriding the generated input body schema, giving you
// more control over documentation and validation.
func (o *Operation) RequestSchema(s *schema.Schema) {
	o.RequestSchemaForContentType("application/json", s)
}

// Deprecated marks the operation is deprecated, warning consumers should
// refrain from using this.
func (o *Operation) Deprecated() {
	o.deprecated = true
}

func (o *Operation) RequestSchemaForContentType(ct string, s *schema.Schema) {
	if o.requests[ct] == nil {
		o.requests[ct] = &request{}
	}
	o.requests[ct].override = true
	o.requests[ct].schema = s
}

// Run registers the handler function for this operation. It should be of the
// form: `func (ctx huma.Context)` or `func (ctx huma.Context, input)` where
// input is your input struct describing the input parameters and/or body.
func (o *Operation) Run(handler interface{}) {
	if reflect.ValueOf(handler).Kind() != reflect.Func {
		panic(fmt.Errorf("Handler must be a function taking a huma.Context and optionally a user-defined input struct, but got: %s for %s %s", handler, o.method, o.resource.path))
	}

	var register func(string, http.HandlerFunc)

	switch o.method {
	case http.MethodPost:
		register = o.resource.mux.Post
	case http.MethodHead:
		register = o.resource.mux.Head
	case http.MethodGet:
		register = o.resource.mux.Get
	case http.MethodPut:
		register = o.resource.mux.Put
	case http.MethodPatch:
		register = o.resource.mux.Patch
	case http.MethodDelete:
		register = o.resource.mux.Delete
	default:
		panic(fmt.Errorf("Unknown HTTP verb: %s", o.method))
	}

	t := reflect.TypeOf(handler)
	if t.Kind() == reflect.Func && t.NumIn() > 1 {
		input := t.In(1)

		// Get parameters
		o.params, o.paramsOrder = getParamInfo(input)
		for _, k := range o.paramsOrder {
			v := o.params[k]
			if v.In == inPath {
				// Confirm each declared input struct path parameter is actually a part
				// of the declared resource path.
				if !strings.Contains(o.resource.path, "{"+k+"}") {
					panic(fmt.Errorf("Parameter '%s' not in URI path: %s", k, o.resource.path))
				}
			}
		}

		possible := []int{http.StatusBadRequest}
		foundBody := false

		for i := 0; i < input.NumField(); i++ {
			f := input.Field(i)
			if ct, ok := f.Tag.Lookup(locationBody); ok || f.Name == strings.Title(locationBody) {
				foundBody = true

				if ct == "" || ct == "true" {
					// Default to JSON
					ct = "application/json"
				}

				if o.defaultContentType == "" {
					o.defaultContentType = ct
				}

				if o.requests[ct] == nil {
					o.requests[ct] = &request{}
				}

				o.requests[ct].model = f.Type

				if !o.requests[ct].override {
					nestedSchemas := map[string]schema.NestedSchemaReference{}
					s, err := schema.GenerateWithMode(f.Type, schema.ModeWrite, nil, nestedSchemas)
					if err != nil {
						panic(fmt.Errorf("unable to generate JSON schema: %w", err))
					}
					if o.resource != nil && o.resource.router != nil && !o.resource.router.disableSchemaProperty {
						s.AddSchemaField()
					}
					o.requests[ct].schema = s
				}
			}
		}

		if foundBody || len(o.params) > 0 {
			// Invalid parameter values or body values can cause a 422.
			possible = append(possible, http.StatusUnprocessableEntity)
		}

		if foundBody {
			possible = append(possible,
				http.StatusRequestEntityTooLarge,
				http.StatusRequestTimeout,
			)
		}

		// It's possible for the inputs to generate a few different errors, so
		// generate them if not already present.
		found := map[int]bool{}
		for _, r := range o.responses {
			found[r.status] = true
		}

		for _, s := range possible {
			if !found[s] {
				o.responses = append(o.responses, NewResponse(s, http.StatusText(s)).ContentType("application/problem+json").Model(&ErrorModel{}))
			}
		}
	}

	// Future improvement idea: use a sync.Pool for the input structure to save
	// on allocations if the struct has a Reset() method.

	register("/", func(w http.ResponseWriter, r *http.Request) {
		// Update the operation info for loggers/metrics/etc middlware to use later.
		opInfo := GetOperationInfo(r.Context())
		opInfo.ID = o.id
		opInfo.URITemplate = o.resource.path
		opInfo.Summary = o.summary
		opInfo.Tags = append([]string{}, o.resource.tags...)

		ctx := &hcontext{
			Context:               r.Context(),
			ResponseWriter:        w,
			r:                     r,
			op:                    o,
			docsPath:              o.resource.router.DocsPath(),
			schemasPath:           o.resource.router.SchemasPath(),
			specPath:              o.resource.router.OpenAPIPath(),
			urlPrefix:             o.resource.router.urlPrefix,
			disableSchemaProperty: o.resource.router.disableSchemaProperty,
			errorCode:             http.StatusBadRequest,
		}

		// If there is no input struct (just a context), then the call is simple.
		if simple, ok := handler.(func(Context)); ok {
			simple(ctx)
			return
		}

		// Otherwise, create a new input struct instance and populate it.
		v := reflect.ValueOf(handler)
		inputType := v.Type().In(1)
		input := reflect.New(inputType)

		// Limit the request body size.
		if r.Body != nil {
			if o.maxBodyBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, o.maxBodyBytes)
			}
		}

		ct, reqDef := o.requestForContentType(r.Header.Get("Content-Type"))

		// Set a read deadline for reading/parsing the input request body, but
		// only for operations that have a request body model.
		var conn net.Conn
		if reqDef != nil && reqDef.model != nil && o.bodyReadTimeout > 0 {
			if conn = GetConn(r.Context()); conn != nil {
				conn.SetReadDeadline(time.Now().Add(o.bodyReadTimeout))
			}
		}

		setFields(ctx, ctx.r, input, inputType, ct, reqDef)
		if !ctx.HasError() {
			// No errors yet, so any errors that come after should be treated as a
			// semantic rather than structural error.
			ctx.errorCode = http.StatusUnprocessableEntity
		}
		resolveFields(ctx, "", input)
		if ctx.HasError() {
			ctx.WriteError(ctx.errorCode, "Error while processing input parameters")
			return
		}

		// Clear any body read deadline if one was set as the body has now been
		// read in. The one exception is when the body is streamed in via an
		// `io.Reader` so we don't reset the deadline for that.
		if conn != nil && reqDef != nil && reqDef.model != readerType {
			conn.SetReadDeadline(time.Time{})
		}

		// Call the handler with the context and newly populated input struct.
		in := []reflect.Value{reflect.ValueOf(ctx), input.Elem()}
		reflect.ValueOf(handler).Call(in)
	})
}
