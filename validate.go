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

// ErrContextNotFirst is returned when a registered operation has a handler
// that takes a context but it is not the first parameter of the function.
var ErrContextNotFirst = errors.New("context should be first parameter")

// ErrParamsMustMatch is returned when a registered operation has a handler
// function that takes the wrong number of arguments.
var ErrParamsMustMatch = errors.New("handler function args must match registered params")

// ErrResponsesMustMatch is returned when the registered operation has a handler
// function that returns the wrong number of arguments.
var ErrResponsesMustMatch = errors.New("handler function return values must match registered responses")

var paramRe = regexp.MustCompile(`:([^/]+)|{([^}]+)}`)

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
		if method.Type().NumOut() > 1 {
			k := method.Type().Out(1).Kind()
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

	types := []reflect.Type{}
	for i := 0; i < method.Type().NumIn(); i++ {
		paramType := method.Type().In(i)

		if paramType.String() == "*gin.Context" {
			// Skip context parameter
			if i != 0 {
				return ErrContextNotFirst
			}
			continue
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
			// The last item has no associated param.
			s, err := GenerateSchema(paramType)
			if err != nil {
				return err
			}
			o.RequestSchema = s
			continue
		}

		p := o.Params[i]
		p.typ = paramType
		if p.Schema == nil {
			// Auto-generate a schema for this parameter
			s, err := GenerateSchema(paramType)
			if err != nil {
				return err
			}
			p.Schema = s
		}
	}

	// Check that outputs match registered responses and add their type info
	if method.Type().NumOut() != len(o.Responses) {
		return ErrResponsesMustMatch
	}

	for i, resp := range o.Responses {
		respType := method.Type().Out(i)
		// HTTP 204 explicitly forbids a response body.
		if resp.StatusCode != 204 && resp.Schema == nil {
			s, err := GenerateSchema(respType)
			if err != nil {
				return err
			}
			resp.Schema = s
		}
	}

	return nil
}
