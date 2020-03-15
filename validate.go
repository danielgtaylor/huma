package huma

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/gosimple/slug"
)

// ErrOperationInvalid is returned when validating an operation has failed.
var ErrOperationInvalid = errors.New("invalid operation")

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
		return fmt.Errorf("method field required: %w", ErrOperationInvalid)
	}

	if o.Path == "" {
		return fmt.Errorf("path field required: %w", ErrOperationInvalid)
	}

	if o.Description == "" {
		return fmt.Errorf("description field required: %w", ErrOperationInvalid)
	}

	if len(o.Responses) == 0 {
		return fmt.Errorf("at least one response is required: %w", ErrOperationInvalid)
	}

	method := reflect.ValueOf(o.Handler).Type()

	totalIn := len(o.Depends) + len(o.Params)
	totalOut := len(o.ResponseHeaders) + len(o.Responses)
	if !(method.NumIn() == totalIn || (o.Method != http.MethodGet && method.NumIn() == totalIn+1) || method.NumOut() != totalOut) {
		expected := "func("
		for _, dep := range o.Depends {
			expected += "? " + reflect.ValueOf(dep.Value).Type().String() + ", "
		}
		for _, param := range o.Params {
			expected += param.Name + " ?, "
		}
		expected = strings.TrimRight(expected, ", ")
		expected += ") ("
		for _, h := range o.ResponseHeaders {
			expected += h.Name + " ?, "
		}
		for _, r := range o.Responses {
			expected += fmt.Sprintf("*Response%d, ", r.StatusCode)
		}
		expected = strings.TrimRight(expected, ", ")
		expected += ")"

		fmt.Printf("%d in, %d out expected, found %d, %d", totalIn, totalOut, method.NumIn(), method.NumOut())

		return fmt.Errorf("expected %s but found %s: %w", expected, method, ErrOperationInvalid)
	}

	if o.ID == "" {
		verb := o.Method

		// Try to detect calls returning lists of things.
		if method.NumOut() > 0 {
			k := method.Out(0).Kind()
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
		paramType := method.In(i)

		// Catch common errors.
		if paramType.String() == "gin.Context" {
			return fmt.Errorf("gin.Context should be pointer *gin.Context: %w", ErrOperationInvalid)
		}

		if paramType.String() == "huma.Operation" {
			return fmt.Errorf("huma.Operation should be pointer *huma.Operation: %w", ErrOperationInvalid)
		}

		if err := dep.validate(paramType); err != nil {
			return err
		}
	}

	types := []reflect.Type{}
	for i := len(o.Depends); i < method.NumIn(); i++ {
		paramType := method.In(i)

		switch paramType.String() {
		case "gin.Context", "*gin.Context":
			return fmt.Errorf("expected param but found gin.Context: %w", ErrOperationInvalid)
		case "huma.Operation", "*huma.Operation":
			return fmt.Errorf("expected param but found huma.Operation: %w", ErrOperationInvalid)
		}

		types = append(types, paramType)
	}

	requestBody := false
	if len(types) == len(o.Params)+1 {
		requestBody = true
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

	for i, header := range o.ResponseHeaders {
		if err := validateHeader(header, method.Out(i)); err != nil {
			return err
		}
	}

	for i, resp := range o.Responses {
		respType := method.Out(len(o.ResponseHeaders) + i)
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
