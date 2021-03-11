package huma

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/istreamlabs/huma/schema"
)

// Operation represents an operation (an HTTP verb, e.g. GET / PUT) against
// a resource attached to a router.
type Operation struct {
	resource           *Resource
	method             string
	id                 string
	summary            string
	description        string
	params             map[string]oaParam
	requestContentType string
	requestSchema      *schema.Schema
	requestModel       reflect.Type
	responses          []Response
	maxBodyBytes       int64
	bodyReadTimeout    time.Duration
}

func newOperation(resource *Resource, method, id, docs string, responses []Response) *Operation {
	summary, desc := splitDocs(docs)
	return &Operation{
		resource:    resource,
		method:      method,
		id:          id,
		summary:     summary,
		description: desc,
		responses:   responses,
		// 1 MiB body limit by default
		maxBodyBytes: 1024 * 1024,
		// 15 second timeout by default
		bodyReadTimeout: 15 * time.Second,
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

	// Request params
	for _, param := range o.params {
		if param.Internal {
			// Skip documenting internal-only params.
			continue
		}

		doc.ArrayAppend(param, "parameters")
	}

	// Request body
	if o.requestSchema != nil {
		ct := o.requestContentType
		if ct == "" {
			ct = "application/json"
		}
		ref := components.AddSchema(o.requestModel, schema.ModeAll, o.id+"-request")
		doc.Set(ref, "requestBody", "content", ct, "schema", "$ref")
	}

	// responses
	for _, resp := range o.responses {
		status := fmt.Sprintf("%v", resp.status)
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
			ref := components.AddSchema(resp.model, schema.ModeAll, o.id+"-response")
			doc.Set(ref, "responses", status, "content", resp.contentType, "schema", "$ref")
		}
	}

	return doc
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
	o.requestSchema = s
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
		var err error
		input := t.In(1)

		// Get parameters
		o.params = getParamInfo(input)
		for k, v := range o.params {
			if v.In == inPath {
				// Confirm each declared input struct path parameter is actually a part
				// of the declared resource path.
				if !strings.Contains(o.resource.path, "{"+k+"}") {
					panic(fmt.Errorf("Parameter '%s' not in URI path: %s", k, o.resource.path))
				}
			}
		}

		// Get body if present.
		if body, ok := input.FieldByName("Body"); ok {
			o.requestModel = body.Type

			if o.requestSchema == nil {
				o.requestSchema, err = schema.GenerateWithMode(body.Type, schema.ModeWrite, nil)
				if err != nil {
					panic(fmt.Errorf("unable to generate JSON schema: %w", err))
				}
			}
		}

		// It's possible for the inputs to generate a 400, so add it if it wasn't
		// explicitly defined.
		found400 := false
		for _, r := range o.responses {
			if r.status == http.StatusBadRequest {
				found400 = true
				break
			}
		}

		if !found400 {
			o.responses = append(o.responses, NewResponse(http.StatusBadRequest, http.StatusText(http.StatusBadRequest)).Model(&ErrorModel{}))
		}
	}

	// Future improvement idea: use a sync.Pool for the input structure to save
	// on allocations if the struct has a Reset() method.

	register("/", func(w http.ResponseWriter, r *http.Request) {
		// Limit the request body size and set a read timeout.
		if r.Body != nil {
			if o.maxBodyBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, o.maxBodyBytes)
			}

			if conn := GetConn(r.Context()); o.bodyReadTimeout > 0 && conn != nil {
				conn.SetReadDeadline(time.Now().Add(o.bodyReadTimeout))
			}
		}

		ctx := &hcontext{
			Context:        r.Context(),
			ResponseWriter: w,
			r:              r,
			op:             o,
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

		setFields(ctx, ctx.r, input, inputType)
		resolveFields(ctx, "", input)
		if ctx.HasError() {
			ctx.WriteError(http.StatusBadRequest, "Error while parsing input parameters")
			return
		}

		// Call the handler with the context and newly populated input struct.
		in := []reflect.Value{reflect.ValueOf(ctx), input.Elem()}
		reflect.ValueOf(handler).Call(in)
	})
}
