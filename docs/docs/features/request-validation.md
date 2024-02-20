---
description: Built-in JSON Schema validation rules for request parameters and bodies.
---

# Request Validation

## Request Validation { .hidden }

Go struct tags are used to annotate inputs/output structs with information that gets turned into [JSON Schema](https://json-schema.org/) for documentation and validation. For example:

```go title="code.go"
type Person struct {
    Name string `json:"name" doc:"Person's name" minLength:"1" maxLength:"80"`
    Age  uint   `json:"age,omitempty" doc:"Person's age" maximum:"120"`
}
```

The standard `json` tag is supported and can be used to rename a field and mark fields as optional using `omitempty`. The following additional tags are supported on model fields:

| Tag                | Description                               | Example                  |
| ------------------ | ----------------------------------------- | ------------------------ |
| `doc`              | Describe the field                        | `doc:"Who to greet"`     |
| `format`           | Format hint for the field                 | `format:"date-time"`     |
| `enum`             | A comma-separated list of possible values | `enum:"one,two,three"`   |
| `default`          | Default value                             | `default:"123"`          |
| `minimum`          | Minimum (inclusive)                       | `minimum:"1"`            |
| `exclusiveMinimum` | Minimum (exclusive)                       | `exclusiveMinimum:"0"`   |
| `maximum`          | Maximum (inclusive)                       | `maximum:"255"`          |
| `exclusiveMaximum` | Maximum (exclusive)                       | `exclusiveMaximum:"100"` |
| `multipleOf`       | Value must be a multiple of this value    | `multipleOf:"2"`         |
| `minLength`        | Minimum string length                     | `minLength:"1"`          |
| `maxLength`        | Maximum string length                     | `maxLength:"80"`         |
| `pattern`          | Regular expression pattern                | `pattern:"[a-z]+"`       |
| `minItems`         | Minimum number of array items             | `minItems:"1"`           |
| `maxItems`         | Maximum number of array items             | `maxItems:"20"`          |
| `uniqueItems`      | Array items must be unique                | `uniqueItems:"true"`     |
| `minProperties`    | Minimum number of object properties       | `minProperties:"1"`      |
| `maxProperties`    | Maximum number of object properties       | `maxProperties:"20"`     |
| `example`          | Example value                             | `example:"123"`          |
| `readOnly`         | Sent in the response only                 | `readOnly:"true"`        |
| `writeOnly`        | Sent in the request only                  | `writeOnly:"true"`       |
| `deprecated`       | This field is deprecated                  | `deprecated:"true"`      |

Parameters have some additional validation tags:

| Tag      | Description                       | Example         |
| -------- | --------------------------------- | --------------- |
| `hidden` | Hide parameter from documentation | `hidden:"true"` |

## Strict vs. Loose Field Validation

By default, Huma is strict about which fields are allowed in an object, making use of the `additionalProperties: false` JSON Schema setting. This means if a client sends a field that is not defined in the schema, the request will be rejected with an error. This can help to prevent typos and other issues and is recommended for most APIs.

If you need to allow additional fields, for example when using a third-party service which will call your system and you only care about a few fields, you can use the `additionalProperties:"true"` field tag on the struct by assigning it to a dummy `_` field.

```go title="code.go"
type PartialInput struct {
	_      struct{} `json:"-" additionalProperties:"true"`
	Field1 string   `json:"field1"`
	Field2 bool     `json:"field2"`
}
```

!!! info "Note"

    The use of `struct{}` is optional but efficient. It is used to avoid allocating memory for the dummy field as an empty object requires no space.

## Advanced Validation

When using custom JSON Schemas, i.e. not generated from Go structs, it's possible to utilize a few more validation rules. The following schema fields are respected by the built-in validator:

-   `not` for negation
-   `oneOf` for exclusive inputs
-   `anyOf` for matching one-or-more
-   `allOf` for schema unions

See [`huma.Schema`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Schema) for more information. Note that it may be easier to use a custom [resolver](./request-resolvers.md) to implement some of these rules.

## Dive Deeper

-   Tutorial
    -   [Your First API](../tutorial/your-first-api.md) includes string length validation
-   Reference
    -   [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) registers new operations
    -   [`huma.Operation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Operation) the operation
-   External Links
    -   [JSON Schema Validation](https://datatracker.ietf.org/doc/html/draft-bhutton-json-schema-validation-00)
    -   [OpenAPI 3.1 Schema Object](https://spec.openapis.org/oas/v3.1.0#schema-object)
    -   [OpenAPI 3.1 Operation Object](https://spec.openapis.org/oas/v3.1.0#operation-object)
