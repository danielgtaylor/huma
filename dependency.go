package huma

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/gin-gonic/gin"
)

// ErrDependencyInvalid is returned when registering a dependency fails.
var ErrDependencyInvalid = errors.New("dependency invalid")

// OpenAPIDependency represents a handler function dependency and its associated
// inputs and outputs. Value can be either a struct pointer (global dependency)
// or a `func(dependencies, params) (headers, struct pointer, error)` style
// function.
type OpenAPIDependency struct {
	dependencies    []*OpenAPIDependency
	params          []*OpenAPIParam
	responseHeaders []*OpenAPIResponseHeader
	handler         interface{}
}

// Dependencies returns the dependencies associated with this dependency.
func (d *OpenAPIDependency) Dependencies() []*OpenAPIDependency {
	return d.dependencies
}

// Params returns the params associated with this dependency.
func (d *OpenAPIDependency) Params() []*OpenAPIParam {
	return d.params
}

// ResponseHeaders returns the params associated with this dependency.
func (d *OpenAPIDependency) ResponseHeaders() []*OpenAPIResponseHeader {
	return d.responseHeaders
}

// NewSimpleDependency returns a dependency with a function or value.
func NewSimpleDependency(value interface{}) *OpenAPIDependency {
	return NewDependency(nil, value)
}

// NewDependency returns a dependency with the given option and a handler
// function.
func NewDependency(option DependencyOption, handler interface{}) *OpenAPIDependency {
	d := &OpenAPIDependency{
		dependencies:    make([]*OpenAPIDependency, 0),
		params:          make([]*OpenAPIParam, 0),
		responseHeaders: make([]*OpenAPIResponseHeader, 0),
		handler:         handler,
	}

	if option != nil {
		option.ApplyDependency(d)
	}

	return d
}

var contextDependency OpenAPIDependency
var ginContextDependency OpenAPIDependency
var operationDependency OpenAPIDependency

// ContextDependency returns a dependency for the current request's
// `context.Context`. This is useful for timeouts & cancellation.
func ContextDependency() DependencyOption {
	return &dependencyOption{func(d *OpenAPIDependency) {
		d.dependencies = append(d.dependencies, &contextDependency)
	}}
}

// GinContextDependency returns a dependency for the current request's
// `*gin.Context`.
func GinContextDependency() DependencyOption {
	return &dependencyOption{func(d *OpenAPIDependency) {
		d.dependencies = append(d.dependencies, &ginContextDependency)
	}}
}

// OperationDependency returns a dependency  for the current `*huma.Operation`.
func OperationDependency() DependencyOption {
	return &dependencyOption{func(d *OpenAPIDependency) {
		d.dependencies = append(d.dependencies, &operationDependency)
	}}
}

// validate that the dependency deps/params/headers match the function
// signature or that the value is not a function.
func (d *OpenAPIDependency) validate(returnType reflect.Type) {
	if d == &contextDependency || d == &ginContextDependency || d == &operationDependency {
		// Hard-coded known dependencies. These are special and have no value.
		return
	}

	if d.handler == nil {
		panic(fmt.Errorf("handler must be set: %w", ErrDependencyInvalid))
	}

	v := reflect.ValueOf(d.handler)

	if v.Kind() != reflect.Func {
		if returnType != nil && returnType != v.Type() {
			panic(fmt.Errorf("return type should be %s but got %s: %w", v.Type(), returnType, ErrDependencyInvalid))
		}

		// This is just a static value. It shouldn't have params/headers/etc.
		if len(d.params) > 0 {
			panic(fmt.Errorf("global dependency should not have params: %w", ErrDependencyInvalid))
		}

		if len(d.responseHeaders) > 0 {
			panic(fmt.Errorf("global dependency should not set headers: %w", ErrDependencyInvalid))
		}

		return
	}

	fn := v.Type()
	lenArgs := len(d.dependencies) + len(d.params)
	if fn.NumIn() != lenArgs {
		// TODO: generate suggested func signature
		panic(fmt.Errorf("function signature should have %d args but got %s: %w", lenArgs, fn, ErrDependencyInvalid))
	}

	for _, dep := range d.dependencies {
		dep.validate(nil)
	}

	for i, p := range d.params {
		p.validate(fn.In(len(d.dependencies) + i))
	}

	lenReturn := len(d.responseHeaders) + 2
	if fn.NumOut() != lenReturn {
		panic(fmt.Errorf("function should return %d values but got %d: %w", lenReturn, fn.NumOut(), ErrDependencyInvalid))
	}

	for i, h := range d.responseHeaders {
		h.validate(fn.Out(i))
	}
}

// allParams returns all parameters for all dependencies in the graph of this
// dependency in depth-first order without duplicates.
func (d *OpenAPIDependency) allParams() []*OpenAPIParam {
	params := []*OpenAPIParam{}
	seen := map[*OpenAPIParam]bool{}

	for _, p := range d.params {
		seen[p] = true
		params = append(params, p)
	}

	for _, d := range d.dependencies {
		for _, p := range d.allParams() {
			if _, ok := seen[p]; !ok {
				seen[p] = true

				params = append(params, p)
			}
		}
	}

	return params
}

// allResponseHeaders returns all response headers for all dependencies in
// the graph of this dependency in depth-first order without duplicates.
func (d *OpenAPIDependency) allResponseHeaders() []*OpenAPIResponseHeader {
	headers := []*OpenAPIResponseHeader{}
	seen := map[*OpenAPIResponseHeader]bool{}

	for _, h := range d.responseHeaders {
		seen[h] = true
		headers = append(headers, h)
	}

	for _, d := range d.dependencies {
		for _, h := range d.allResponseHeaders() {
			if _, ok := seen[h]; !ok {
				seen[h] = true

				headers = append(headers, h)
			}
		}
	}

	return headers
}

// resolve the value of the dependency. Returns (response headers, value, error).
func (d *OpenAPIDependency) resolve(c *gin.Context, op *OpenAPIOperation) (map[string]string, interface{}, error) {
	// Identity dependencies are first. Just return if it's one of them.
	if d == &contextDependency {
		return nil, c.Request.Context(), nil
	}

	if d == &ginContextDependency {
		return nil, c, nil
	}

	if d == &operationDependency {
		return nil, op, nil
	}

	v := reflect.ValueOf(d.handler)
	if v.Kind() != reflect.Func {
		// Not a function, just return the global value.
		return nil, d.handler, nil
	}

	// Generate the input arguments
	in := make([]reflect.Value, 0, v.Type().NumIn())
	headers := map[string]string{}

	// Resolve each sub-dependency
	for _, dep := range d.dependencies {
		dHeaders, dVal, err := dep.resolve(c, op)
		if err != nil {
			return nil, nil, err
		}

		for h, hv := range dHeaders {
			headers[h] = hv
		}

		in = append(in, reflect.ValueOf(dVal))
	}

	// Get each input parameter
	for _, param := range d.params {
		v, ok := getParamValue(c, param)
		if !ok {
			return nil, nil, fmt.Errorf("could not get param value")
		}

		in = append(in, reflect.ValueOf(v))
	}

	// Call the function.
	out := v.Call(in)

	if last := out[len(out)-1]; !last.IsNil() {
		// There was an error!
		return nil, nil, last.Interface().(error)
	}

	// Get the headers & response value.
	for i, h := range d.responseHeaders {
		headers[h.Name] = out[i].Interface().(string)
	}

	return headers, out[len(d.responseHeaders)].Interface(), nil
}
