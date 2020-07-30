package huma

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/gosimple/slug"
	"github.com/istreamlabs/huma/schema"
)

// ErrAPIInvalid is returned when validating the OpenAPI top-level fields
// has failed.
var ErrAPIInvalid = errors.New("invalid API")

// ErrOperationInvalid is returned when validating an operation has failed.
var ErrOperationInvalid = errors.New("invalid operation")

// ErrParamInvalid is returned when validating the parameter has failed.
var ErrParamInvalid = errors.New("invalid parameter")

var paramRe = regexp.MustCompile(`:([^/]+)|{([^}]+)}`)
var versionRe = regexp.MustCompile(`^/v[0-9]+`)

// validate the top-level API
func (a *openAPI) validate() error {
	if a.Title == "" {
		return fmt.Errorf("title is required: %w", ErrAPIInvalid)
	}

	if a.Version == "" {
		return fmt.Errorf("version is required: %w", ErrAPIInvalid)
	}

	return nil
}

// validate the parameter and generate schemas
func (p *openAPIParam) validate(t reflect.Type) {
	switch p.In {
	case inPath, inQuery, inHeader:
	default:
		panic(fmt.Errorf("parameter %s location invalid: %s", p.Name, p.In))
	}

	if t == nil {
		// Unknown type for unsafe handlers defaults to `string` for path params
		// and to the given default value's type for everything else.
		if p.def != nil {
			t = reflect.TypeOf(p.def)
		} else {
			t = reflect.TypeOf("")
		}
	}

	if p.typ != nil && p.typ != t {
		panic(fmt.Errorf("parameter %s declared as %s was previously declared as %s: %w", p.Name, t, p.typ, ErrParamInvalid))
	}

	if p.def != nil {
		dt := reflect.ValueOf(p.def).Type()
		if t != dt {
			panic(fmt.Errorf("parameter %s declared as %s has default of type %s: %w", p.Name, t, dt, ErrParamInvalid))
		}
	}

	if p.Example != nil {
		et := reflect.ValueOf(p.Example).Type()
		if t != et {
			panic(fmt.Errorf("parameter %s declared as %s has example of type %s: %w", p.Name, t, et, ErrParamInvalid))
		}
	}

	p.typ = t

	if p.Schema == nil || p.Schema.Type == "" {
		s, err := schema.GenerateWithMode(p.typ, schema.ModeWrite, p.Schema)
		if err != nil {
			panic(fmt.Errorf("parameter %s schema generation error: %w", p.Name, err))
		}
		p.Schema = s

		if p.def != nil {
			if t, ok := p.def.(time.Time); ok {
				// Time defaults are only included if they are not the zero time.
				if !t.IsZero() {
					p.Schema.Default = p.def
				}
			} else {
				p.Schema.Default = p.def
			}
		}

		if p.Example != nil {
			// Some tools have better support for the param example, others for the
			// schema example, so we include it in both.
			p.Schema.Example = p.Example
		}
	}
}

// validate the header and generate schemas
func (h *openAPIResponseHeader) validate(t reflect.Type) {
	if t == nil {
		// Unsafe handlers default to string headers
		t = reflect.TypeOf("")
	}

	if h.Schema == nil {
		// Generate the schema from the handler function types.
		s, err := schema.GenerateWithMode(t, schema.ModeRead, nil)
		if err != nil {
			panic(fmt.Errorf("response header %s schema generation error: %w", h.Name, err))
		}
		h.Schema = s
	}
}

// validate checks that the operation is well-formed (e.g. handler signature
// matches the given params) and generates schemas if needed.
func (o *openAPIOperation) validate(method, path string) {
	prefix := method + " " + path + ":"

	if o.summary == "" && o.description == "" {
		panic(fmt.Errorf("%s summary or description field required: %w", prefix, ErrOperationInvalid))
	}

	if len(o.responses) == 0 {
		panic(fmt.Errorf("%s at least one response is required: %w", prefix, ErrOperationInvalid))
	}

	if o.handler == nil {
		panic(fmt.Errorf("%s handler is required: %w", prefix, ErrOperationInvalid))
	}

	handler := reflect.ValueOf(o.handler).Type()

	validateHandler := !o.unsafe()
	if validateHandler {
		totalIn := len(o.dependencies) + len(o.params)
		totalOut := len(o.responseHeaders) + len(o.responses)
		if !(handler.NumIn() == totalIn || (method != http.MethodGet && handler.NumIn() == totalIn+1)) || handler.NumOut() != totalOut {
			expected := "func("
			for _, dep := range o.dependencies {
				val := reflect.ValueOf(dep.handler)
				if !val.IsValid() {
					panic(fmt.Errorf("dependency %s is not a valid type: %w", dep.handler, ErrParamInvalid))
				}
				expected += "? " + val.Type().String() + ", "
			}
			for _, param := range o.params {
				expected += param.Name + " ?, "
			}
			expected = strings.TrimRight(expected, ", ")
			expected += ") ("
			for _, h := range o.responseHeaders {
				expected += h.Name + " ?, "
			}
			for _, r := range o.responses {
				expected += fmt.Sprintf("*Response%d, ", r.StatusCode)
			}
			expected = strings.TrimRight(expected, ", ")
			expected += ")"

			panic(fmt.Errorf("%s expected handler %s but found %s: %w", prefix, expected, handler, ErrOperationInvalid))
		}
	}

	if o.id == "" {
		verb := method

		// Try to detect calls returning lists of things.
		if validateHandler && handler.NumOut() > 0 {
			k := handler.Out(0).Kind()
			if k == reflect.Array || k == reflect.Slice {
				verb = "list"
			}
		}

		// Remove variables from path so they aren't in the generated name.
		path := paramRe.ReplaceAllString(path, "")

		// Remove version at the beginning of the path if present
		path = versionRe.ReplaceAllString(path, "")

		o.id = slug.Make(verb + path)
	}

	for i, dep := range o.dependencies {
		if validateHandler {
			paramType := handler.In(i)

			// Catch common errors.
			if paramType.String() == "gin.Context" {
				panic(fmt.Errorf("%s gin.Context should be pointer *gin.Context: %w", prefix, ErrOperationInvalid))
			}

			dep.validate(paramType)
		} else {
			dep.validate(nil)
		}
	}

	types := []reflect.Type{}
	if validateHandler {
		for i := len(o.dependencies); i < handler.NumIn(); i++ {
			paramType := handler.In(i)

			switch paramType.String() {
			case "gin.Context", "*gin.Context":
				panic(fmt.Errorf("%s expected param but found gin.Context: %w", prefix, ErrOperationInvalid))
			}

			types = append(types, paramType)
		}
	} else {
		for i := 0; i < len(o.params); i++ {
			types = append(types, nil)
		}
	}

	requestBody := false
	if len(types) == len(o.params)+1 {
		requestBody = true
	}

	for i, paramType := range types {
		if i == len(types)-1 && requestBody {
			// The last item has no associated param. It is a request body.
			if o.requestSchema == nil {
				s, err := schema.GenerateWithMode(paramType, schema.ModeWrite, nil)
				if err != nil {
					panic(fmt.Errorf("%s request body schema generation error: %w", prefix, err))
				}
				o.requestSchema = s
			}
			continue
		}

		p := o.params[i]
		p.validate(paramType)
	}

	for i, header := range o.responseHeaders {
		if validateHandler {
			header.validate(handler.Out(i))
		} else {
			header.validate(nil)
		}
	}

	for i, resp := range o.responses {
		if validateHandler {
			respType := handler.Out(len(o.responseHeaders) + i)
			// HTTP 204 explicitly forbids a response body. We model this with an
			// empty content type.
			if resp.ContentType != "" && resp.Schema == nil {
				// Generate the schema from the handler function types.
				s, err := schema.GenerateWithMode(respType, schema.ModeRead, nil)
				if err != nil {
					panic(fmt.Errorf("%s response %d schema generation error: %w", prefix, resp.StatusCode, err))
				}
				resp.Schema = s
			}
		}
	}
}
