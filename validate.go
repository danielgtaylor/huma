package huma

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/gosimple/slug"
)

// ErrFieldRequired is returned when a field is blank but has been required.
var ErrFieldRequired = errors.New("field is required")

// ErrParamsMustMatch is returned when a registered operation has a handler
// function that takes the wrong number of arguments.
var ErrParamsMustMatch = errors.New("handler function args must match registered params")

// ErrParamTypeMustMatch is returned when the parameter and its
// default value's type don't match.
var ErrParamTypeMustMatch = errors.New("param and default types must match")

// ErrResponsesMustMatch is returned when the registered operation has a handler
// function that returns the wrong number of arguments.
var ErrResponsesMustMatch = errors.New("handler function return values must match registered responses & headers")

var paramRe = regexp.MustCompile(`:([^/]+)|{([^}]+)}`)

func validateParam(p *Param, t reflect.Type) error {
	p.typ = t

	if p.Schema == nil {
		s, err := GenerateSchema(p.typ)
		if err != nil {
			return err
		}
		p.Schema = s

		if p.def != nil {
			p.Schema.Default = p.def
		}
	}

	return nil
}

func validateHeader(h *Header, t reflect.Type) error {
	if h.Schema == nil {
		// Generate the schema from the handler function types.
		s, err := GenerateSchema(t)
		if err != nil {
			return err
		}
		h.Schema = s
	}

	return nil
}

// validate checks that the operation is well-formed (e.g. handler signature
// matches the given params) and generates schemas if needed.
func (o *Operation) validate() error {
	if o.Method == "" {
		return fmt.Errorf("Method: %w", ErrFieldRequired)
	}

	if o.Path == "" {
		return fmt.Errorf("Path: %w", ErrFieldRequired)
	}

	if o.Description == "" {
		return fmt.Errorf("Description: %w", ErrFieldRequired)
	}

	method := reflect.ValueOf(o.Handler)

	if o.ID == "" {
		verb := o.Method

		// Try to detect calls returning lists of things.
		if method.Type().NumOut() > 0 {
			k := method.Type().Out(0).Kind()
			if k == reflect.Array || k == reflect.Slice {
				verb = "list"
			}
		}

		// Remove variables from path so they aren't in the generated name.
		path := paramRe.ReplaceAllString(o.Path, "")

		o.ID = slug.Make(verb + path)
	}

	if strings.Contains(o.Path, "{") {
		// Convert from OpenAPI-style parameters to gin-style params
		o.Path = paramRe.ReplaceAllString(o.Path, ":$1$2")
	}

	for i, dep := range o.Depends {
		paramType := method.Type().In(i)

		if paramType.String() == "gin.Context" {
			return fmt.Errorf("gin.Context should be pointer *gin.Context: %w", ErrDependencyInvalid)
		}

		if paramType.String() == "huma.Operation" {
			return fmt.Errorf("huma.Operation should be pointer *huma.Operation: %w", ErrDependencyInvalid)
		}

		if err := dep.validate(paramType); err != nil {
			return err
		}
	}

	types := []reflect.Type{}
	for i := len(o.Depends); i < method.Type().NumIn(); i++ {
		paramType := method.Type().In(i)

		if paramType.String() == "gin.Context" {
			return fmt.Errorf("gin.Context should be pointer *gin.Context: %w", ErrDependencyInvalid)
		}

		if paramType.String() == "huma.Operation" {
			return fmt.Errorf("huma.Operation should be pointer *huma.Operation: %w", ErrDependencyInvalid)
		}

		types = append(types, paramType)
	}

	if len(types) < len(o.Params) {
		// Example: handler function takes 3 params, but 5 are described.
		return ErrParamsMustMatch
	}

	requestBody := false
	if len(types) == len(o.Params)+1 {
		requestBody = true
	} else if len(types) != len(o.Params) {
		// Example: handler function takes 5 params, but 3 are described.
		return ErrParamsMustMatch
	}

	for i, paramType := range types {
		if i == len(types)-1 && requestBody {
			// The last item has no associated param. It is a request body.
			if o.RequestSchema == nil {
				s, err := GenerateSchema(paramType)
				if err != nil {
					return err
				}
				o.RequestSchema = s
			}
			continue
		}

		p := o.Params[i]
		if err := validateParam(p, paramType); err != nil {
			return err
		}
	}

	// Check that outputs match registered responses and add their type info
	numOut := method.Type().NumOut()
	if numOut != len(o.Responses)+len(o.ResponseHeaders) {
		return ErrResponsesMustMatch
	}

	for i, header := range o.ResponseHeaders {
		if err := validateHeader(header, method.Type().Out(i)); err != nil {
			return err
		}
	}

	for i, resp := range o.Responses {
		respType := method.Type().Out(len(o.ResponseHeaders) + i)
		// HTTP 204 explicitly forbids a response body.
		if resp.StatusCode != 204 && resp.Schema == nil {
			// Generate the schema from the handler function types.
			s, err := GenerateSchema(respType)
			if err != nil {
				return err
			}
			resp.Schema = s
		}
	}

	return nil
}
