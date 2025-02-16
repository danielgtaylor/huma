---
description: Group operations with common prefixes, middleware, transformers, and more.
---

# Groups

## Groups { .hidden }

Operations can be grouped under common route prefixes and share middleware, operation modifier functions, and response transformers. This is done using the `huma.Group` wrapper around a `huma.API` instance, which can then be passed to `huma.Register` and its convenience wrappers like `huma.Get`, `huma.Post`, etc.

```go
grp := huma.NewGroup(api, "/v1")
grp.UseMiddleware(authMiddleware)

huma.Get(grp, "/users", func(ctx context.Context, input *struct{}) ([]User, error) {
	// ...
})
```

The above example will register a `GET /v1/users` operation with the `authMiddleware` running before the operation handler.

!!! info "Groups & Documentation"

    Groups assume that `api.AddOperation` is invoked for any operation you want to register and have documented in the OpenAPI. This is done by default with `huma.Register` & its convenience functions. If you use custom registration functions you may need to manually add operations to the OpenAPI.

## Group Features

Groups support the following features:

-   One or more path prefixes for all operations in the group.
-   Middleware that runs before each operation in the group.
-   Operation modifiers that run at operation registration time.
-   Response transformers that run after each operation in the group.

## Middleware

Middleware functions are run before each operation handler in the group. They can be used for common tasks like authentication, logging, and error handling. Middleware functions are registered using the `UseMiddleware` method on a group.

```go
grp.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
	// Do something before the operation runs
	next(ctx)
})
```

## Operation Modifiers

Operation modifiers are functions that run at operation registration time. They can be used to modify the operation before it is registered. Operation modifiers are registered using the `UseOperationModifier` method on a group.

```go
grp.UseOperationModifier(func(op *huma.Operation) {
	op.Summary = "A summary for all operations in this group"
	op.Tags = []string{"my-tag"}
})
```

## Response Transformers

Response transformers are functions that run after each operation handler in the group. They can be used to modify the response before it is returned to the client. Response transformers are registered using the `UseResponseTransformer` method on a group.

```go
grp.UseTransformer(func(ctx huma.Context, status string, v any) (any, error) {
	// Do something with the output
	return output, nil
})
```

## Dive Deeper

-   Features
    -   [Operations](./operations.md) registration & workflows
    -   [Middleware](./middleware.md) for operations
    -   [Response Transformers](./response-transformers.md) to modify response bodies
-   Reference
    -   [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) register an operation
    -   [`huma.Middlewares`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Middlewares) list of middleware
    -   [`huma.Transformer`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Transformer) response transformers
    -   [`huma.API`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#API) the API instance
