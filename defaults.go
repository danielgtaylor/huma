package huma

import (
	"encoding/json"
	"errors"
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
	Unmarshal: unmarshalJSONUnique,
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
func DefaultConfig(title string, version string) Config {
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
		OpenAPIPath:                  "/openapi",
		registryConfig:               getConfig[registryConfig](registry),
		DocsPath:                     "/docs",
		DocsRenderer:                 DocsRendererStoplightElements,
		SchemasPath:                  schemasPath,
		Formats:                      DefaultFormats,
		DefaultFormat:                "application/json",
		NoFormatFallback:             false,
		RejectUnknownQueryParameters: false,
		Transformers:                 nil,
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

// countColons counts the colons outside strings in data, which for valid JSON
// is the number of object members.
func countColons(data []byte) int {
	var escaped, inString bool
	count := 0
	for _, c := range data {
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case !inString && c == ':':
			count++
		}
	}
	return count
}

// countMembers counts the object members in a value decoded into any.
func countMembers(v any) int {
	count := 0
	switch v := v.(type) {
	case map[string]any:
		count += len(v)
		for _, item := range v {
			count += countMembers(item)
		}
	case []any:
		for _, item := range v {
			count += countMembers(item)
		}
	}
	return count
}

// unmarshalJSONUnique is json.Unmarshal that also rejects duplicate object keys
// when decoding into an untyped value, as done for validation. Validation sees
// only the last duplicate while decoding into a struct merges them, so a
// duplicate could carry values that were never validated.
func unmarshalJSONUnique(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	if p, ok := v.(*any); ok && countMembers(*p) < countColons(data) {
		return errors.New("duplicate object key")
	}
	return nil
}
