package huma

import (
	"net/http"
	"reflect"
	"strings"
)

// Resource describes a REST resource at a given URI path. Resources are
// typically created from a router or as a sub-resource of an existing resource.
type Resource struct {
	*openAPIOperation
	router *Router
	path   string
}

// NewResource creates a new resource with the given router and path. All
// dependencies, security requirements, params, headers, and responses are
// empty.
func NewResource(router *Router, path string, options ...ResourceOption) *Resource {
	r := &Resource{
		openAPIOperation: newOperation(),
		router:           router,
		path:             path,
	}

	for _, option := range options {
		option.ApplyResource(r)
	}

	return r
}

// Copy the resource. New arrays are created for dependencies, security
// requirements, params, response headers, and responses but the underlying
// pointer values themselves are the same.
func (r *Resource) Copy() *Resource {
	return &Resource{
		openAPIOperation: r.openAPIOperation.Copy(),
		router:           r.router,
		path:             r.path,
	}
}

// With returns a copy of this resource with the given dependencies, security
// requirements, params, response headers, or responses added to it.
func (r *Resource) With(options ...ResourceOption) *Resource {
	c := r.Copy()

	for _, option := range options {
		option.ApplyResource(c)
	}

	return c
}

// Path returns the generated path including any path parameters.
func (r *Resource) Path() string {
	generated := r.path

	for _, p := range r.params {
		if p.In == "path" {
			component := "{" + p.Name + "}"
			if !strings.Contains(generated, component) {
				if !strings.HasSuffix(generated, "/") {
					generated += "/"
				}
				generated += component
			}
		}
	}

	return generated
}

// PathParams returns the name of all path parameters.
func (r *Resource) PathParams() []string {
	params := make([]string, len(r.params))

	for i, p := range r.params {
		params[i] = p.Name
	}

	return params
}

// SubResource creates a new resource at the given path, which is appended
// to the existing resource path after adding any existing path parameters.
func (r *Resource) SubResource(path string, options ...ResourceOption) *Resource {
	// Apply all existing params to the path.
	newPath := r.Path()

	// Apply the new passed-in path component.
	if !strings.HasSuffix(newPath, "/") {
		newPath += "/"
	}
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	newPath += path

	// Clone the resource and update the path.
	c := r.With(options...)
	c.path = newPath

	return c
}

// Operation adds the operation to this resource's router with all the
// combined deps, security requirements, params, headers, responses, etc.
func (r *Resource) operation(method string, docs string, handler interface{}) {
	summary, desc := splitDocs(docs)

	// Copy the operation and set new fields.
	op := r.openAPIOperation.Copy()
	op.summary = summary
	op.description = desc

	op.handler = handler
	if op.handler != nil {
		// Only apply auto-response if it's *not* an unsafe handler.
		if !op.unsafe() {
			t := reflect.TypeOf(op.handler)
			if t.NumOut() == len(op.responseHeaders)+len(op.responses)+1 {
				rtype := t.Out(t.NumOut() - 1)
				switch rtype.Kind() {
				case reflect.Bool:
					op = op.With(Response(http.StatusNoContent, "Success"))
				case reflect.String:
					op = op.With(ResponseText(http.StatusOK, "Success"))
				default:
					op = op.With(ResponseJSON(http.StatusOK, "Success"))
				}
			}
		}
	}

	// Update path with any required path parameters if they are not yet present.
	allParams := append([]*openAPIParam{}, r.params...)
	allParams = append(allParams, op.params...)
	path := r.path
	for _, p := range allParams {
		if p.In == "path" {
			component := "{" + p.Name + "}"
			if !strings.Contains(path, component) {
				if !strings.HasSuffix(path, "/") {
					path += "/"
				}
				path += component
			}
		}
	}

	r.router.register(method, path, op)
}

// Head creates an HTTP HEAD operation on the resource.
func (r *Resource) Head(docs string, handler interface{}) {
	r.operation(http.MethodHead, docs, handler)
}

// List is an alias for `Get`.
func (r *Resource) List(docs string, handler interface{}) {
	r.Get(docs, handler)
}

// Get creates an HTTP GET operation on the resource.
func (r *Resource) Get(docs string, handler interface{}) {
	r.operation(http.MethodGet, docs, handler)
}

// Post creates an HTTP POST operation on the resource.
func (r *Resource) Post(docs string, handler interface{}) {
	r.operation(http.MethodPost, docs, handler)
}

// Put creates an HTTP PUT operation on the resource.
func (r *Resource) Put(docs string, handler interface{}) {
	r.operation(http.MethodPut, docs, handler)
}

// Patch creates an HTTP PATCH operation on the resource.
func (r *Resource) Patch(docs string, handler interface{}) {
	r.operation(http.MethodPatch, docs, handler)
}

// Delete creates an HTTP DELETE operation on the resource.
func (r *Resource) Delete(docs string, handler interface{}) {
	r.operation(http.MethodDelete, docs, handler)
}

// Options creates an HTTP OPTIONS operation on the resource.
func (r *Resource) Options(docs string, handler interface{}) {
	r.operation(http.MethodOptions, docs, handler)
}
