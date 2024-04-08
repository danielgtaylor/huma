---
description: API configuration options & OpenAPI 3.1 spec generation.
---

# Configuration & Open API

## Configuration & Open API { .hidden }

Huma generates Open API 3.1 compatible JSON/YAML specs and provides rendered documentation automatically. Every operation that is registered with the API is included in the spec by default. The operation's inputs and outputs are used to generate the request and response parameters / schemas.

The [`huma.Config`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) controls where the OpenAPI, docs, and schemas are available. The default config uses `/openapi`, `/docs`, and `/schemas` respectively. You can change these to whatever you want, or disable them entirely by leaving them blank. The OpenAPI spec is available in multiple versions (to better support older tools) and in JSON or YAML:

-   OpenAPI 3.1 JSON: [http://localhost:8888/openapi.json](http://localhost:8888/openapi.json)
-   OpenAPI 3.1 YAML: [http://localhost:8888/openapi.yaml](http://localhost:8888/openapi.yaml)
-   OpenAPI 3.0.3 JSON: [http://localhost:8888/openapi-3.0.json](http://localhost:8888/openapi-3.0.json)
-   OpenAPI 3.0.3 YAML: [http://localhost:8888/openapi-3.0.yaml](http://localhost:8888/openapi-3.0.yaml)

You may want to customize the generated Open API spec. With Huma v2 you have full access and can modify it as needed in the API configuration or when registering operations. For example, to set up and then use a security scheme:

```go title="code.go"
config := huma.DefaultConfig("My API", "1.0.0")
config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {
			Type: "http",
			Scheme: "bearer",
			BearerFormat: "JWT",
		},
	}
api := humachi.New(router, config)

huma.Register(api, huma.Operation{
	OperationID: "get-greeting",
	Method:      http.MethodGet,
	Path:        "/greeting/{name}",
	Summary:     "Get a greeting",
	Security: []map[string][]string{
		{"bearer": {}},
	},
}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
	// ...
})
```

!!! info "Spec"

    See the [OpenAPI 3.1 spec](https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md) and Huma's [OpenAPI struct](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#OpenAPI) for everything that can be set and how it is expected to be used.

## OpenAPI Settings Composition

Because you have full access to the OpenAPI spec, you can compose it however you want and write convenience functions to make things more straightforward. The above example could be made easier to read:

```go title="code.go"
config := huma.DefaultConfig("My API", "1.0.0")
config = withBearerAuthScheme(config)

api := humachi.New(router, config)

huma.Register(api, withBearerAuth(huma.Operation{
	OperationID: "get-greeting",
	Method:      http.MethodGet,
	Path:        "/greeting/{name}",
	Summary:     "Get a greeting",
}), func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
	// ...
})
```

Set this up however you like. Even the [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) function can be wrapped or replaced by your organization to ensure that all operations are registered with the same settings.

## Custom OpenAPI Extensions

Custom extensions to the OpenAPI are supported via the `Extensions` field on most OpenAPI structs:

```go title="code.go"
config := huma.DefaultConfig("My API", "1.0.0")
config.Extensions = map[string]any{
	"my-extension": "my-value",
}
```

Anything in the `Extensions` map will be flattened during serialization so that its fields are peers with the `Extensions` peers in the OpenAPI spec. For example, the above would result in:

```json title="openapi.json"
{
	"openapi": "3.1.0",
	"info": {
		"title": "My API",
		"version": "1.0.0"
	},
	"my-extension": "my-value"
}
```

## Dive Deeper

-   Tutorial
    -   [Your First API](../tutorial/your-first-api.md) includes using the default config
-   Reference
    -   [`huma.Config`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) the API config
    -   [`huma.DefaultConfig`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultConfig) the default API config
    -   [`huma.OpenAPI`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#OpenAPI) the OpenAPI spec
    -   [`huma.API`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#API) the API instance
    -   [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) registers new operations
-   External Links
    -   [OpenAPI 3.1 spec](https://github.com/OAI/OpenAPI-Specification/blob/main/versions/3.1.0.md)
