package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/negotiation"
	"github.com/danielgtaylor/huma/schema"
	"github.com/fxamacker/cbor/v2"
	"gopkg.in/yaml.v2"
)

// ContextFromRequest returns a Huma context for a request, useful for
// accessing high-level convenience functions from e.g. middleware.
func ContextFromRequest(w http.ResponseWriter, r *http.Request) Context {
	return &hcontext{
		ResponseWriter: w,
		r:              r,
	}
}

// Context provides a request context and response writer with convenience
// functions for error and model marshaling in handler functions.
type Context interface {
	context.Context
	http.ResponseWriter

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
	schema *schema.Schema
}

func (c *hcontext) AddError(err error) {
	c.errors = append(c.errors, err)
}

func (c *hcontext) HasError() bool {
	return len(c.errors) > 0
}

func (c *hcontext) WriteError(status int, message string, errors ...error) {
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
	case "application/json":
		ct = "application/problem+json"
	case "application/yaml", "application/x-yaml":
		ct = "application/problem+yaml"
	}

	c.writeModel(ct, status, model)
}

func (c *hcontext) WriteModel(status int, model interface{}) {
	// Get the negotiated content type the client wants and we are willing to
	// provide.
	ct := selectContentType(c.r)

	c.writeModel(ct, status, model)
}

func (c *hcontext) writeModel(ct string, status int, model interface{}) {
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
	c.ResponseWriter.Header().Set("Content-Type", ct)
	c.ResponseWriter.WriteHeader(status)
	c.ResponseWriter.Write(encoded)
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
