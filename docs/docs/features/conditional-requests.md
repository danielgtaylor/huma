---
description: Enable cache use & prevent distributed concurrent write conflicts with industry-standard headers.
---

# Conditional Requests

## Conditional Requests { .hidden }

There are built-in utilities for handling [conditional requests](https://developer.mozilla.org/en-US/docs/Web/HTTP/Conditional_requests), which serve two broad purposes:

1. Sparing bandwidth on reading a document that has not changed, i.e. "only send if the version is different from what I already have".
2. Preventing multiple writers from clobbering each other's changes, i.e. "only save if the version on the server matches what I saw last".

Adding support for handling conditional requests requires four steps:

1. Import the [`github.com/danielgtaylor/huma/v2/conditional`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/conditional) package.
2. (optional) Add the response definition (`304 Not Modified` for reads or `412 Precondition Failed` for writes)
3. Add `conditional.Params` to your input struct.
4. Check if conditional params were passed and handle them. The `HasConditionalParams()` and `PreconditionFailed(...)` methods can help with this.

## Example

Implementing a conditional read might look like:

```go
huma.Register(api, huma.Operation{
	OperationID: "get-resource",
	Method:      http.MethodGet,
	Path:        "/resource",
	Summary:     "Get a resource",
}, func(ctx context.Context, input struct {
	conditional.Params
}) (*YourOutput, error) {
	if input.HasConditionalParams() {
		// TODO: Get the ETag and last modified time from the resource.
		etag := ""
		modified := time.Time{}

		// If preconditions fail, abort the request processing. Response status
		// codes are already set for you, but you can optionally provide a body.
		// Returns an HTTP 304 not modified.
		if err := input.PreconditionFailed(etag, modified); err != nil {
			return err
		}

		// Otherwise do the normal request processing here...
		// ...
	}
})
```

!!! info "Conditional Request Efficiency"

    Note that it is more efficient to construct custom DB queries to handle conditional requests, however Huma is not aware of your database. The built-in conditional utilities are designed to be generic and work with any data source, and are a quick and easy way to get started with conditional request handling.

## Dive Deeper

-   Reference
    -   [`conditional`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/conditional) package
    -   [`conditional.Params`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/conditional/Params)
-   External Links
    -   [Conditional Requests](https://developer.mozilla.org/en-US/docs/Web/HTTP/Conditional_requests)
