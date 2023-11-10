# Operations

## Operations { .hidden }

Operations are at the core of Huma. They map an HTTP method verb and resource path to a handler function with well-defined inputs and outputs. Operations are created using the [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) function:

```go
huma.Register(api, huma.Operation{
	OperationID: "your-operation-name",
	Method:      http.MethodGet,
	Path:        "/path/to/resource/{id}",
	Summary:     "A short description of the operation",
}, func(ctx context.Context, input *YourInput) (*YourOutput, error) {
	// ... Implementation goes here ...
})
```

!!! info "REST"

    If following REST-ish conventions, operation paths should be nouns, and plural if they return more than one item. Good examples: `/notes`, `/likes`, `/users/{user-id}`, `/videos/{video-id}/stats`, etc. Huma does not enforce this or care, so RPC-style paths are also fine to use. Use what works best for you and your team.

The operation handler function always has the following generic format, where `Input` and `Output` are custom structs defined by the developer that represent the entirety of the request (path/query/header params & body) and response (headers & body), respectively:

```go
func(context.Context, *Input) (*Output, error)
```

There are many options available for configuring OpenAPI settings for the operation, and custom extensions are supported as well. See the [`huma.Operation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Schema) struct for more details.

!!! info "Naming"

    Did you know? The `OperationID` is used to generate friendly CLI commands in [Restish](https://rest.sh/) and used when generating SDKs! It should be unique, descriptive, and easy to type.

## Input & Output Models

Inputs and outputs are **always** structs that represent the entirety of the incoming request or outgoing response. This is a deliberate design decision to make it easier to reason about the data flow in your application. It also makes it easier to share code as well as generate documentation and SDKs.

If your operation has no inputs or outputs, you can use `*struct{}` when registering it.

## Dive Deeper

-   Tutorial
    -   [Your First API](/tutorial/your-first-api/#operation) includes registering an operation
-   Reference
    -   [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) registers new operations
    -   [`huma.Operation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Operation) the operation
-   External Links
    -   [OpenAPI 3.1 Operation Object](https://spec.openapis.org/oas/v3.1.0#operation-object)
