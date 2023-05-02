package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/danielgtaylor/huma/negotiation"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"
)

// Resolver runs a `Resolve` function after a request has been parsed, enabling
// you to run custom validation or other code that can modify the request and /
// or return errors.
type Resolver[R any] interface {
	Resolve(r R) []error
}

// Adapter is an interface that allows the API to be used with different HTTP
// routers and frameworks. It is designed to work with the standard library
// `http.Request` and `http.ResponseWriter` types as well as types like
// `gin.Context` or `fiber.Ctx` that provide both request and response
// functionality in one place.
type Adapter[R, W any] interface {
	Handle(method, path string, handler func(W, R))
	GetMatched(r R) string
	GetContext(r R) context.Context
	GetParam(r R, name string) string
	GetQuery(r R, name string) string
	GetHeader(r R, name string) string
	GetBody(r R) ([]byte, error)
	WriteStatus(w W, code int)
	AppendHeader(w W, name string, value string)
	WriteHeader(w W, name string, value string)
	BodyWriter(w W) io.Writer
}

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

	// Transformers are a way to modify a response body before it is serialized.
	Transformers []func(op *Operation, status string, v any) (any, error)

	// DisableSchemaField turns off the automatic generation of the `$schema`
	// field for struct responses. Disabling this feature can slightly improve
	// performance.
	DisableSchemaField bool

	// DisableSchemaLink turns off the automatic generation of the `describedby`
	// schema link relation response header. Disabling this feature can slightly
	// improve performance.
	DisableSchemaLink bool
}

// API represents a Huma API wrapping a specific router.
type API[R, W any] interface {
	// GetAdapter returns the router adapter for this API, providing a generic
	// interface to get request information and write responses.
	GetAdapter() Adapter[R, W]

	// OpenAPI returns the OpenAPI spec for this API. You may edit this spec
	// until the server starts.
	OpenAPI() *OpenAPI

	// AddFormat adds a new format to the API. The suffix is used to match
	// custom formats with a known extension like `application/my-format+json`.
	AddFormat(mime, suffix string, marshal func(w io.Writer, v any) error, unmarshal func(data []byte, v any) error)

	// Negotiate returns the selected content type given the client's `accept`
	// header and the server's supported content types. If the client does not
	// send an `accept` header, then JSON is used.
	Negotiate(accept string) (string, error)

	// Marshal marshals the given value into the given writer. The content type
	// is used to determine which format to use. Use `Negotiate` to get the
	// content type from an accept header.
	Marshal(contentType string, w io.Writer, v any) error

	// Unmarshal unmarshals the given data into the given value. The content type
	Unmarshal(contentType string, data []byte, v any) error

	// SchemaLink converts a JSON Schema ref into a link appropriate for the
	// HTTP link header, pointing to the JSON file describing this schema
	// independent of the OpenAPI spec.
	SchemaLink(ref string) string

	// TransformBody transforms the given value before it is serialized. This
	// is useful for modifying or adding fields to a response body before it
	// is just a stream of bytes. Think of it like one level up from middleware.
	// TODO: should status really be a string?!?
	TransformBody(op *Operation, status string, value any) (any, error)

	// HasSchemaField returns true if the API has the `$schema` field enabled.
	HasSchemaField() bool

	// HasSchemaLink returns true if the API has the `describedby` schema link
	// relation response header enabled.
	HasSchemaLink() bool
}

// StdAPI is an API using the standard library `http.Request` and
// `http.ResponseWriter` types.
type StdAPI = API[*http.Request, http.ResponseWriter]

type format struct {
	marshal   func(writer io.Writer, v any) error
	unmarshal func(data []byte, v any) error
}

type api[R, W any] struct {
	config       Config
	adapter      Adapter[R, W]
	formats      map[string]format
	formatKeys   []string
	transformers []func(op *Operation, status string, v any) (any, error)
}

func (r *api[R, W]) GetAdapter() Adapter[R, W] {
	return r.adapter
}

func (r *api[R, W]) OpenAPI() *OpenAPI {
	return r.config.OpenAPI
}

func (r *api[R, W]) AddFormat(contentType, suffix string, marshal func(w io.Writer, v any) error, unmarshal func(data []byte, v any) error) {
	r.formats[contentType] = format{marshal, unmarshal}
	r.formats[suffix] = format{marshal, unmarshal}
	if !slices.Contains(r.formatKeys, contentType) {
		r.formatKeys = append(r.formatKeys, contentType)
	}
}

func (r *api[R, W]) Unmarshal(contentType string, data []byte, v any) error {
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
	return f.unmarshal(data, v)
}

func (r *api[R, W]) Negotiate(accept string) (string, error) {
	ct := negotiation.SelectQValueFast(accept, r.formatKeys)
	if ct == "" {
		ct = r.formatKeys[0]
	}
	if _, ok := r.formats[ct]; !ok {
		return ct, fmt.Errorf("unknown content type: %s", ct)
	}
	return ct, nil
}

func (r *api[R, W]) Marshal(ct string, w io.Writer, v any) error {
	// fmt.Println("marshaling", ct)
	f, ok := r.formats[ct]
	if !ok {
		start := strings.IndexRune(ct, '+') + 1
		f, ok = r.formats[ct[start:]]
	}
	if !ok {
		return fmt.Errorf("unknown content type: %s", ct)
	}
	return f.marshal(w, v)
}

func (r *api[R, W]) SchemaLink(ref string) string {
	return r.config.SchemasPath + "/" + path.Base(ref) + ".json"
}

func (r *api[R, W]) TransformBody(op *Operation, status string, v any) (any, error) {
	var err error
	for _, t := range r.transformers {
		v, err = t(op, status, v)
		if err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (r *api[R, W]) HasSchemaField() bool {
	return !r.config.DisableSchemaField
}

func (r *api[R, W]) HasSchemaLink() bool {
	return !r.config.DisableSchemaLink
}

func NewAPI[R, W any](config Config, a Adapter[R, W]) API[R, W] {
	newAPI := &api[R, W]{
		config:  config,
		adapter: a,
		formats: map[string]format{},
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

	newAPI.AddFormat("application/json", "json", func(w io.Writer, v any) error {
		return json.NewEncoder(w).Encode(v)
	}, json.Unmarshal)

	encOpts := cbor.CanonicalEncOptions()
	encOpts.Time = cbor.TimeUnixDynamic
	encOpts.TimeTag = cbor.EncTagRequired
	encmode, _ := encOpts.EncMode()
	newAPI.AddFormat("application/cbor", "cbor", func(w io.Writer, v any) error {
		return encmode.NewEncoder(w).Encode(v)
	}, cbor.Unmarshal)

	if config.OpenAPIPath == "" {
		config.OpenAPIPath = "/openapi"
	}
	var specJSON []byte
	a.Handle(http.MethodGet, config.OpenAPIPath+".json", func(w W, r R) {
		a.WriteHeader(w, "Content-Type", "application/vnd.oai.openapi+json")
		if specJSON == nil {
			specJSON, _ = json.Marshal(newAPI.OpenAPI())
		}
		a.BodyWriter(w).Write(specJSON)
	})
	var specYAML []byte
	a.Handle(http.MethodGet, config.OpenAPIPath+".yaml", func(w W, r R) {
		a.WriteHeader(w, "Content-Type", "application/vnd.oai.openapi+yaml")
		if specYAML == nil {
			specYAML, _ = yaml.Marshal(newAPI.OpenAPI())
		}
		a.BodyWriter(w).Write(specYAML)
	})

	if config.DocsPath == "" {
		config.DocsPath = "/docs"
	}
	a.Handle(http.MethodGet, config.DocsPath, func(w W, r R) {
		a.WriteHeader(w, "Content-Type", "text/html")
		a.BodyWriter(w).Write([]byte(`<!doctype html>
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

	if config.SchemasPath == "" {
		config.SchemasPath = "/schemas"
	}

	rxSchema := regexp.MustCompile(`#/components/schemas/([^"]+)`)
	a.Handle(http.MethodGet, config.SchemasPath+"/{schema}.json", func(w W, r R) {
		schema := a.GetParam(r, "schema")
		a.WriteHeader(w, "Content-Type", "application/json")
		// TODO: copy & convert refs...
		b, _ := json.Marshal(config.OpenAPI.Components.Schemas.Map()[schema])

		b = rxSchema.ReplaceAll(b, []byte(config.SchemasPath+`/$1.json`))
		a.BodyWriter(w).Write(b)
	})

	if !config.DisableSchemaField {
		newAPI.transformers = append(newAPI.transformers, TransformAddSchemaField(config.SchemasPath))
	}

	return newAPI
}
