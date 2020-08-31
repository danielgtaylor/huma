package huma

import (
	"fmt"
	"net/http"
	"reflect"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/danielgtaylor/huma/schema"
)

// Operation represents an operation (an HTTP verb, e.g. GET / PUT) against
// a resource attached to a router.
type Operation struct {
	resource           *Resource
	method             string
	id                 string
	description        string
	params             map[string]oaParam
	requestContentType string
	requestSchema      *schema.Schema
	responses          []Response
	maxBodyBytes       int64
	bodyReadTimeout    time.Duration
}

func newOperation(resource *Resource, method, id, description string, responses []Response) *Operation {
	return &Operation{
		resource:    resource,
		method:      method,
		id:          id,
		description: description,
		responses:   responses,
		// 1 MiB body limit by default
		maxBodyBytes: 1024 * 1024,
		// 15 second timeout by default
		bodyReadTimeout: 15 * time.Second,
	}
}

func (o *Operation) toOpenAPI() *gabs.Container {
	doc := gabs.New()

	doc.SetP(o.id, "operationId")
	if o.description != "" {
		doc.SetP(o.description, "description")
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
		doc.Set(o.requestSchema, "requestBody", "content", ct, "schema")
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
			doc.Set(header, "responses", status, "headers", name)
		}

		if resp.model != nil {
			schema, err := schema.GenerateWithMode(resp.model, schema.ModeRead, nil)
			if err != nil {
				panic(err)
			}
			doc.Set(schema, "responses", status, "content", resp.contentType, "schema")
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

// Run registers the handler function for this operation. It should be of the
// form: `func (ctx huma.Context)` or `func (ctx huma.Context, input)` where
// input is your input struct describing the input parameters and/or body.
func (o *Operation) Run(handler interface{}) {
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
	}

	t := reflect.TypeOf(handler)
	if t.Kind() == reflect.Func && t.NumIn() > 1 {
		var err error
		input := t.In(1)

		// Get parameters
		o.params = getParamInfo(input)

		// Get body if present.
		if body, ok := input.FieldByName("Body"); ok {
			o.requestSchema, err = schema.GenerateWithMode(body.Type, schema.ModeWrite, nil)
			if err != nil {
				panic(fmt.Errorf("unable to generate JSON schema: %w", err))
			}
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

		callHandler(ctx, handler)
	})
}
