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

## Field Naming

The standard `json` tag is supported and can be used to rename a field. Any field tagged with `json:"-"` will be ignored in the schema, as if it did not exist.

## Optional / Required / Nullable

Fields being optional/required is determined automatically but can be overidden as needed using the logic below:

1. Start with all fields as required.
2. If the field is a pointer, it is optional.
3. If the field uses `omitempty`, it is optional.
4. If the field has the `required:"false"` tag, it is optional.
5. If the field has the `required:"true"` tag, it is required.

Fields are not nullable by default as most programming languages do not distinguish between `null`/`nil` and undefined and nullability makes schemas significantly more complex. For most languages a field being optional is enough to use pointers and thus allow `null`/`nil` values. If you need to explicitly allow such values, you can use the `nullable:"true"` tag:

```go title="code.go"
// Make an entire struct nullable.
type MyStruct1 struct {
    _ struct{} `nullable:"true"`
    Field1 string `json:"field1"`
    Field2 string `json:"field2"`
}

// Make a specific scalar field nullable. This is *not* supported for
// slices, maps, or structs. Structs *must* use the method above.
type MyStruct2 struct {
    Field *string `json:"field" nullable:"true"`
}
```

This will cause the `MyStruct2.Field` JSON Schema `type` to be generated as `["string", "null"]` instead of just `"string"`. Also keep in mind you can always provide a [custom schema](./schema-customization.md) if the built-in features aren't exactly what you need.

## Validation Tags

The following additional tags are supported on model fields:

| Tag                  | Description                                | Example                         |
| -------------------- | ------------------------------------------ | ------------------------------- |
| `doc`                | Describe the field                         | `doc:"Who to greet"`            |
| `format`             | Format hint for the field                  | `format:"date-time"`            |
| `enum`               | A comma-separated list of possible values  | `enum:"one,two,three"`          |
| `default`            | Default value                              | `default:"123"`                 |
| `minimum`            | Minimum (inclusive)                        | `minimum:"1"`                   |
| `exclusiveMinimum`   | Minimum (exclusive)                        | `exclusiveMinimum:"0"`          |
| `maximum`            | Maximum (inclusive)                        | `maximum:"255"`                 |
| `exclusiveMaximum`   | Maximum (exclusive)                        | `exclusiveMaximum:"100"`        |
| `multipleOf`         | Value must be a multiple of this value     | `multipleOf:"2"`                |
| `minLength`          | Minimum string length                      | `minLength:"1"`                 |
| `maxLength`          | Maximum string length                      | `maxLength:"80"`                |
| `pattern`            | Regular expression pattern                 | `pattern:"[a-z]+"`              |
| `patternDescription` | Description of the pattern used for errors | `patternDescription:"alphanum"` |
| `minItems`           | Minimum number of array items              | `minItems:"1"`                  |
| `maxItems`           | Maximum number of array items              | `maxItems:"20"`                 |
| `uniqueItems`        | Array items must be unique                 | `uniqueItems:"true"`            |
| `minProperties`      | Minimum number of object properties        | `minProperties:"1"`             |
| `maxProperties`      | Maximum number of object properties        | `maxProperties:"20"`            |
| `example`            | Example value                              | `example:"123"`                 |
| `readOnly`           | Sent in the response only                  | `readOnly:"true"`               |
| `writeOnly`          | Sent in the request only                   | `writeOnly:"true"`              |
| `deprecated`         | This field is deprecated                   | `deprecated:"true"`             |
| `hidden`             | Hide field/param from documentation        | `hidden:"true"`                 |
| `dependentRequired`  | Required fields when the field is present  | `dependentRequired:"one,two"`   |

Built-in string formats include:

| Format                            | Description                     | Example                                |
| --------------------------------- | ------------------------------- | -------------------------------------- |
| `date-time`                       | Date and time in RFC3339 format | `2021-12-31T23:59:59Z`                 |
| `date-time-http`                  | Date and time in HTTP format    | `Fri, 31 Dec 2021 23:59:59 GMT`        |
| `date`                            | Date in RFC3339 format          | `2021-12-31`                           |
| `time`                            | Time in RFC3339 format          | `23:59:59`                             |
| `email` / `idn-email`             | Email address                   | `kari@example.com`                     |
| `hostname`                        | Hostname                        | `example.com`                          |
| `ipv4`                            | IPv4 address                    | `127.0.0.1`                            |
| `ipv6`                            | IPv6 address                    | `::1`                                  |
| `uri` / `iri`                     | URI                             | `https://example.com`                  |
| `uri-reference` / `iri-reference` | URI reference                   | `/path/to/resource`                    |
| `uri-template`                    | URI template                    | `/path/{id}`                           |
| `json-pointer`                    | JSON Pointer                    | `/path/to/field`                       |
| `relative-json-pointer`           | Relative JSON Pointer           | `0/1`                                  |
| `regex`                           | Regular expression              | `[a-z]+`                               |
| `uuid`                            | UUID                            | `550e8400-e29b-41d4-a716-446655440000` |

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
