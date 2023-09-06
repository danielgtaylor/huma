package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2/negotiation"
	"github.com/goccy/go-yaml"
)

var rxSchema = regexp.MustCompile(`#/components/schemas/([^"]+)`)

// Resolver runs a `Resolve` function after a request has been parsed, enabling
// you to run custom validation or other code that can modify the request and /
// or return errors.
type Resolver interface {
	Resolve(ctx Context) []error
}

// ResolverWithPath runs a `Resolve` function after a request has been parsed,
// enabling you to run custom validation or other code that can modify the
// request and / or return errors. The `prefix` is the path to the current
// location for errors, e.g. `body.foo[0].bar`.
type ResolverWithPath interface {
	Resolve(ctx Context, prefix *PathBuffer) []error
}

var resolverType = reflect.TypeOf((*Resolver)(nil)).Elem()
var resolverWithPathType = reflect.TypeOf((*ResolverWithPath)(nil)).Elem()

// Adapter is an interface that allows the API to be used with different HTTP
// routers and frameworks. It is designed to work with the standard library
// `http.Request` and `http.ResponseWriter` types as well as types like
// `gin.Context` or `fiber.Ctx` that provide both request and response
// functionality in one place.
type Adapter interface {
	Handle(op *Operation, handler func(ctx Context))
	ServeHTTP(http.ResponseWriter, *http.Request)
}

// Context is the current request/response context. It provides a generic
// interface to get request information and write responses.
type Context interface {
	Operation() *Operation
	Context() context.Context
	Method() string
	Host() string
	URL() url.URL
	Param(name string) string
	Query(name string) string
	Header(name string) string
	EachHeader(cb func(name, value string))
	BodyReader() io.Reader
	GetMultipartForm() (*multipart.Form, error)
	SetReadDeadline(time.Time) error
	SetStatus(code int)
	SetHeader(name, value string)
	AppendHeader(name, value string)
	BodyWriter() io.Writer
}

// Transformer is a function that can modify a response body before it is
// serialized. The `status` is the HTTP status code for the response and `v` is
// the value to be serialized. The return value is the new value to be
// serialized or an error.
type Transformer func(ctx Context, status string, v any) (any, error)

// Config represents a configuration for a new API. See `huma.DefaultConfig()`
// as a starting point.
type Config struct {
	// OpenAPI spec for the API. You should set at least the `Info.Title` and
	// `Info.Version` fields.
	*OpenAPI

	// OpenAPIPath is the path to the OpenAPI spec without extension. If set
	// to `/openapi` it will allow clients to get `/openapi.json` or
	// `/openapi.yaml`, for example.
	OpenAPIPath string
	DocsPath    string
	SchemasPath string

	// Formats defines the supported request/response formats by content type or
	// extension (e.g. `json` for `application/my-format+json`).
	Formats map[string]Format

	// DefaultFormat specifies the default content type to use when the client
	// does not specify one. If unset, the default type will be randomly
	// chosen from the keys of `Formats`.
	DefaultFormat string

	// Transformers are a way to modify a response body before it is serialized.
	Transformers []Transformer
}

// API represents a Huma API wrapping a specific router.
type API interface {
	// Adapter returns the router adapter for this API, providing a generic
	// interface to get request information and write responses.
	Adapter() Adapter

	// OpenAPI returns the OpenAPI spec for this API. You may edit this spec
	// until the server starts.
	OpenAPI() *OpenAPI

	// Negotiate returns the selected content type given the client's `accept`
	// header and the server's supported content types. If the client does not
	// send an `accept` header, then JSON is used.
	Negotiate(accept string) (string, error)

	// Marshal marshals the given value into the given writer. The content type
	// is used to determine which format to use. Use `Negotiate` to get the
	// content type from an accept header. TODO: update
	Marshal(ctx Context, respKey string, contentType string, v any) error

	// Unmarshal unmarshals the given data into the given value. The content type
	Unmarshal(contentType string, data []byte, v any) error
}

// Format represents a request / response format. It is used to marshal and
// unmarshal data.
type Format struct {
	// Marshal a value to a given writer (e.g. response body).
	Marshal func(writer io.Writer, v any) error

	// Unmarshal a value into `v` from the given bytes (e.g. request body).
	Unmarshal func(data []byte, v any) error
}

type api struct {
	config       Config
	adapter      Adapter
	formats      map[string]Format
	formatKeys   []string
	transformers []Transformer
}

func (r *api) Adapter() Adapter {
	return r.adapter
}

func (r *api) OpenAPI() *OpenAPI {
	return r.config.OpenAPI
}

func (r *api) Unmarshal(contentType string, data []byte, v any) error {
	// Handle e.g. `application/json; charset=utf-8` or `my/format+json`
	start := strings.IndexRune(contentType, '+') + 1
	end := strings.IndexRune(contentType, ';')
	if end == -1 {
		end = len(contentType)
	}
	ct := contentType[start:end]
	if ct == "" {
		// Default to assume JSON since this is an API.
		ct = "application/json"
	}
	f, ok := r.formats[ct]
	if !ok {
		return fmt.Errorf("unknown content type: %s", contentType)
	}
	return f.Unmarshal(data, v)
}

func (r *api) Negotiate(accept string) (string, error) {
	ct := negotiation.SelectQValueFast(accept, r.formatKeys)
	if ct == "" {
		ct = r.formatKeys[0]
	}
	if _, ok := r.formats[ct]; !ok {
		return ct, fmt.Errorf("unknown content type: %s", ct)
	}
	return ct, nil
}

func (a *api) Marshal(ctx Context, respKey string, ct string, v any) error {
	var err error

	for _, t := range a.transformers {
		v, err = t(ctx, respKey, v)
		if err != nil {
			return err
		}
	}

	f, ok := a.formats[ct]
	if !ok {
		start := strings.IndexRune(ct, '+') + 1
		f, ok = a.formats[ct[start:]]
	}
	if !ok {
		return fmt.Errorf("unknown content type: %s", ct)
	}
	return f.Marshal(ctx.BodyWriter(), v)
}

func NewAPI(config Config, a Adapter) API {
	newAPI := &api{
		config:       config,
		adapter:      a,
		formats:      map[string]Format{},
		transformers: config.Transformers,
	}

	if config.OpenAPI == nil {
		config.OpenAPI = &OpenAPI{}
	}

	if config.OpenAPI.OpenAPI == "" {
		config.OpenAPI.OpenAPI = "3.1.0"
	}

	if config.OpenAPI.Components == nil {
		config.OpenAPI.Components = &Components{}
	}

	if config.OpenAPI.Components.Schemas == nil {
		config.OpenAPI.Components.Schemas = NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	}

	if config.DefaultFormat == "" && config.Formats["application/json"].Marshal != nil {
		config.DefaultFormat = "application/json"
	}
	if config.DefaultFormat != "" {
		newAPI.formatKeys = append(newAPI.formatKeys, config.DefaultFormat)
	}
	for k, v := range config.Formats {
		newAPI.formats[k] = v
		newAPI.formatKeys = append(newAPI.formatKeys, k)
	}

	if config.OpenAPIPath != "" {
		var specJSON []byte
		a.Handle(&Operation{
			Method: http.MethodGet,
			Path:   config.OpenAPIPath + ".json",
		}, func(ctx Context) {
			ctx.SetHeader("Content-Type", "application/vnd.oai.openapi+json")
			if specJSON == nil {
				specJSON, _ = json.Marshal(newAPI.OpenAPI())
			}
			ctx.BodyWriter().Write(specJSON)
		})
		var specYAML []byte
		a.Handle(&Operation{
			Method: http.MethodGet,
			Path:   config.OpenAPIPath + ".yaml",
		}, func(ctx Context) {
			ctx.SetHeader("Content-Type", "application/vnd.oai.openapi+yaml")
			if specYAML == nil {
				specYAML, _ = yaml.Marshal(newAPI.OpenAPI())
			}
			ctx.BodyWriter().Write(specYAML)
		})
	}

	if config.DocsPath != "" {
		a.Handle(&Operation{
			Method: http.MethodGet,
			Path:   config.DocsPath,
		}, func(ctx Context) {
			ctx.SetHeader("Content-Type", "text/html")
			ctx.BodyWriter().Write([]byte(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no">
    <title>Elements in HTML</title>
    <!-- Embed elements Elements via Web Component -->
    <script src="https://unpkg.com/@stoplight/elements/web-components.min.js"></script>
    <link rel="stylesheet" href="https://unpkg.com/@stoplight/elements/styles.min.css">
  </head>
  <body>

    <elements-api
      apiDescriptionUrl="` + config.OpenAPIPath + `.yaml"
      router="hash"
      layout="sidebar"
    />

  </body>
</html>`))
		})
	}

	if config.SchemasPath != "" {
		a.Handle(&Operation{
			Method: http.MethodGet,
			Path:   config.SchemasPath + "/{schema}",
		}, func(ctx Context) {
			// Some routers dislike a path param+suffix, so we strip it here instead.
			schema := strings.TrimSuffix(ctx.Param("schema"), ".json")
			ctx.SetHeader("Content-Type", "application/json")
			b, _ := json.Marshal(config.OpenAPI.Components.Schemas.Map()[schema])
			b = rxSchema.ReplaceAll(b, []byte(config.SchemasPath+`/$1.json`))
			ctx.BodyWriter().Write(b)
		})
	}

	return newAPI
}
