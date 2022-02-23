package huma

import (
	"net/http"
	"strings"

	"github.com/Jeffail/gabs/v2"
	"github.com/go-chi/chi"
)

// Resource represents an API resource attached to a router at a specific path
// (URI template). Resources can have operations or subresources attached to
// them.
type Resource struct {
	path   string
	mux    chi.Router
	router *Router

	subResources []*Resource
	operations   []*Operation

	tags []string
}

func (r *Resource) toOpenAPI(components *oaComponents) *gabs.Container {
	doc := gabs.New()

	for _, sub := range r.subResources {
		doc.Merge(sub.toOpenAPI(components))
	}

	for _, op := range r.operations {
		opValue := op.toOpenAPI(components)

		if len(r.tags) > 0 {
			opValue.Set(r.tags, "tags")
		}

		doc.Set(opValue, r.path, strings.ToLower(op.method))
	}

	return doc
}

// Operation creates a new HTTP operation with the given method at this resource.
func (r *Resource) Operation(method, operationID, docs string, responses ...Response) *Operation {
	op := newOperation(r, method, operationID, docs, responses)
	r.operations = append(r.operations, op)

	return op
}

// Post creates a new HTTP POST operation at this resource.
func (r *Resource) Post(operationID, docs string, responses ...Response) *Operation {
	return r.Operation(http.MethodPost, operationID, docs, responses...)
}

// Head creates a new HTTP HEAD operation at this resource.
func (r *Resource) Head(operationID, docs string, responses ...Response) *Operation {
	return r.Operation(http.MethodHead, operationID, docs, responses...)
}

// Get creates a new HTTP GET operation at this resource.
func (r *Resource) Get(operationID, docs string, responses ...Response) *Operation {
	return r.Operation(http.MethodGet, operationID, docs, responses...)
}

// Put creates a new HTTP PUT operation at this resource.
func (r *Resource) Put(operationID, docs string, responses ...Response) *Operation {
	return r.Operation(http.MethodPut, operationID, docs, responses...)
}

// Patch creates a new HTTP PATCH operation at this resource.
func (r *Resource) Patch(operationID, docs string, responses ...Response) *Operation {
	return r.Operation(http.MethodPatch, operationID, docs, responses...)
}

// Delete creates a new HTTP DELETE operation at this resource.
func (r *Resource) Delete(operationID, docs string, responses ...Response) *Operation {
	return r.Operation(http.MethodDelete, operationID, docs, responses...)
}

// Middleware adds a new standard middleware to this resource, so it will
// apply to requests at the resource's path (including any subresources).
// Middleware can also be applied at the router level to apply to all requests.
func (r *Resource) Middleware(middlewares ...func(next http.Handler) http.Handler) {
	r.mux.Use(middlewares...)
}

// SubResource creates a new resource attached to this resource. The passed
// path will be appended to the resource's existing path. The path can
// include parameters, e.g. `/things/{thing-id}`. Each resource path must
// be unique.
func (r *Resource) SubResource(path string) *Resource {
	sub := &Resource{
		path:         r.path + path,
		mux:          r.mux.Route(path, nil),
		router:       r.router,
		subResources: []*Resource{},
		operations:   []*Operation{},
		tags:         append([]string{}, r.tags...),
	}

	r.subResources = append(r.subResources, sub)

	return sub
}

// Tags appends to the list of tags, used for documentation.
func (r *Resource) Tags(names ...string) {
	r.tags = append(r.tags, names...)
}
