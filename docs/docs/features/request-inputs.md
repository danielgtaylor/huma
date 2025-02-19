---
description: Path, query, and header input parameters as well as input request body definitions & parsing.
---

# Request Inputs

## Parameters

Requests can have parameters and/or a body as input to the handler function. Inputs use standard Go structs with special fields and/or tags. Here are the available tags:

| Tag        | Description                           | Example                  |
| ---------- | ------------------------------------- | ------------------------ |
| `path`     | Name of the path parameter            | `path:"thing-id"`        |
| `query`    | Name of the query string parameter    | `query:"q"`              |
| `header`   | Name of the header parameter          | `header:"Authorization"` |
| `cookie`   | Name of the cookie parameter          | `cookie:"session"`       |
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

For example, if the parameter is a query param and the type is `[]string` it might look like `?tags=tag1,tag2` in the URI. Query parameters also support specifying the same parameter multiple times by setting the `explode` tag, e.g. `query:"tags,explode"` would parse a query string like `?tags=tag1&tags=tag2` instead of a comma separated list. The comma separated list is faster and recommended for most use cases.

For cookies, the default behavior is to read the cookie _value_ from the request and convert it to one of the types above. If you want to access the entire cookie, you can use `http.Cookie` as the type instead:

```go title="code.go"
type MyInput struct {
	Session http.Cookie `cookie:"session"`
}
```

Then you can access e.g. `input.Session.Name` or `input.Session.Value`.

### Custom wrapper types

Request parameters can be parsed into custom wrapper types, by implementing the [`ParamWrapper`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ParamWrapper) interface, which should give access to the wrapper field as a [`reflect.Value`](https://pkg.go.dev/reflect#Value).

Interface [`ParamReactor`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ParamReactor) may optionally be implemented to define a callback to execute after a request parameter was parsed.

Example usage with a custom wrapper to handle null query parameters:

```go
type OptionalParam[T any] struct {
	Value T
	IsSet bool
}

// Define schema to use wrapped type
func (o OptionalParam[T]) Schema(r huma.Registry) *huma.Schema {
	return huma.SchemaFromType(r, reflect.TypeOf(o.Value))
}

// Expose wrapped value to receive parsed value from Huma
// MUST have pointer receiver
func (o *OptionalParam[T]) Receiver() reflect.Value {
	return reflect.ValueOf(o).Elem().Field(0)
}

// React to request param being parsed to update internal state
// MUST have pointer receiver
func (o *OptionalParam[T]) OnParamSet(isSet bool, parsed any) {
	o.IsSet = isSet
}

// Define request input with the wrapper type
type MyRequestInput struct {
    MaybeText OptionalParam[string] `query:"text"`
}
```

## Request Body

The special struct field `Body` will be treated as the input request body and can refer to any other type or you can embed a struct or slice inline. If the body is a pointer, then it is optional. All doc & validation tags are allowed on the body in addition to these tags:

| Tag           | Description               | Example                                  |
| ------------- | ------------------------- | ---------------------------------------- |
| `contentType` | Override the content type | `contentType:"application/my-type+json"` |
| `required`    | Mark the body as required | `required:"true"`                        |

`RawBody []byte` can also be used alongside `Body` to provide access to the `[]byte` used to validate & parse `Body`.

### Special Types

The following special types are supported out of the box:

| Type              | Schema                                      | Example                       |
| ----------------- | ------------------------------------------- | ----------------------------- |
| `time.Time`       | `{"type": "string", "format": "date-time"}` | `"2020-01-01T12:00:00Z"`      |
| `url.URL`         | `{"type": "string", "format": "uri"}`       | `"https://example.com"`       |
| `net.IP`          | `{"type": "string", "format": "ipv4"}`      | `"127.0.0.1"`                 |
| `netip.Addr`      | `{"type": "string", "format": "ipv4"}`      | `"127.0.0.1"`                 |
| `json.RawMessage` | `{}`                                        | `["whatever", "you", "want"]` |

You can override this default behavior if needed as described in [Schema Customization](./schema-customization.md) and [Request Validation](./request-validation.md), e.g. setting a custom `format` tag for IPv6.

### Other Body Types

Sometimes, you want to bypass the normal body parsing and instead read the raw body contents directly. This is useful for unstructured data, file uploads, or other binary data. You can use `RawBody []byte` **without** a `Body` field to access the raw body bytes without any parsing/validation being applied. For example, to accept some `text/plain` input:

```go title="code.go"
huma.Register(api, huma.Operation{
	OperationID: "post-plain-text",
	Method:      http.MethodPost,
	Path:        "/text",
	Summary:     "Example to post plain text input",
}, func(ctx context.Context, input *struct {
	RawBody []byte `contentType:"text/plain"`
}) (*struct{}, error) {
	fmt.Println("Got input:", input.RawBody)
	return nil, nil
}
```

This enables you to also do your own parsing of the input, if needed.

### Multipart Form Data

Multipart form data is supported by using a `RawBody` with a type of [`multipart.Form`](https://pkg.go.dev/mime/multipart#Form) in the input struct. This will parse the request using Go standard library multipart processing implementation.

For example:

```go title="multipart.go"
huma.Register(api, huma.Operation{
	OperationID: "upload-files",
    Method:      http.MethodPost,
    Path:        "/upload",
    Summary:     "Example to upload a file",
}, func(ctx context.Context, input *struct {
    RawBody multipart.Form
}) (*struct{}, error) {
    // Process multipart form here.
	for name, _ := range input.RawBody.File {
	    fmt.Printf("Obtained file with name '%s'", name)
	}
	for name, val := range input.RawBody.Value {
	    fmt.Printf("Obtained value with name '%s' and value '%s'", name, val)
	}
    return nil, nil
})
```

This will be useful for supporting file uploads. Moreover, Huma can process files and values from the multipart form into a struct for you. In this case, you should define what the processed struct should look like:

```go title="multipart_form_files.go"
huma.Register(api, huma.Operation{
	OperationID: "upload-and-decode-files"
	Method:      http.MethodPost,
	Path:        "/upload",
}, func(ctx context.Context, input *struct {
	RawBody huma.MultipartFormFiles[struct {
		MyFile                    huma.FormFile   `form:"file" contentType:"text/plain" required:"true"`
		SomeOtherFiles            []huma.FormFile `form:"other-files" contentType:"text/plain" required:"true"`
		NoTagBindingFile          huma.FormFile   `contentType:"text/plain"`
		MyGreeting                string          `form:"greeting", minLength:"6"`
		SomeNumbers               []int           `form:"numbers"`
		NonTaggedValuesAreIgnored string  // ignored
	}]
}) (*struct{}, error) {
	// The raw multipart.Form body is again available under input.RawBody.Form.
	// E.g. input.RawBody.Form.File("file")
	// E.g. input.RawBody.Form.Value("greeting")

	// The processed input struct is available under input.RawBody.Data().
	formData := input.RawBody.Data()

	// Non-files are available and validated if they have a "form" tag
	fmt.Println(formData.MyGreeting)
	fmt.Println("These are your numbers:")
	for _, n := range formData.SomeNumbers {
		fmt.Println(n)
	}

	// Non-files without "form" tag are not available
	if formData.NonTaggedValuesAreIgnored != nil {
		panic("This should not happen")
	}

	// Process files here.
	b, err := io.ReadAll(formData.MyFile)
	fmt.Println(string(b))

	for _, f := range formData.SomeOtherFiles {
		b, err := io.ReadAll(f)
		fmt.Println(string(b))
	}

	// Flag for checking optional file existence.
	if formData.NoTagBindingFile.IsSet {
		fmt.Println("The form contained a file entry with name 'NoTagBinding'!")
	}
	return nil, nil
})
```

The files are decoded according to the specified contentType. If no contentType is provided, it defaults to `application/octet-stream`.

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
}, func(ctx context.Context, input *struct {
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
