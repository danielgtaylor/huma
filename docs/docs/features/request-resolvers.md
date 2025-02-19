---
description: Custom validation and transformations for request inputs using simple Go code.
---

# Request Resolvers

## Request Resolvers { .hidden }

Sometimes the built-in validation isn't sufficient for your use-case, or you want to do something more complex with the incoming request object. This is where resolvers come in.

Any input struct can be a resolver by implementing the [`huma.Resolver`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Resolver) or [`huma.ResolverWithPath`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ResolverWithPath) interface, including embedded structs. Each resolver takes the current context and can return a list of exhaustive errors. For example:

```go title="code.go"
// MyInput demonstrates inputs/transformation
type MyInput struct {
	Host   string
	Name string `query:"name"`
}

func (m *MyInput) Resolve(ctx huma.Context) []error {
	// Get request info you don't normally have access to.
	m.Host = ctx.Host()

	// Transformations or other data validation
	m.Name = strings.Title(m.Name)

	return nil
}

// Then use it like any other input struct:
huma.Register(api, huma.Operation{
	OperationID: "list-things",
	Method:      http.MethodGet,
	Path:        "/things",
	Summary:     "Get a filtered list of things",
}, func(ctx context.Context, input *MyInput) (*YourOutput, error) {
	fmt.Printf("Host: %s\n", input.Host)
	fmt.Printf("Name: %s\n", input.Name)
})
```

It is recommended that you do not save the context object passed to the `Resolve` method for later use.

For deeply nested structs within the request body, you may not know the current location of the field being validated (e.g. it may appear in multiple places or be shared by multiple request objects). The [`huma.ResolverWithPath`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ResolverWithPath) interface provides a path prefix that can be used to generate the full path to the field being validated. It uses a [`huma.PathBuffer`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#PathBuffer) for efficient path generation reusing a shared buffer. For example:

```go title="code.go"
func (m *MyInput) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	return []error{&huma.ErrorDetail{
		Message: "Foo has a bad value",
		Location: prefix.With("foo"),
		Value: m.Foo,
	}}
}
```

!!! info "Validation Preference"

    Prefer using built-in validation over resolvers whenever possible, as it will be better documented and is also usable by OpenAPI tooling to provide a better developer experience.

### Resolver Errors

Resolvers can set errors as needed and Huma will automatically return a 400-level error response before calling your handler. This makes resolvers a good place to run additional complex validation steps so you can provide the user with a set of exhaustive errors.

```go title="code.go"
type MyInput struct {
	Host   string
}

func (m *MyInput) Resolve(ctx huma.Context) []error {
	m.Host = ctx.Host()
	if m.Host == "localhost" {
		return []error{&huma.ErrorDetail{
			Message: "Unsupported host value!",
			Location: "request.host",
			Value: m.Host,
		}}
	}
	return nil
}
```

It is also possible for resolvers to return custom HTTP status codes for the response, by returning an error which satisfies the `huma.StatusError` interface. Errors are processed in the order they are returned and the last one wins, so this feature should be used sparingly. For example:

```go title="code.go"
type MyInput struct{}

func (i *MyInput) Resolve(ctx huma.Context) []error {
	return []error{huma.Error403Forbidden("nope")}
}
```

!!! info "Why Exhaustive Errors?"

    Exhaustive errors lessen frustration for users. It's better to return three errors in response to one request than to have the user make three requests which each return a new different error.

### Unwrapping

While generally not recommended, if you need to access the underlying router-specific request and response objects, you can `Unwrap()` them using the router-specific adapter package you used to create the API instance. See the [middleware documentation](./middleware.md#unwrapping) for more information.

## Implementation Check

There is a Go trick for ensuring that a struct implements a certain interface, and you can utilize it to ensure your resolvers will be called as expected. For example:

```go title="code.go"
// Ensure MyInput implements huma.Resolver
var _ huma.Resolver = (*MyInput)(nil)
```

This creates a new `nil` pointer to your struct and assigns it to an unnamed variable of type `huma.Resolver`. It will be compiled and then thrown away during optimization. If your resolver code changes and no longer implements the interface, the code will fail to compile.

## Dive Deeper

-   How-To
    -   [Custom Validation](../how-to/custom-validation.md) includes using resolvers
-   Reference
    -   [`huma.Resolver`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Resolver) is the basic interface
    -   [`huma.ResolverWithPath`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ResolverWithPath) has a path prefix
    -   [`huma.Context`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Context) a router-agnostic request/response context
