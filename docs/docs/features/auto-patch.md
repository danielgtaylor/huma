---
description: Automatically generate PATCH operations for resources in your API.
---

# Auto Patch

## Auto Patch { .hidden }

If a `GET` and a `PUT` exist for the same resource, but no `PATCH` exists at server start up, then a `PATCH` operation can be generated for you to make editing more convenient for clients. You can opt-in to this behavior with the `autopatch` package:

```go
import "github.com/danielgtaylor/huma/v2/autopatch"

// ...

// Later in the code *after* registering operations...
autopatch.AutoPatch(api)
```

If the `GET` returns an `ETag` or `Last-Modified` header, then these will be used to make conditional requests on the `PUT` operation to prevent distributed write conflicts that might otherwise overwrite someone else's changes.

The following formats are supported out of the box, selected via the `Content-Type` header:

-   [JSON Merge Patch](https://datatracker.ietf.org/doc/html/rfc7386) `application/merge-patch+json`
-   [Shorthand Merge Patch](https://rest.sh/#/shorthand?id=patch-partial-update) `application/merge-patch+shorthand`
-   [JSON Patch](https://www.rfc-editor.org/rfc/rfc6902.html) `application/json-patch+json`

!!! info "Merge on Steroids"

    You can think of the Shorthand Merge Patch as an extension to the JSON merge patch with support for field paths, arrays, and a few other features. Patches like this are possible, appending an item to an array (creating it if needed):

    ```yaml
    {
    	foo.bar[]: "baz",
    }
    ```

If the `PATCH` request has no `Content-Type` header, or uses `application/json` or a variant thereof, then JSON Merge Patch is assumed.

## Disabling Auto Patch

The auto patch feature can be disabled per resource by setting metadata on an operation:

```go title="code.go" hl_lines="7-9"
// Register an operation that won't get a PATCH generated.
huma.Register(api, huma.Operation{
	OperationID: "get-greeting",
	Method:      http.MethodGet,
	Path:        "/greeting/{name}",
	Summary:     "Get a greeting",
	Metadata: map[string]interface{}{
		"autopatch": false,
	},
}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
	// ...
})
```

## Dive Deeper

-   Reference
    -   [`autopatch`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/autopatch) package
-   External Links
    -   [HTTP PATCH Method](https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods/PATCH)
    -   [RFC7386 JSON Merge Patch](https://datatracker.ietf.org/doc/html/rfc7386)
    -   [Shorthand Merge Patch](https://rest.sh/#/shorthand?id=patch-partial-update)
    -   [RFC6902 JSON Patch](https://www.rfc-editor.org/rfc/rfc6902.html)
