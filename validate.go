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

// ErrAPIInvalid is returned when validating the OpenAPI top-level fields
// has failed.
var ErrAPIInvalid = errors.New("invalid API")

// ErrOperationInvalid is returned when validating an operation has failed.
var ErrOperationInvalid = errors.New("invalid operation")

// ErrParamInvalid is returned when validating the parameter has failed.
var ErrParamInvalid = errors.New("invalid parameter")

var paramRe = regexp.MustCompile(`:([^/]+)|{([^}]+)}`)

// validate the top-level API
func (a *OpenAPI) validate() error {
	if a.Title == "" {
		return fmt.Errorf("title is required: %w", ErrAPIInvalid)
	}

	if a.Version == "" {
		return fmt.Errorf("version is required: %w", ErrAPIInvalid)
	}

	return nil
}

// validate the parameter and generate schemas
func (p *Param) validate(t reflect.Type) error {
	if p.typ != nil && p.typ != t {
		return fmt.Errorf("parameter declared as %s was previously declared as %s: %w", t, p.typ, ErrParamInvalid)
	}

	if p.Example != nil {
		et := reflect.ValueOf(p.Example).Type()
		if t != et {
			return fmt.Errorf("parameter declared as %s has example of type %s: %w", t, et, ErrParamInvalid)
		}
	}

	p.typ = t

	if p.Schema == nil || p.Schema.Type == "" {
		s, err := GenerateSchemaWithMode(p.typ, SchemaModeWrite, p.Schema)
		if err != nil {
			return err
		}
		p.Schema = s

		if p.def != nil {
			p.Schema.Default = p.def
		}

		if p.Example != nil {
			// Some tools have better support for the param example, others for the
			// schema example, so we include it in both.
			p.Schema.Example = p.Example
		}
	}

	return nil
}

// validate the header and generate schemas
func (h *ResponseHeader) validate(t reflect.Type) error {
	if h.Schema == nil {
		// Generate the schema from the handler function types.
		s, err := GenerateSchemaWithMode(t, SchemaModeRead, nil)
		if err != nil {
			return err
		}
		h.Schema = s
	}

	return nil
}

// validate checks that the operation is well-formed (e.g. handler signature
// matches the given params) and generates schemas if needed.
func (o *Operation) validate(method, path string) error {
	if o.Description == "" {
		return fmt.Errorf("description field required: %w", ErrOperationInvalid)
	}

	if len(o.Responses) == 0 {
		return fmt.Errorf("at least one response is required: %w", ErrOperationInvalid)
	}

	if o.Handler == nil {
		return fmt.Errorf("handler is required: %w", ErrOperationInvalid)
	}

	handler := reflect.ValueOf(o.Handler).Type()

	totalIn := len(o.Dependencies) + len(o.Params)
	totalOut := len(o.ResponseHeaders) + len(o.Responses)
	if !(handler.NumIn() == totalIn || (method != http.MethodGet && handler.NumIn() == totalIn+1)) || handler.NumOut() != totalOut {
		expected := "func("
		for _, dep := range o.Dependencies {
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

		return fmt.Errorf("expected %s but found %s: %w", expected, handler, ErrOperationInvalid)
	}

	if o.ID == "" {
		verb := method

		// Try to detect calls returning lists of things.
		if handler.NumOut() > 0 {
			k := handler.Out(0).Kind()
			if k == reflect.Array || k == reflect.Slice {
				verb = "list"
			}
		}

		// Remove variables from path so they aren't in the generated name.
		path := paramRe.ReplaceAllString(path, "")

		o.ID = slug.Make(verb + path)
	}

	for i, dep := range o.Dependencies {
		paramType := handler.In(i)

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
	for i := len(o.Dependencies); i < handler.NumIn(); i++ {
		paramType := handler.In(i)

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
				s, err := GenerateSchemaWithMode(paramType, SchemaModeWrite, nil)
				if err != nil {
					return err
				}
				o.RequestSchema = s
			}
			continue
		}

		p := o.Params[i]
		if err := p.validate(paramType); err != nil {
			return err
		}
	}

	for i, header := range o.ResponseHeaders {
		if err := header.validate(handler.Out(i)); err != nil {
			return err
		}
	}

	for i, resp := range o.Responses {
		respType := handler.Out(len(o.ResponseHeaders) + i)
		// HTTP 204 explicitly forbids a response body.
		if resp.StatusCode != 204 && resp.Schema == nil {
			// Generate the schema from the handler function types.
			s, err := GenerateSchemaWithMode(respType, SchemaModeRead, nil)
			if err != nil {
				return err
			}
			resp.Schema = s
		}
	}

	return nil
}
