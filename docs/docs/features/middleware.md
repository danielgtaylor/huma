---
description: Add middleware to your API to enable things like authentication, logging, and more.
---

# Middleware

## Middleware { .hidden }

Huma has support for two variants of middleware:

1. Router-specific - works at the router level, i.e. before router-agnostic middleware. You can use any middleware that is implemented for your router.
2. Router-agnostic - runs in the Huma processing chain, i.e. after calls to router-specific middleware.

```mermaid
graph LR
	Request([Request])
	RouterSpecificMiddleware[Router-Specific Middleware]
	HumaMiddleware[Huma Middleware]
	OperationHandler[Operation Handler]

	Request --> RouterSpecificMiddleware
	RouterSpecificMiddleware --> HumaMiddleware
	subgraph Huma
		HumaMiddleware --> OperationHandler
	end
```

## Router-specific

Each router implementation has its own middlewares, you can use these as you normally would before creating the Huma API instance.

Chi router example:

```go title="code.go"
router := chi.NewMux()
router.Use(jwtauth.Verifier(tokenAuth))
api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))
```

Fiber router example:

```go title="code.go"
app := fiber.New()
app.Use(logger.New())
api := humafiber.New(app, huma.DefaultConfig("My API", "1.0.0"))
```

!!! info "Huma v1"

    Huma v1 middleware is compatible with Chi v4, so if you use that router with Huma v2 you can continue to use the Huma v1 middleware. See [`humachi.NewV4`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humachi#NewV4).

## Router-agnostic

You can write you own Huma middleware without any dependency to the specific router implementation. This uses the router-agnostic `huma.Context` interface, which exposes the request and response properties to your middleware.

Example:

```go title="code.go"
func MyMiddleware(ctx huma.Context, next func(huma.Context)) {
	// Set a custom header on the response.
	ctx.SetHeader("My-Custom-Header", "Hello, world!")

	// Call the next middleware in the chain. This eventually calls the
	// operation handler as well.
	next(ctx)
}

func NewHumaAPI() huma.API {
	// ...
	api := humachi.New(router, config)
	api.UseMiddleware(MyMiddleware)

	// Register the handler after UseMiddleware() for the middleware to take effect
	huma.Get(api, "/greeting/{name}", handler.GreetingGetHandler)
}
```

### Unwrapping

While generally not recommended, if you need to access the underlying router-specific request and response objects, you can `Unwrap()` them using the router-specific adapter package you used to create the API instance (e.g. `humachi.Unwrap()` for Chi or `humago.Unwrap()` for Go's `http` package):

```go title="code.go"
func MyMiddleware(ctx huma.Context, next func(huma.Context)) {
	// Unwrap the request and response objects.
	r, w := humago.Unwrap(ctx)

	// Do something with the request and response objects.
	otherMiddleware(func (_ http.Handler) {
		// Note this assumes the request/response are modified in-place.
		next(ctx)
	}).ServeHTTP(w, r)
}
```

This can be useful when migrating a large existing project to Huma as you can apply router-specific middleware to individual operations through router-agnostic middleware on the `huma.Operation.Middleware` field.

### Context Values

The `huma.Context` interface provides a `Context()` method to retrieve the underlying request `context.Context` value. This can be used to retrieve context values in middleware and operation handlers, such as request-scoped loggers, metrics, or user information.

```go title="code.go"
if v, ok := ctx.Context().Value("some-key").(string); ok {
	// Do something with `v`!
}
```

You can also wrap the `huma.Context` to provide additional or override functionality. Some utilities are provided for this, including [`huma.WithValue`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#WithValue):

```go title="code.go"
func MyMiddleware(ctx huma.Context, next func(huma.Context)) {
	// Wrap the context to add a value.
	ctx = huma.WithValue(ctx, "some-key", "some-value")

	// Call the next middleware in the chain. This eventually calls the
	// operation handler as well.
	next(ctx)
}
```

Then you can get the value in the handler context:

```go title="handler.go"
huma.Get(api, "/greeting/{name}", func(ctx context.Context, input *struct{
		Name string `path:"name" maxLength:"30" example:"world" doc:"Name to greet"`
	}) (*GreetingOutput, error) {
		// "some-value"
		ctx.Value("some-key")
		resp := &GreetingOutput{}
		resp.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)
		return resp, nil
	})
```

### Cookies

You can use the `huma.Context` interface along with [`huma.ReadCookie`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ReadCookie) or [`huma.ReadCookies`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ReadCookies) to access cookies from middleware, and can also write cookies by adding `Set-Cookie` headers in the response:

```go
func MyMiddleware(ctx huma.Context, next func(huma.Context)) {
	// Read a cookie by name.
	sessionCookie := huma.ReadCookie(ctx, "session")
	fmt.Println(sessionCookie)

	// Read all the cookies from the request.
	cookies := huma.ReadCookies(ctx)
	fmt.Println(cookies)

	// Set a cookie in the response. Using `ctx.AppendHeader` won't overwrite
	// any existing headers, for example if other middleware might also set
	// headers or if this code were moved after the `next` call and the operation
	// might set the same header. You can also call `ctx.AppendHeader` multiple
	// times to write more than one cookie.
	cookie := http.Cookie{
		Name:  "session",
		Value: "123",
	}
	ctx.AppendHeader("Set-Cookie", cookie.String())

	// Call the next middleware in the chain. This eventually calls the
	// operation handler as well.
	next(ctx)
}
```

### Errors

If your middleware encounters an error, you can stop the processing of the next middleware or operation handler by skipping the call to `next` and writing an error response.

The [`huma.WriteErr(api, ctx, status, message, ...error)`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#WriteErr) function can be used to write nice structured error responses which respect client-driven content negotiation for marshaling:

```go title="code.go"
func MyMiddleware(ctx huma.Context, next func(ctx huma.Context)) {
	// If there is a query parameter "error=true", then return an error
	if ctx.Query("error") == "true" {
		huma.WriteErr(api, ctx, http.StatusInternalServerError,
			"Some friendly message", fmt.Errorf("error detail"),
		)
		return
	}

	// Otherwise, just continue as normal.
	next(ctx)
})
```

!!! info "Error Details"

    The [`huma.ErrorDetail`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ErrorDetail) struct can be used to provide more information about the error, such as the location of the error and the value which was seen.

### Operations

You can also add router-agnostic middleware to individual operations by setting the [`huma.Operation.Middlewares`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Operation) field. This middleware will run after the router-specific middleware and before the operation handler.

```go title="code.go"
func MyMiddleware(ctx huma.Context, next func(huma.Context)) {
	// Call the next middleware in the chain. This eventually calls the
	// operation handler as well.
	next(ctx)
}

func main() {
	// ...
	api := humachi.New(router, config)

	huma.Register(api, huma.Operation{
		OperationID: "demo",
		Method:      http.MethodGet,
		Path:        "/demo",
		Middlewares: huma.Middlewares{MyMiddleware},
	}, func(ctx context.Context, input *MyInput) (*MyOutput, error) {
		// TODO: implement handler...
		return nil, nil
	})
}
```

It's also possible for global middleware to run only for certain paths by checking the request context's URL within the middleware, or by using something like the `huma.Operation.Metadata` to trigger the middleware logic using custom settings. It's up to you to decide how to structure your middleware and operations.

## Dive Deeper

-   Reference
    -   [`huma.Context`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Context) a router-agnostic request/response context
    -   [`huma.Middlewares`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Middlewares) list of middleware
    -   [`huma.ReadCookie`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ReadCookie) reads a named cookie from a request
    -   [`huma.ReadCookies`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ReadCookies) reads cookies from a request
    -   [`huma.WriteErr`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#WriteErr) function to write error responses
    -   [`huma.API`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#API) the API instance
