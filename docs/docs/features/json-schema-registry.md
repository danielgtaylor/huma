---
description: Generate JSON Schemas from your Go structs and use them to validate requests and responses.
---

## JSON Schema

Using the default Huma config (or manually via the [`huma.SchemaLinkTransformer`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#SchemaLinkTransformer)), each resource operation returns a `describedby` HTTP link relation header which references a JSON-Schema file. These schemas use the [`config.SchemasPath`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) to serve their content. For example:

```http title="HTTP Response"
Link: </schemas/Note.json>; rel="describedby"
```

Object resources (i.e. not arrays or simple scalars) can also optionally return a `$schema` property with such a link, which enables the described-by relationship to outlive the HTTP request (i.e. saving the body to a file for later editing) and enables some editors like [VSCode](https://code.visualstudio.com/docs/languages/json#_mapping-in-the-json) to provide code completion and validation as you type.

```json title="response.json"
{
	"$schema": "http://localhost:8888/schemas/Note.json",
	"title": "I am a note title",
	"contents": "Example note contents",
	"labels": ["todo"]
}
```

Operations which accept objects as input will ignore the `$schema` property, so it is safe to submit back to the API, aka "round-trip" the data.

!!! info "Editing"

    The `$schema` field is incredibly powerful when paired with Restish's [edit](https://rest.sh/#/guide?id=editing-resources) command, giving you a quick and easy way to edit strongly-typed resources in your favorite editor.

## Schema Registry

Huma uses a customizable registry to keep track of all the schemas that have been generated from Go structs. This is used to avoid generating the same schema multiple times, and to provide a way to reference schemas by name for OpenAPI operations & hosted JSON Schemas.

The default schema implementation uses a `map` to store schemas by name,generated from the Go type name without the package name. This supports recursive schemas and generates simple names like `Thing` or `ThingList`.

!!! warning "Schema Names"

    Note that by design the default registry does **not** support multiple models with the same name in different packages. For example, adding both `foo.Thing` and `bar.Thing` will result in a conflict. You can work around this by defining a new type like `type BarThing bar.Thing` and using that instead, or using a custom [registry naming function](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultSchemaNamer).

### Custom Registry

You can create your own registry with custom behavior by implementing the [`huma.Registry`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Registry) interface and setting it on `config.OpenAPI.Components.Schemas` when creating your API.

## Dive Deeper

-   Reference
    -   [`huma.Schema`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Schema) is a JSON Schema
    -   [`huma.Registry`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Registry) generates & stores JSON Schemas
    -   [`huma.DefaultSchemaNamer`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultSchemaNamer) names schemas from types
    -   [`huma.Config`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) the API config
    -   [`huma.DefaultConfig`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultConfig) the default API config
    -   [`huma.OpenAPI`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#OpenAPI) the OpenAPI spec
    -   [`huma.Components`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Components) contains the `Schemas` registry
-   External Links
    -   [JSON Schema spec](https://json-schema.org/)
    -   [OpenAPI 3.1 Components Object](https://spec.openapis.org/oas/v3.1.0#components-object)
-   See Also
    -   [Model Validation](./model-validation.md) utility to validate custom JSON objects
