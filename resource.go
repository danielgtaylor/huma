package huma

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// Resource describes a REST resource at a given URI path. Resources are
// typically created from a router or as a sub-resource of an existing resource.
type Resource struct {
	router          *Router
	path            string
	deps            []*Dependency
	security        []SecurityRequirement
	params          []*Param
	responseHeaders []*ResponseHeader
	responses       []*Response
	maxBodyBytes    int64
}

// NewResource creates a new resource with the given router and path. All
// dependencies, security requirements, params, headers, and responses are
// empty.
func NewResource(router *Router, path string) *Resource {
	return &Resource{
		router:          router,
		path:            path,
		deps:            make([]*Dependency, 0),
		security:        make([]SecurityRequirement, 0),
		params:          make([]*Param, 0),
		responseHeaders: make([]*ResponseHeader, 0),
		responses:       make([]*Response, 0),
	}
}

// Copy the resource. New arrays are created for dependencies, security
// requirements, params, response headers, and responses but the underlying
// pointer values themselves are the same.
func (r *Resource) Copy() *Resource {
	return &Resource{
		router:          r.router,
		path:            r.path,
		deps:            append([]*Dependency{}, r.deps...),
		security:        append([]SecurityRequirement{}, r.security...),
		params:          append([]*Param{}, r.params...),
		responseHeaders: append([]*ResponseHeader{}, r.responseHeaders...),
		responses:       append([]*Response{}, r.responses...),
		maxBodyBytes:    r.maxBodyBytes,
	}
}

// With returns a copy of this resource with the given dependencies, security
// requirements, params, response headers, or responses added to it.
func (r *Resource) With(depsParamHeadersOrResponses ...interface{}) *Resource {
	c := r.Copy()

	// For each input, determine which type it is and store it.
	for _, dph := range depsParamHeadersOrResponses {
		switch v := dph.(type) {
		case *Dependency:
			c.deps = append(c.deps, v)
		case []SecurityRequirement:
			c.security = v
		case SecurityRequirement:
			c.security = append(c.security, v)
		case *Param:
			c.params = append(c.params, v)
		case *ResponseHeader:
			c.responseHeaders = append(c.responseHeaders, v)
		case *Response:
			c.responses = append(c.responses, v)
		default:
			panic(fmt.Errorf("unsupported type %v", v))
		}
	}

	return c
}

// MaxBodyBytes sets the max number of bytes read from a request body before
// the handler aborts and returns an error. Applies to all sub-resources.
func (r *Resource) MaxBodyBytes(value int64) *Resource {
	r.maxBodyBytes = value
	return r
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

// Operation adds the operation to this resource's router with all the
// combined deps, security requirements, params, headers, responses, etc.
func (r *Resource) Operation(method string, op *Operation) {
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

	if op.Handler != nil {
		t := reflect.TypeOf(op.Handler)
		if t.NumOut() == len(op.ResponseHeaders)+len(op.Responses)+1 {
			rtype := t.Out(t.NumOut() - 1)
			switch rtype.Kind() {
			case reflect.Bool:
				op.Responses = append(op.Responses, ResponseEmpty(http.StatusNoContent, "Success"))
			case reflect.String:
				op.Responses = append(op.Responses, ResponseText(http.StatusOK, "Success"))
			default:
				op.Responses = append(op.Responses, ResponseJSON(http.StatusOK, "Success"))
			}
		}
	}

	if op.MaxBodyBytes == 0 {
		op.MaxBodyBytes = r.maxBodyBytes
	}

	r.router.Register(method, path, op)
}

// Text is shorthand for `r.With(huma.ResponseText(...))`.
func (r *Resource) Text(statusCode int, description string, headers ...string) *Resource {
	return r.With(ResponseText(statusCode, description, headers...))
}

// JSON is shorthand for `r.With(huma.ResponseJSON(...))`.
func (r *Resource) JSON(statusCode int, description string, headers ...string) *Resource {
	return r.With(ResponseJSON(statusCode, description, headers...))
}

// NoContent is shorthand for `r.With(huma.ResponseEmpty(http.StatusNoContent, ...)`
func (r *Resource) NoContent(description string, headers ...string) *Resource {
	return r.With(ResponseEmpty(http.StatusNoContent, description, headers...))
}

// Empty is shorthand for `r.With(huma.ResponseEmpty(...))`.
func (r *Resource) Empty(statusCode int, description string, headers ...string) *Resource {
	return r.With(ResponseEmpty(statusCode, description, headers...))
}

// Head creates an HTTP HEAD operation on the resource.
func (r *Resource) Head(description string, handler interface{}) {
	r.Operation(http.MethodHead, &Operation{
		Description: description,
		Handler:     handler,
	})
}

// List is an alias for `Get`.
func (r *Resource) List(description string, handler interface{}) {
	r.Get(description, handler)
}

// Get creates an HTTP GET operation on the resource.
func (r *Resource) Get(description string, handler interface{}) {
	r.Operation(http.MethodGet, &Operation{
		Description: description,
		Handler:     handler,
	})
}

// Post creates an HTTP POST operation on the resource.
func (r *Resource) Post(description string, handler interface{}) {
	r.Operation(http.MethodPost, &Operation{
		Description: description,
		Handler:     handler,
	})
}

// Put creates an HTTP PUT operation on the resource.
func (r *Resource) Put(description string, handler interface{}) {
	r.Operation(http.MethodPut, &Operation{
		Description: description,
		Handler:     handler,
	})
}

// Patch creates an HTTP PATCH operation on the resource.
func (r *Resource) Patch(description string, handler interface{}) {
	r.Operation(http.MethodPatch, &Operation{
		Description: description,
		Handler:     handler,
	})
}

// Delete creates an HTTP DELETE operation on the resource.
func (r *Resource) Delete(description string, handler interface{}) {
	r.Operation(http.MethodDelete, &Operation{
		Description: description,
		Handler:     handler,
	})
}
