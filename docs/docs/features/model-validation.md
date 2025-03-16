---
description: Validate any custom data against a JSON Schema outside of the normal HTTP request/response flow.
---

# Model Validation

## Model Validation { .hidden }

Huma includes a utility to make it a little easier to validate models outside of the normal HTTP request/response flow, for example on app startup to load example or default data and verify it is correct. This is just a thin wrapper around the built-in validation functionality, but abstracts away some of the boilerplate required for efficient operation and provides a simple API.

```go title="code.go"
type MyExample struct {
	Name string `json:"name" maxLength:"5"`
	Age int `json:"age" minimum:"25"`
}

var value any
json.Unmarshal([]byte(`{"name": "abcdefg", "age": 1}`), &value)

validator := huma.NewModelValidator()
errs := validator.Validate(reflect.TypeOf(MyExample{}), value)
if errs != nil {
	fmt.Println("Validation error", errs)
}
```

!!! warning "Concurrency"

    The `huma.ModelValidator` is **not** goroutine-safe! For more flexible validation, use the `huma.Validate` function directly and provide your own registry, path buffer, validation result struct, etc.

## Dive Deeper

-   Reference
    -   [`huma.ModelValidator`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ModelValidator) the model validator utility
-   External Links
    -   [JSON Schema spec](https://json-schema.org/)
    -   [OpenAPI 3.1 spec](https://spec.openapis.org/oas/v3.1.0)
-   See Also
    -   [Config & OpenAPI](./openapi-generation.md)
