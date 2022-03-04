package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/negotiation"
	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-yaml"
)

// allowedHeaders is a list of built-in headers that are always allowed without
// explicitly being documented. Mostly they are low-level HTTP headers that
// control access or connection settings.
var allowedHeaders = map[string]bool{
	"access-control-allow-origin":  true,
	"access-control-allow-methods": true,
	"access-control-allow-headers": true,
	"access-control-max-age":       true,
	"connection":                   true,
	"keep-alive":                   true,
	"vary":                         true,
}

// ContextFromRequest returns a Huma context for a request, useful for
// accessing high-level convenience functions from e.g. middleware.
func ContextFromRequest(w http.ResponseWriter, r *http.Request) Context {
	return &hcontext{
		Context:        r.Context(),
		ResponseWriter: w,
		r:              r,
	}
}

// Context provides a request context and response writer with convenience
// functions for error and model marshaling in handler functions.
type Context interface {
	context.Context
	http.ResponseWriter

	// WithValue returns a shallow copy of the context with a new context value
	// applied to it.
	WithValue(key, value interface{}) Context

	// SetValue sets a context value. The Huma context is modified in place while
	// the underlying request context is copied. This is particularly useful for
	// setting context values from input resolver functions.
	SetValue(key, value interface{})

	// AddError adds a new error to the list of errors for this request.
	AddError(err error)

	// HasError returns true if at least one error has been added to the context.
	HasError() bool

	// WriteError writes out an HTTP status code, friendly error message, and
	// optionally a set of error details set with `AddError` and/or passed in.
	WriteError(status int, message string, errors ...error)

	// WriteModel writes out an HTTP status code and marshalled model based on
	// content negotiation (e.g. JSON or CBOR). This must match the registered
	// response status code & type.
	WriteModel(status int, model interface{})
}

type hcontext struct {
	context.Context
	http.ResponseWriter
	r      *http.Request
	errors []error
	op     *Operation
	closed bool
}

func (c *hcontext) WithValue(key, value interface{}) Context {
	r := c.r.WithContext(context.WithValue(c.r.Context(), key, value))
	return &hcontext{
		Context:        context.WithValue(c.Context, key, value),
		ResponseWriter: c.ResponseWriter,
		r:              r,
		errors:         append([]error{}, c.errors...),
		op:             c.op,
		closed:         c.closed,
	}
}

func (c *hcontext) SetValue(key, value interface{}) {
	c.r = c.r.WithContext(context.WithValue(c.r.Context(), key, value))
	c.Context = c.r.Context()
}

func (c *hcontext) AddError(err error) {
	c.errors = append(c.errors, err)
}

func (c *hcontext) HasError() bool {
	return len(c.errors) > 0
}

func (c *hcontext) WriteHeader(status int) {
	if c.op != nil {
		allowed := []string{}
		for _, r := range c.op.responses {
			if r.status == status {
				for _, h := range r.headers {
					allowed = append(allowed, h)
				}
			}
		}

		// Check that all headers were allowed to be sent.
		for name := range c.Header() {
			if allowedHeaders[strings.ToLower(name)] {
				continue
			}

			found := false

			for _, h := range allowed {
				if strings.ToLower(name) == strings.ToLower(h) {
					found = true
					break
				}
			}

			if !found {
				panic(fmt.Errorf("Response header %s is not declared for %s %s with status code %d (allowed: %s)", name, c.r.Method, c.r.URL.Path, status, allowed))
			}
		}
	}

	c.ResponseWriter.WriteHeader(status)
}

func (c *hcontext) Write(data []byte) (int, error) {
	if c.closed {
		panic(fmt.Errorf("Trying to write to response after WriteModel or WriteError for %s %s", c.r.Method, c.r.URL.Path))
	}

	return c.ResponseWriter.Write(data)
}

func (c *hcontext) WriteError(status int, message string, errors ...error) {
	if c.closed {
		panic(fmt.Errorf("Trying to write to response after WriteModel or WriteError for %s %s", c.r.Method, c.r.URL.Path))
	}

	details := []*ErrorDetail{}

	c.errors = append(c.errors, errors...)
	for _, err := range c.errors {
		if d, ok := err.(ErrorDetailer); ok {
			details = append(details, d.ErrorDetail())
		} else {
			details = append(details, &ErrorDetail{Message: err.Error()})
		}
	}

	model := &ErrorModel{
		Title:  http.StatusText(status),
		Status: status,
		Detail: message,
		Errors: details,
	}

	// Select content type and transform it to the appropriate error type.
	ct := selectContentType(c.r)
	switch ct {
	case "application/cbor":
		ct = "application/problem+cbor"
	case "", "application/json":
		ct = "application/problem+json"
	case "application/yaml", "application/x-yaml":
		ct = "application/problem+yaml"
	}

	c.writeModel(ct, status, model)
}

func (c *hcontext) WriteModel(status int, model interface{}) {
	if c.closed {
		panic(fmt.Errorf("Trying to write to response after WriteModel or WriteError for %s %s", c.r.Method, c.r.URL.Path))
	}

	// Get the negotiated content type the client wants and we are willing to
	// provide.
	ct := selectContentType(c.r)

	c.writeModel(ct, status, model)
}

func (c *hcontext) writeModel(ct string, status int, model interface{}) {
	// Is this allowed? Find the right response.
	if c.op != nil {
		responses := []Response{}
		names := []string{}
		statuses := []string{}
		for _, r := range c.op.responses {
			statuses = append(statuses, fmt.Sprintf("%d", r.status))
			if r.status == status {
				responses = append(responses, r)
				if r.model != nil {
					names = append(names, r.model.Name())
				}
			}
		}

		if len(responses) == 0 {
			panic(fmt.Errorf("HTTP status %d not allowed for %s %s, expected one of %s", status, c.r.Method, c.r.URL.Path, statuses))
		}

		found := false
		for _, r := range responses {
			if r.model == reflect.TypeOf(model) {
				found = true
				break
			}
		}

		if !found {
			panic(fmt.Errorf("Invalid model %s, expecting %s for %s %s", reflect.TypeOf(model), strings.Join(names, ", "), c.r.Method, c.r.URL.Path))
		}
	}

	// Do the appropriate encoding.
	var encoded []byte
	var err error
	if strings.HasPrefix(ct, "application/json") || strings.HasSuffix(ct, "+json") {
		encoded, err = json.Marshal(model)
		if err != nil {
			panic(fmt.Errorf("Unable to marshal JSON: %w", err))
		}
	} else if strings.HasPrefix(ct, "application/yaml") || strings.HasPrefix(ct, "application/x-yaml") || strings.HasSuffix(ct, "+yaml") {
		encoded, err = yaml.Marshal(model)
		if err != nil {
			panic(fmt.Errorf("Unable to marshal YAML: %w", err))
		}
	} else if strings.HasPrefix(ct, "application/cbor") || strings.HasSuffix(ct, "+cbor") {
		opts := cbor.CanonicalEncOptions()
		opts.Time = cbor.TimeRFC3339Nano
		opts.TimeTag = cbor.EncTagRequired
		mode, err := opts.EncMode()
		if err != nil {
			panic(fmt.Errorf("Unable to marshal CBOR: %w", err))
		}
		encoded, err = mode.Marshal(model)
		if err != nil {
			panic(fmt.Errorf("Unable to marshal JSON: %w", err))
		}
	}

	// Encoding succeeded, write the data!
	c.Header().Set("Content-Type", ct)
	c.WriteHeader(status)
	c.Write(encoded)
	c.closed = true
}

// selectContentType selects the best availalable content type via content
// negotiation with the client, defaulting to JSON.
func selectContentType(r *http.Request) string {
	ct := "application/json"

	if accept := r.Header.Get("Accept"); accept != "" {
		best := negotiation.SelectQValue(accept, []string{
			"application/cbor",
			"application/json",
			"application/yaml",
			"application/x-yaml",
		})

		if best != "" {
			ct = best
		}
	}

	return ct
}
