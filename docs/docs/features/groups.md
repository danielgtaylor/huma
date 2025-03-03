---
description: Group operations with common prefixes, middleware, transformers, and more.
---

# Groups

## Groups { .hidden }

Operations can be grouped under common route prefixes and share middleware, operation modifier functions, and response transformers. This is done using the `huma.Group` wrapper around a `huma.API` instance, which can then be passed to `huma.Register` and its convenience wrappers like `huma.Get`, `huma.Post`, etc.

```go
grp := huma.NewGroup(api, "/v1")
grp.UseMiddleware(authMiddleware)

huma.Get(grp, "/users", func(ctx context.Context, input *struct{}) (*UsersResponse, error) {
	// ...
})
```

The above example will register a `GET /v1/users` operation with the `authMiddleware` running before the operation handler.

!!! info "Groups & Documentation"

    Groups assume that `huma.Register` or one of its convenience wrappers is used to register operations. If you are not, then you may need to invoke `group.DocumentOperation(*huma.Operation)` to ensure that the operation is documented correctly.

## Group Features

Groups support the following features:

-   One or more path prefixes for all operations in the group.
-   Middleware that runs before each operation in the group.
-   Operation modifiers that run at operation registration time.
-   Response transformers that run after each operation in the group.

## Prefixes

Groups can have one or more path prefixes that are prepended to all operations in the group. This is useful for grouping related operations under a common prefix and is typically done with a single prefix.

```go
grp := huma.NewGroup(api, "/prefix1", "/prefix2", "...")
```

This is just a convenience for the following equivalent code:

```go
grp := huma.NewGroup(api)
grp.UseModifier(huma.PrefixModifier("/prefix1", "/prefix2", "..."))
```

The built-in [`huma.PrefixModifier`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#PrefixModifier) will adjust the operation's ID and tags when more than one prefix is used. If you with to customize this behavior, you can write your own operation modifier.

## Middleware

Middleware functions are run before each operation handler in the group. They can be used for common tasks like authentication, logging, and error handling. Middleware functions are registered using the `UseMiddleware` method on a group.

```go
grp.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
	// Do something before the operation runs
	next(ctx)
})
```

## Operation Modifiers

Operation modifiers are functions that run at operation registration time. They can be used to modify the operation before it is registered. Operation modifiers are registered using the `UseModifier` method on a group.

```go
grp.UseModifier(func(op *huma.Operation, next func(*huma.Operation)) {
	op.Summary = "A summary for all operations in this group"
	op.Tags = []string{"my-tag"}
    next(op)
})
```

There is also a simplified form you can use:

```go
grp.UseSimpleModifier(func(op *huma.Operation) {
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

## Customizing Documentation

Groups implement [`huma.OperationDocumenter`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#OperationDocumenter) which bypasses the normal flow of documentation generation and instead calls a function. This allows you to customize the documentation for all operations in the group. You can override the `DocumentOperation` method to customize the documentation if needed:

```go
type MyGroup huma.Group

func (g *MyGroup) DocumentOperation(op *huma.Operation) {
	g.ModifyOperation(op, func(op *huma.Operation) {
		if documenter, ok := g.API.(huma.OperationDocumenter); ok {
			// Support nested operation documenters (i.e. groups of groups).
			documenter.DocumentOperation(op)
		} else {
			// Default behavior to add operations.
			if op.Hidden {
				return
			}
			g.OpenAPI().AddOperation(op)
		}
	})
}
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
    -   [`huma.OperationDocumenter`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#OperationDocumenter) to customize OpenAPI generation
    -   [`huma.API`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#API) the API instance
