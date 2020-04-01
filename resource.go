package huma

import (
	"fmt"
	"net/http"
	"strings"
)

// Resource describes a REST resource at a given URI path. Resources are
// typically created from a router or as a sub-resource of an existing resource.
type Resource struct {
	router          *Router
	path            string
	deps            []*Dependency
	params          []*Param
	responseHeaders []*ResponseHeader
	responses       []*Response
}

// NewResource creates a new resource with the given router and path. All
// dependencies, params, headers, and responses are empty.
func NewResource(router *Router, path string) *Resource {
	return &Resource{
		router:          router,
		path:            path,
		deps:            make([]*Dependency, 0),
		params:          make([]*Param, 0),
		responseHeaders: make([]*ResponseHeader, 0),
		responses:       make([]*Response, 0),
	}
}

// Copy the resource. New arrays are created for dependencies, params, response
// headers, and responses but the underlying pointer values themselves are the
// same.
func (r *Resource) Copy() *Resource {
	return &Resource{
		router:          r.router,
		path:            r.path,
		deps:            append([]*Dependency{}, r.deps...),
		params:          append([]*Param{}, r.params...),
		responseHeaders: append([]*ResponseHeader{}, r.responseHeaders...),
		responses:       append([]*Response{}, r.responses...),
	}
}

// With returns a copy of this resource with the given dependencies, params,
// response headers, or responses added to it.
func (r *Resource) With(depsParamHeadersOrResponses ...interface{}) *Resource {
	c := r.Copy()

	// For each input, determine which type it is and store it.
	for _, dph := range depsParamHeadersOrResponses {
		switch v := dph.(type) {
		case *Dependency:
			c.deps = append(r.deps, v)
		case *Param:
			c.params = append(r.params, v)
		case *ResponseHeader:
			c.responseHeaders = append(r.responseHeaders, v)
		case *Response:
			c.responses = append(r.responses, v)
		default:
			panic(fmt.Errorf("unsupported type %v", v))
		}
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
func (r *Resource) SubResource(path string, depsParamHeadersOrResponses ...interface{}) *Resource {
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
	c := r.With(depsParamHeadersOrResponses...)
	c.path = newPath

	return c
}

// addOperation adds the operation to this resource's router with all the
// combined deps, params, headers, responses, etc.
func (r *Resource) addOperation(method string, op *Operation) {
	// Set params, etc
	allDeps := append([]*Dependency{}, r.deps...)
	allDeps = append(allDeps, op.Dependencies...)
	op.Dependencies = allDeps

	// Combine resource and operation params. Update path with any required
	// path parameters if they are not yet present.
	allParams := append([]*Param{}, r.params...)
	allParams = append(allParams, op.Params...)
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
	op.Params = allParams

	allHeaders := append([]*ResponseHeader{}, r.responseHeaders...)
	allHeaders = append(allHeaders, op.ResponseHeaders...)
	op.ResponseHeaders = allHeaders

	allResponses := append([]*Response{}, r.responses...)
	allResponses = append(allResponses, op.Responses...)
	op.Responses = allResponses

	r.router.Register(method, path, op)
}

// Head creates an HTTP HEAD operation on the resource.
func (r *Resource) Head(op *Operation) {
	r.addOperation(http.MethodHead, op)
}

// List is an alias for `Get`.
func (r *Resource) List(op *Operation) {
	r.Get(op)
}

// ListJSON is an alias for `GetJSON`.
func (r *Resource) ListJSON(statusCode int, description string, handler interface{}) {
	r.GetJSON(statusCode, description, handler)
}

// Get creates an HTTP GET operation on the resource.
func (r *Resource) Get(op *Operation) {
	r.addOperation(http.MethodGet, op)
}

// GetJSON is shorthand for adding an HTTP GET operation on the resource that
// sets a description and a JSON success response.
func (r *Resource) GetJSON(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodGet, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseJSON(statusCode, "Success"),
		},
		Handler: handler,
	})
}

// Post creates an HTTP POST operation on the resource.
func (r *Resource) Post(op *Operation) {
	r.addOperation(http.MethodPost, op)
}

// PostJSON is shorthand for adding an HTTP POST operation on the resource that
// sets a description and a JSON success response.
func (r *Resource) PostJSON(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodPost, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseJSON(statusCode, "Success"),
		},
		Handler: handler,
	})
}

// PostNoContent is shorthand for adding an HTTP POST operation to the resource
// that sets a description and an empty response.
func (r *Resource) PostNoContent(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodPost, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseEmpty(statusCode, "Success"),
		},
		Handler: handler,
	})
}

// Put creates an HTTP PUT operation on the resource.
func (r *Resource) Put(op *Operation) {
	r.addOperation(http.MethodPut, op)
}

// PutJSON is shorthand for adding an HTTP PUT operation on the resource that
// sets a description and a JSON success response.
func (r *Resource) PutJSON(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodPut, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseJSON(statusCode, "Success"),
		},
		Handler: handler,
	})
}

// PutNoContent is shorthand for adding an HTTP PUT operation to the resource
// that sets a description and an empty response.
func (r *Resource) PutNoContent(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodPut, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseEmpty(statusCode, "Success"),
		},
		Handler: handler,
	})
}

// Patch creates an HTTP PATCH operation on the resource.
func (r *Resource) Patch(op *Operation) {
	r.addOperation(http.MethodPatch, op)
}

// PatchJSON is shorthand for adding an HTTP PATCH operation on the resource that
// sets a description and a JSON success response.
func (r *Resource) PatchJSON(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodPatch, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseJSON(statusCode, "Success"),
		},
		Handler: handler,
	})
}

// PatchNoContent is shorthand for adding an HTTP PATCH operation to the
// resource that sets a description and an empty response.
func (r *Resource) PatchNoContent(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodPatch, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseEmpty(statusCode, "Success"),
		},
		Handler: handler,
	})
}

// Delete creates an HTTP DELETE operation on the resource.
func (r *Resource) Delete(op *Operation) {
	r.addOperation(http.MethodDelete, op)
}

// DeleteNoContent is shorthand for adding an HTTP DELETE operation to the
// resource that sets a description and an empty response.
func (r *Resource) DeleteNoContent(statusCode int, description string, handler interface{}) {
	r.addOperation(http.MethodDelete, &Operation{
		Description: description,
		Responses: []*Response{
			ResponseEmpty(statusCode, "Success"),
		},
		Handler: handler,
	})
}
