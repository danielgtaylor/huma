package huma

import (
	"net/http"
	"reflect"
	"strings"
)

// Resource describes a REST resource at a given URI path. Resources are
// typically created from a router or as a sub-resource of an existing resource.
type Resource struct {
	*OpenAPIOperation
	router *Router
	path   string
}

// NewResource creates a new resource with the given router and path. All
// dependencies, security requirements, params, headers, and responses are
// empty.
func NewResource(router *Router, path string, options ...ResourceOption) *Resource {
	r := &Resource{
		OpenAPIOperation: NewOperation(),
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
		OpenAPIOperation: r.OpenAPIOperation.Copy(),
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
func (r *Resource) operation(method string, op *OpenAPIOperation) {
	// Set params, etc
	allDeps := append([]*OpenAPIDependency{}, r.dependencies...)
	allDeps = append(allDeps, op.dependencies...)
	op.dependencies = allDeps

	// Combine resource and operation params. Update path with any required
	// path parameters if they are not yet present.
	allParams := append([]*OpenAPIParam{}, r.params...)
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
	op.params = allParams

	allHeaders := append([]*OpenAPIResponseHeader{}, r.responseHeaders...)
	allHeaders = append(allHeaders, op.responseHeaders...)
	op.responseHeaders = allHeaders

	allResponses := append([]*OpenAPIResponse{}, r.responses...)
	allResponses = append(allResponses, op.responses...)
	op.responses = allResponses

	if op.handler != nil {
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

	if op.maxBodyBytes == 0 {
		op.maxBodyBytes = r.maxBodyBytes
	}

	if op.bodyReadTimeout == 0 {
		op.bodyReadTimeout = r.bodyReadTimeout
	}

	r.router.Register(method, path, op)
}

// Head creates an HTTP HEAD operation on the resource.
func (r *Resource) Head(description string, handler interface{}) {
	r.operation(http.MethodHead, &OpenAPIOperation{
		description:       description,
		OpenAPIDependency: &OpenAPIDependency{handler: handler},
	})
}

// List is an alias for `Get`.
func (r *Resource) List(description string, handler interface{}) {
	r.Get(description, handler)
}

// Get creates an HTTP GET operation on the resource.
func (r *Resource) Get(description string, handler interface{}) {
	r.operation(http.MethodGet, &OpenAPIOperation{
		description:       description,
		OpenAPIDependency: &OpenAPIDependency{handler: handler},
	})
}

// Post creates an HTTP POST operation on the resource.
func (r *Resource) Post(description string, handler interface{}) {
	r.operation(http.MethodPost, &OpenAPIOperation{
		description:       description,
		OpenAPIDependency: &OpenAPIDependency{handler: handler},
	})
}

// Put creates an HTTP PUT operation on the resource.
func (r *Resource) Put(description string, handler interface{}) {
	r.operation(http.MethodPut, &OpenAPIOperation{
		description:       description,
		OpenAPIDependency: &OpenAPIDependency{handler: handler},
	})
}

// Patch creates an HTTP PATCH operation on the resource.
func (r *Resource) Patch(description string, handler interface{}) {
	r.operation(http.MethodPatch, &OpenAPIOperation{
		description:       description,
		OpenAPIDependency: &OpenAPIDependency{handler: handler},
	})
}

// Delete creates an HTTP DELETE operation on the resource.
func (r *Resource) Delete(description string, handler interface{}) {
	r.operation(http.MethodDelete, &OpenAPIOperation{
		description:       description,
		OpenAPIDependency: &OpenAPIDependency{handler: handler},
	})
}
