---
description: Define and write the response status code, headers, and body.
---

# Response Outputs

Responses can have an optional status code, headers, and/or body. Like inputs, they use standard Go structs which describe the _entirety_ of the response.

## Status Code

Huma uses the following default response status codes:

-   `200` for responses with bodies
-   `204` for responses without a body

You can override this behavior in two ways. The first is by setting [`huma.Operation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Operation) `DefaultStatus` field at operation registration time.

```go title="code.go"
// Register an operation with a default status code of 201.
huma.Register(api, huma.Operation{
	OperationID:  "create-thing",
	Method:       http.MethodPost,
	Path:         "/things",
	Summary:      "Create a thing",
	DefaultStatus: 201,
}, func(ctx context.Context, input ThingRequest) (*struct{}, error) {
	// Do nothing...
	return nil, nil
}
```

If the response code needs to be **dynamic**, you can use the special `Status` field in your response struct. This is not recommended, but is available if needed.

```go title="code.go"
type ThingResponse struct {
	Status int
}

huma.Register(api, huma.Operation{
	OperationID:  "get-thing",
	Method:       http.MethodGet,
	Path:         "/things/{thing-id}",
	Summary:      "Get a thing by ID",
}, func(ctx context.Context, input ThingRequest) (*struct{}, error) {
	// Create a response and set the dynamic status
	resp := &ThingResponse{}
	if input.ID < 500 {
		resp.Status = 200
	} else {
		// This is a made-up status code used for newer things.
		resp.Status = 250
	}
	return resp, nil
}
```

!!! info "Dynamic Status"

    It is much more common to set the default status code than to need a `Status` field in your response struct!

## Headers

Headers are set by fields on the response struct. Here are the available tags:

| Tag          | Description                 | Example                                      |
| ------------ | --------------------------- | -------------------------------------------- |
| `header`     | Name of the response header | `header:"Authorization"`                     |
| `timeFormat` | Format of a `time.Time`     | `timeFormat:"Mon, 02 Jan 2006 15:04:05 GMT"` |

Here's an example of a response with several headers of different types:

```go title="code.go"
// Example struct with several headers
type MyOutput struct {
	ContentType  string    `header:"Content-Type"`
	LastModified time.Time `header:"Last-Modified"`
	MyHeader     int       `header:"My-Header"`
}
```

If the field type implements the [`fmt.Stringer`](https://pkg.go.dev/fmt#Stringer) interface then that will be used to convert the value to a string.

### Set vs. Append

By default, headers are set on the response, which overwrites any existing header of the same name. If you want to append to an existing header, you can use an array of values instead of a single value.

```go title="code.go"
type MyOutput struct {
	MyHeader []string `header:"My-Header"`
}
```

If you want to append just one header, you can use a slice with a single value.

### Cookies

You can set cookies in the response by using the [`Set-Cookie`](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Set-Cookie) header. The [`http.Cookie`](https://pkg.go.dev/net/http#Cookie) type can be used to represent the cookie without needing to manually convert it to a string.

```go title="code.go"
type MyOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

huma.Register(api, huma.Operation{
	OperationID: "set-cookie",
	Method:      http.MethodGet,
	Path:        "/set-cookie",
	Summary:     "Set a cookie",
}, func(ctx context.Context, *struct{}) (*MyOutput, error) {
	// Create a response and set the cookie
	resp := &MyOutput{
		SetCookie: http.Cookie{
			Name:  "session",
			Value: "123",
		},
	}
	return resp, nil
}
```

You can set multiple cookies by using a slice like `[]http.Cookie` instead.

## Body

The special struct field `Body` will be treated as the response body and can refer to any other type or you can embed a struct or slice inline. A default `Content-Type` header will be set if none is present, selected via client-driven content negotiation with the server based on the registered serialization types.

Example:

```go title="code.go" hl_lines="6"
type MyBody struct {
	Name string `json:"name"`
}

type MyOutput struct {
	Body MyBody
}
```

Use a type of `[]byte` to bypass [serialization](./response-serialization.md).

```go title="code.go"
type MyOutput struct {
	Body []byte
}
```

You can also stream the response body, see [streaming](./response-streaming.md) for more details.

## Dive Deeper

-   Reference
    -   [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) registers new operations
    -   [`huma.Operation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Operation) the operation
-   External Links
    -   [HTTP Status Codes](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status)
