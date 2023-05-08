package huma

import (
	"encoding/json"
	"io"

	"github.com/fxamacker/cbor/v2"
)

// DefaultJSONFormat is the default JSON formatter that can be set in the API's
// `Config.Formats` map.
var DefaultJSONFormat = Format{
	Marshal: func(w io.Writer, v any) error {
		return json.NewEncoder(w).Encode(v)
	},
	Unmarshal: json.Unmarshal,
}

var cborEncMode, _ = cbor.EncOptions{
	// Canonical enc opts
	Sort:          cbor.SortCanonical,
	ShortestFloat: cbor.ShortestFloat16,
	NaNConvert:    cbor.NaNConvert7e00,
	InfConvert:    cbor.InfConvertFloat16,
	IndefLength:   cbor.IndefLengthForbidden,
	// Time handling
	Time:    cbor.TimeUnixDynamic,
	TimeTag: cbor.EncTagRequired,
}.EncMode()

// DefaultCBORFormat is the default CBOR formatter that can be set in the API's
// `Config.Formats` map.
var DefaultCBORFormat = Format{
	Marshal: func(w io.Writer, v any) error {
		return cborEncMode.NewEncoder(w).Encode(v)
	},
	Unmarshal: cbor.Unmarshal,
}

func DefaultConfig(title, version string) Config {
	schemaPrefix := "#/components/schemas/"
	schemasPath := "/schemas"

	registry := NewMapRegistry(schemaPrefix, DefaultSchemaNamer)

	linkTransformer := NewSchemaLinkTransformer(schemaPrefix, schemasPath)

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
			OnAddOperation: []AddOpFunc{
				linkTransformer.OnAddOperation,
			},
		},
		OpenAPIPath: "/openapi",
		DocsPath:    "/docs",
		SchemasPath: schemasPath,
		Formats: map[string]Format{
			"application/json": DefaultJSONFormat,
			"json":             DefaultJSONFormat,
			"application/cbor": DefaultCBORFormat,
			"cbor":             DefaultCBORFormat,
		},
		Transformers: []Transformer{
			linkTransformer.Transform,
		},
	}
}
