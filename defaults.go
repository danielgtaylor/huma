package huma

import (
	"encoding/json"
	"io"
)

// DefaultJSONFormat is the default JSON formatter that can be set in the API's
// `Config.Formats` map. This is used by the `DefaultConfig` function.
//
//	config := huma.Config{}
//	config.Formats = map[string]huma.Format{
//		"application/json": huma.DefaultJSONFormat,
//		"json":             huma.DefaultJSONFormat,
//	}
var DefaultJSONFormat = Format{
	Marshal: func(w io.Writer, v any) error {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(v)
	},
	Unmarshal: json.Unmarshal,
}

// DefaultFormats is a map of default formats that can be set in the API's
// `Config.Formats` map, used for content negotiation for marshaling and
// unmarshaling request/response bodies. This is used by the `DefaultConfig`
// function and can be modified to add or remove additional formats. For
// example, to add support for CBOR, simply import it:
//
//	import _ "github.com/danielgtaylor/huma/v2/formats/cbor"
var DefaultFormats = map[string]Format{
	"application/json": DefaultJSONFormat,
	"json":             DefaultJSONFormat,
}

// DefaultConfig returns a default configuration for a new API. It is a good
// starting point for creating your own configuration. It supports the JSON
// format out of the box. The registry uses references for structs and a link
// transformer is included to add `$schema` fields and links into responses. The
// `/openapi.[json|yaml]`, `/docs`, and `/schemas` paths are set up to serve the
// OpenAPI spec, docs UI, and schemas respectively.
//
//	// Create and customize the config (if desired).
//	config := huma.DefaultConfig("My API", "1.0.0")
//
//	// Create the API using the config.
//	router := chi.NewMux()
//	api := humachi.New(router, config)
//
// If desired, CBOR (a binary format similar to JSON) support can be
// automatically enabled by importing the CBOR package:
//
//	import _ "github.com/danielgtaylor/huma/v2/formats/cbor"
func DefaultConfig(title, version string) Config {
	schemaPrefix := "#/components/schemas/"
	schemasPath := "/schemas"

	registry := NewMapRegistry(schemaPrefix, DefaultSchemaNamer)

	return Config{
		OpenAPI: &OpenAPI{
			OpenAPI: "3.1.0",
			Info: &Info{
				Title:   title,
				Version: version,
			},
			Components: &Components{
				Schemas: registry,
			},
		},
		OpenAPIPath:   "/openapi",
		DocsPath:      "/docs",
		SchemasPath:   schemasPath,
		Formats:       DefaultFormats,
		DefaultFormat: "application/json",
		CreateHooks: []func(Config) Config{
			func(c Config) Config {
				// Add a link transformer to the API. This adds `Link` headers and
				// puts `$schema` fields in the response body which point to the JSON
				// Schema that describes the response structure.
				// This is a create hook so we get the latest schema path setting.
				linkTransformer := NewSchemaLinkTransformer(schemaPrefix, c.SchemasPath)
				c.OnAddOperation = append(c.OnAddOperation, linkTransformer.OnAddOperation)
				c.Transformers = append(c.Transformers, linkTransformer.Transform)
				return c
			},
		},
	}
}
