---
description: Require fields conditionally when other fields are present
---

# Conditional Fields

Some fields might be required only when other fields are present. This is know as `dependentRequired` in the [JSON Schema](https://json-schema.org/understanding-json-schema/reference/conditionals#dependentRequired).

## Struct Tag

In Huma the `dependentRequired` tag is supported to apply conditional requirements to fields, as per the example below:

```go title="example.go"
type MyInput struct {
    Value      string `json:"value,omitempty" dependentRequired:"dependent1,dependent2"`
    Dependent1 string `json:"dependent1,omitempty"`
    Dependent2 string `json:"dependent2,omitempty"`
}
```

In the example above, all the fields are optional but, if `value` is sent, than both `dependent1` and `dependent2` must also be sent.

## Schema

It is also possible to change in the schema directly without using the struct tags. To do this, one must set the
property `DependentRequired` in the desired schema to a `map[string][]string` where the key of the map is the field
where the struct tag would be created, and the slice of strings is the dependent fields.
