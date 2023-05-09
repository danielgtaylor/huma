package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/negotiation"
	"gopkg.in/yaml.v2"
)

var rxSchema = regexp.MustCompile(`#/components/schemas/([^"]+)`)

// Resolver runs a `Resolve` function after a request has been parsed, enabling
// you to run custom validation or other code that can modify the request and /
// or return errors.
type Resolver interface {
	Resolve(ctx Context) []error
}

var resolverType = reflect.TypeOf((*Resolver)(nil)).Elem()

// Adapter is an interface that allows the API to be used with different HTTP
// routers and frameworks. It is designed to work with the standard library
// `http.Request` and `http.ResponseWriter` types as well as types like
// `gin.Context` or `fiber.Ctx` that provide both request and response
// functionality in one place.
type Adapter interface {
	Handle(method, path string, handler func(ctx Context))
}

// Context is the current request/response context. It provides a generic
// interface to get request information and write responses.
type Context interface {
	GetContext() context.Context
	GetURL() url.URL
	GetParam(name string) string
	GetQuery(name string) string
	GetHeader(name string) string
	GetBodyReader() io.Reader
	WriteStatus(code int)
	AppendHeader(name, value string)
	WriteHeader(name, value string)
	BodyWriter() io.Writer
}

type Transformer func(ctx Context, op *Operation, status string, v any) (any, error)

// Config represents a configuration for a new API.
type Config struct {
	// OpenAPI spec for the API. You should set at least the `Info.Title` and
	// `Info.Version` fields.
	*OpenAPI

	// OpenAPIPath is the path to the OpenAPI spec without extension. Defaults
	// to `/openapi` which allows clients to get `/openapi.json` or
	// `/openapi.yaml`.
	OpenAPIPath string
	DocsPath    string
	SchemasPath string

	// Formats defines the supported request/response formats by content type or
	// extension (e.g. `+json`). If not set, this defaults to supporting JSON
	// and CBOR with associated `+json` and `+cbor` extensions.
	Formats map[string]Format

	// DefaultFormat specifies the default content type to use when the client
	// does not specify one. Defaults to `application/json` if present in the
	// `Formats` map, otherwise panics on startup if empty.
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
	Marshal(ctx Context, op *Operation, respKey string, contentType string, v any) error

	// Unmarshal unmarshals the given data into the given value. The content type
	Unmarshal(contentType string, data []byte, v any) error
}

type Format struct {
	Marshal   func(writer io.Writer, v any) error
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
	f, ok := r.formats[contentType[start:end]]
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

func (a *api) Marshal(ctx Context, op *Operation, respKey string, ct string, v any) error {
	// fmt.Println("marshaling", ct)
	var err error

	for _, t := range a.transformers {
		v, err = t(ctx, op, respKey, v)
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

	if config.DefaultFormat != "" {
		newAPI.formatKeys = append(newAPI.formatKeys, config.DefaultFormat)
	}
	for k, v := range config.Formats {
		newAPI.formats[k] = v
		newAPI.formatKeys = append(newAPI.formatKeys, k)
	}

	if config.OpenAPIPath != "" {
		var specJSON []byte
		a.Handle(http.MethodGet, config.OpenAPIPath+".json", func(ctx Context) {
			ctx.WriteHeader("Content-Type", "application/vnd.oai.openapi+json")
			if specJSON == nil {
				specJSON, _ = json.Marshal(newAPI.OpenAPI())
			}
			ctx.BodyWriter().Write(specJSON)
		})
		var specYAML []byte
		a.Handle(http.MethodGet, config.OpenAPIPath+".yaml", func(ctx Context) {
			ctx.WriteHeader("Content-Type", "application/vnd.oai.openapi+yaml")
			if specYAML == nil {
				specYAML, _ = yaml.Marshal(newAPI.OpenAPI())
			}
			ctx.BodyWriter().Write(specYAML)
		})
	}

	if config.DocsPath != "" {
		a.Handle(http.MethodGet, config.DocsPath, func(ctx Context) {
			ctx.WriteHeader("Content-Type", "text/html")
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
		a.Handle(http.MethodGet, config.SchemasPath+"/{schema}.json", func(ctx Context) {
			schema := ctx.GetParam("schema")
			ctx.WriteHeader("Content-Type", "application/json")
			// TODO: copy & convert refs...
			b, _ := json.Marshal(config.OpenAPI.Components.Schemas.Map()[schema])

			b = rxSchema.ReplaceAll(b, []byte(config.SchemasPath+`/$1.json`))
			ctx.BodyWriter().Write(b)
		})
	}

	return newAPI
}
