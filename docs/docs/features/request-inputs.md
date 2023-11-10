# Request Inputs

## Parameters

Requests can have parameters and/or a body as input to the handler function. Inputs use standard Go structs with special fields and/or tags. Here are the available tags:

| Tag        | Description                           | Example                  |
| ---------- | ------------------------------------- | ------------------------ |
| `path`     | Name of the path parameter            | `path:"thing-id"`        |
| `query`    | Name of the query string parameter    | `query:"q"`              |
| `header`   | Name of the header parameter          | `header:"Authorization"` |
| `required` | Mark a query/header param as required | `required:"true"`        |

!!! info "Required"

    The `required` tag is discouraged and is only used for query/header params, which should generally be optional for clients to send.

### Parameter Types

The following parameter types are supported out of the box:

| Type                | Example Inputs         |
| ------------------- | ---------------------- |
| `bool`              | `true`, `false`        |
| `[u]int[16/32/64]`  | `1234`, `5`, `-1`      |
| `float32/64`        | `1.234`, `1.0`         |
| `string`            | `hello`, `t`           |
| `time.Time`         | `2020-01-01T12:00:00Z` |
| slice, e.g. `[]int` | `1,2,3`, `tag1,tag2`   |

For example, if the parameter is a query param and the type is `[]string` it might look like `?tags=tag1,tag2` in the URI.

## Request Body

The special struct field `Body` will be treated as the input request body and can refer to any other type or you can embed a struct or slice inline. If the body is a pointer, then it is optional. All doc & validation tags are allowed on the body in addition to these tags:

| Tag           | Description               | Example                                  |
| ------------- | ------------------------- | ---------------------------------------- |
| `contentType` | Override the content type | `contentType:"application/octet-stream"` |
| `required`    | Mark the body as required | `required:"true"`                        |

`RawBody []byte` can also be used alongside `Body` or standalone to provide access to the `[]byte` used to validate & parse `Body`, or to the raw input without any validation/parsing.

## Request Example

Here is an example request input struct, which has a path param, query param, header param, and a structured body alongside the raw body bytes:

```go title="code.go"
type MyInput struct {
	ID      string `path:"id"`
	Detail  bool   `query:"detail" doc:"Show full details"`
	Auth    string `header:"Authorization"`
	Body    MyBody
	RawBody []byte
}
```

A request to such an endpoint might look like:

```sh title="Terminal"
# Via high-level operations:
$ restish api my-op 123 --detail=true --authorization=foo <body.json

# Via URL:
$ restish api/my-op/123?detail=true -H "Authorization: foo" <body.json
```

!!! info "Uploads"

    You can use `RawBody []byte` without a corresponding `Body` field in order to support small file uploads.

## Input Composition

Because inputs are just Go structs, they are composable and reusable. For example:

```go title="code.go"
type AuthParam struct {
	Authorization string `header:"Authorization"`
}

type PaginationParams struct {
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
}

// ... Later in the code
huma.Register(api, huma.Operation{
	OperationID: "list-things",
	Method:      http.MethodGet,
	Path:        "/things",
	Summary:     "Get a filtered list of things",
}, func(ctx context.Context, input struct {
	// Embed both structs to compose your input.
	AuthParam
	PaginationParams
}) (*struct{}, error) {
	fmt.Printf("Auth: %s, Cursor: %s, Limit: %d\n", input.Authorization, input.Cursor, input.Limit)
	return nil, nil
}
```

## Dive Deeper

-   Tutorial
    -   [Your First API](../tutorial/your-first-api.md) includes registering an operation with a path param
-   Reference
    -   [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) registers new operations
    -   [`huma.Operation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Operation) the operation
-   External Links
    -   [OpenAPI 3.1 Operation Object](https://spec.openapis.org/oas/v3.1.0#operation-object)
    -   [OpenAPI 3.1 Parameter Object](https://spec.openapis.org/oas/v3.1.0#parameter-object)
