---
description: Customize the generated JSON Schema for request & response bodies.
---

# Schema Customization

## Operation Schema

Schemas that are generated for input/output bodies can be customized in a couple of different ways. First, when registering your operation you can provide your own request and/or response schemas if you want to override the entire body. The automatic generation only applies when you have not provided your own schema in the OpenAPI.

```go title="code.go"
// Register an operation with a custom input body schema.
huma.Register(api, huma.Operation{
	OperationID: "my-operation",
	Method:      http.MethodPut,
	Path:        "/things/{thing-id}",
	Summary:     "Update a thing",
	RequestBody: &huma.RequestBody{
		Description: "My custom request schema",
		Content: map[string]*huma.MediaType{
			"application/json": {
				Schema: &huma.Schema{
					Type: 		 huma.TypeObject,
					Properties: map[string]*huma.Schema{
						"foo": {
							Type: huma.TypeString,
							Extensions: map[string]any{
								"x-custom-thing": "abc123",
							},
						},
					},
				},
			},
		},
	},
}, func(ctx context.Context, input *MyInput) (*MyOutput, error) {
	// Implementation goes here...
	return nil, nil
})
```

## Field Schema

Second, this can be done on a per-field basis by making a struct that implements a special interface to get a schema, allowing you to e.g. encapsulate additional functionality within that field. This is the interface:

```go title="code.go"
// SchemaProvider is an interface that can be implemented by types to provide
// a custom schema for themselves, overriding the built-in schema generation.
// This can be used by custom types with their own special serialization rules.
type SchemaProvider interface {
	Schema(r huma.Registry) *huma.Schema
}
```

The [`huma.Registry`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Registry) is passed to you and can be used to get schemas or refs for any embedded structs. Here is an example, where we want to know if a field was omitted vs. null vs. a value when sent as part of a request body. First we start by defining the custom generic struct:

```go title="code.go"
// OmittableNullable is a field which can be omitted from the input,
// set to `null`, or set to a value. Each state is tracked and can
// be checked for in handling code.
type OmittableNullable[T any] struct {
	Sent  bool
	Null  bool
	Value T
}

// UnmarshalJSON unmarshals this value from JSON input.
func (o *OmittableNullable[T]) UnmarshalJSON(b []byte) error {
	if len(b) > 0 {
		o.Sent = true
		if bytes.Equal(b, []byte("null")) {
			o.Null = true
			return nil
		}
		return json.Unmarshal(b, &o.Value)
	}
	return nil
}

// Schema returns a schema representing this value on the wire.
// It returns the schema of the contained type.
func (o OmittableNullable[T]) Schema(r huma.Registry) *huma.Schema {
	return r.Schema(reflect.TypeOf(o.Value), true, "")
}
```

This is how it can be used in an operation:

```go
type MyResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

huma.Register(api, huma.Operation{
	OperationID: "omittable",
	Method:      http.MethodPost,
	Path:        "/omittable",
	Summary:     "Omittable / nullable example",
}, func(ctx context.Context, input *struct {
	// Making the body a pointer makes it optional, as it may be `nil`.
	Body *struct {
		Name OmittableNullable[string] `json:"name,omitempty" maxLength:"10"`
	}
}) (*MyResponse, error) {
	resp := &MyResponse{}
	if input.Body == nil {
		resp.Body.Message = "Body was not sent"
	} else if !input.Body.Name.Sent {
		resp.Body.Message = "Name was omitted from the request"
	} else if input.Body.Name.Null {
		resp.Body.Message = "Name was set to null"
	} else {
		resp.Body.Message = "Name was set to: " + input.Body.Name.Value
	}
	return resp, nil
})
```

If you go to view the generated docs, you will see that the type of the `name` field is `string` and that it is optional, with a max length of 10, indicating that the custom schema was correctly used in place of one generated for the `OmittableNullable[string]` struct.

See [https://github.com/danielgtaylor/huma/blob/main/examples/omit/main.go](https://github.com/danielgtaylor/huma/blob/main/examples/omit/main.go) for a full example along with how to call it. This just scratches the surface of what's possible with custom schemas for fields.

## Dive Deeper

-   Reference
    -   [`huma.Schema`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Schema) is a JSON Schema
    -   [`huma.Registry`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Registry) generates & stores JSON Schemas
    -   [`huma.DefaultSchemaNamer`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultSchemaNamer) names schemas from types
-   External Links
    -   [JSON Schema spec](https://json-schema.org/)
    -   [OpenAPI 3.1 spec](https://spec.openapis.org/oas/v3.1.0)
-   See Also
    -   [Config & OpenAPI](./openapi-generation.md)
