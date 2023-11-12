---
description: Set limits on request body size, timeouts, and more.
---

# Request Limits

## Deadlines & Timeouts

A combination of the server and the request context can be used to control deadlines & timeouts. Go's built-in HTTP server supports a few timeout settings:

```go title="code.go"
srv := &http.Server{
	ReadTimeout:       5 * time.Second,
	WriteTimeout:      5 * time.Second,
	IdleTimeout:       30 * time.Second,
	ReadHeaderTimeout: 2 * time.Second,
	// ...
}
```

The Huma request context (accessible via resolvers) can be used to set a read deadline, which can be used to process large or streaming inputs:

```go title="code.go"
type MyInput struct {}

func (m *MyInput) Resolve(ctx huma.Context) []error {
	ctx.SetReadDeadline(time.Now().Add(5 * time.Second))
}
```

Additionally, a `context.Context` can be used to set a deadline for dependencies like databases:

```go title="code.go"
// Create a new context with a 10 second timeout.
newCtx, cancel := context.WithTimeout(ctx, 10 * time.Second)
defer cancel()

// Use the new context for any dependencies.
result, err := myDB.Get(newCtx, /* ... */)
if err != nil {
	// Deadline may have been hit, handle it here!
}
```

## Body Size Limits

By default each operation has a 1 MiB request body size limit. This can be changed by setting `huma.Operation.MaxBodyBytes` to a different value when registering the operation. If the request body is larger than the limit then a `413 Request Entity Too Large` error will be returned.

```go title="code.go" hl_lines="6"
huma.Register(api, huma.Operation{
	OperationID:  "put-thing",
	Method:       http.MethodPut,
	Path:         "/things/{thing-id}",
	Summary:      "Put a thing by ID",
	MaxBodyBytes: 10 * 1024 * 1024, // 10 MiB
}, func(ctx context.Context, input ThingRequest) (*struct{}, error) {
	// Do nothing...
	return nil, nil
}
```

Keep in mind that the body is read into memory before being passed to the handler function.

## Dive Deeper

-   Reference
    -   [`huma.Resolver`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Resolver) is the basic interface
    -   [`huma.ResolverWithPath`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ResolverWithPath) has a path prefix
    -   [`huma.Operation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Operation) the operation
    -   [`huma.Context`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Context) a router-agnostic request/response context
-   External Links
    -   [Go Contexts](https://blog.golang.org/context) from the Go blog
    -   [`context.Context`](https://pkg.go.dev/context)
    -   [`http.Server`](https://pkg.go.dev/net/http#Server)
