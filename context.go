package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

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

	// WriteContent wraps http.ServeContent in order to handle serving streams
	// it will handle Range and Modified (like If-Unmodified-Since) headers.
	WriteContent(name string, content io.ReadSeeker, lastModified time.Time)
}

type hcontext struct {
	context.Context
	http.ResponseWriter
	r                     *http.Request
	errors                []error
	errorCode             int
	op                    *Operation
	closed                bool
	docsPrefix            string
	urlPrefix             string
	disableSchemaProperty bool
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
			if r.status == status || r.status == 0 {
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
		panic(fmt.Errorf("Trying to write to response after WriteModel, WriteError, or WriteContent for %s %s", c.r.Method, c.r.URL.Path))
	}

	return c.ResponseWriter.Write(data)
}

func (c *hcontext) WriteError(status int, message string, errors ...error) {
	if c.closed {
		panic(fmt.Errorf("Trying to write to response after WriteModel, WriteError, or WriteContent for %s %s", c.r.Method, c.r.URL.Path))
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
		panic(fmt.Errorf("Trying to write to response after WriteModel, WriteError, or WriteContent for %s %s", c.r.Method, c.r.URL.Path))
	}

	// Get the negotiated content type the client wants and we are willing to
	// provide.
	ct := selectContentType(c.r)

	c.writeModel(ct, status, model)
}

// URLPrefix returns the prefix to use for non-relative URL links.
func (c *hcontext) URLPrefix() string {
	if c.urlPrefix != "" {
		return c.urlPrefix
	}

	scheme := "https"
	if strings.HasPrefix(c.r.Host, "localhost") {
		scheme = "http"
	}

	return scheme + "://" + c.r.Host
}

// shallowStructToMap converts a struct to a map similar to how encoding/json
// would do it, but only one level deep so that the map may be modified before
// serialization.
func shallowStructToMap(v reflect.Value, result map[string]interface{}) {
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		shallowStructToMap(v.Elem(), result)
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := f.Name
		if len(name) > 0 && strings.ToUpper(name)[0] != name[0] {
			// Private field we somehow have access to?
			continue
		}
		if f.Anonymous {
			// Anonymous embedded struct, process its fields as our own.
			shallowStructToMap(v.Field(i), result)
			continue
		}
		if json := f.Tag.Get("json"); json != "" {
			parts := strings.Split(json, ",")
			if parts[0] != "" {
				name = parts[0]
			}
			if name == "-" {
				continue
			}
			if len(parts) > 1 && parts[1] == "omitempty" {
				vf := v.Field(i)
				zero := vf.IsZero()
				if vf.Kind() == reflect.Slice || vf.Kind() == reflect.Map {
					// Special case: omit if they have no items in them to match the
					// JSON encoder.
					zero = vf.Len() == 0
				}
				if zero {
					continue
				}
			}
		}
		result[name] = v.Field(i).Interface()
	}
}

func (c *hcontext) writeModel(ct string, status int, model interface{}) {
	// Is this allowed? Find the right response.
	modelRef := ""
	modelType := reflect.TypeOf(model)
	if c.op != nil {
		responses := []Response{}
		names := []string{}
		statuses := []string{}
		for _, r := range c.op.responses {
			statuses = append(statuses, fmt.Sprintf("%d", r.status))
			if r.status == status || r.status == 0 {
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
			if r.model == modelType {
				found = true
				modelRef = r.modelRef
				break
			}
		}

		if !found {
			panic(fmt.Errorf("Invalid model %s, expecting %s for %s %s", modelType, strings.Join(names, ", "), c.r.Method, c.r.URL.Path))
		}
	}

	// If possible, insert a link relation header to the JSON Schema describing
	// this response. If it's an object (not an array), then we can also try
	// inserting the `$schema` key to make editing & validation easier.
	parts := strings.Split(modelRef, "/")
	if len(parts) > 0 {
		id := parts[len(parts)-1]

		link := c.Header().Get("Link")
		if link != "" {
			link += ", "
		}
		link += "<" + c.docsPrefix + "/schemas/" + id + ".json>; rel=\"describedby\""
		c.Header().Set("Link", link)

		if modelType.Kind() == reflect.Ptr {
			modelType = modelType.Elem()
		}
		if !c.disableSchemaProperty && modelType != nil && modelType.Kind() == reflect.Struct && modelType != timeType {
			tmp := map[string]interface{}{}
			shallowStructToMap(reflect.ValueOf(model), tmp)
			if tmp["$schema"] == nil {
				tmp["$schema"] = c.URLPrefix() + c.docsPrefix + "/schemas/" + id + ".json"
			}
			model = tmp
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
			panic(fmt.Errorf("Unable to marshal CBOR: %w", err))
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

func (c *hcontext) WriteContent(name string, content io.ReadSeeker, lastModified time.Time) {
	if c.closed {
		panic(fmt.Errorf("Trying to write to response after WriteModel, WriteError, or WriteContent for %s %s", c.r.Method, c.r.URL.Path))
	}

	http.ServeContent(c.ResponseWriter, c.r, name, lastModified, content)
	c.closed = true
}
