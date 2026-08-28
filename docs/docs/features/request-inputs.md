---
description: Path, query, and header input parameters as well as input request body definitions & parsing.
---

# Request Inputs

## Parameters

Requests can have parameters and/or a body as input to the handler function. Inputs use standard Go structs with special fields and/or tags. Here are the available tags:

| Tag        | Description                                  | Example                  |
| ---------- |----------------------------------------------| ------------------------ |
| `path`     | Name of the path parameter                   | `path:"thing-id"`        |
| `query`    | Name of the query string parameter           | `query:"q"`              |
| `header`   | Name of the header parameter                 | `header:"Authorization"` |
| `cookie`   | Name of the cookie parameter                 | `cookie:"session"`       |
| `required` | Mark a cookie/header/query param as required | `required:"true"`        |

!!! info "Default Optionality"

    Cookier, header, and query parameters are **optional by default**. Path parameters are always required. This differs from object fields (e.g. in a request body), which are required by default unless `omitempty` or `omitzero` is used.

!!! info "Required"

    The `required` tag is discouraged and is only used for header/query params, which should generally be optional for clients to send.

### Parameter Types

The following parameter types are supported out of the box:

| Type                        | Example Inputs         |
| --------------------------- | ---------------------- |
| `bool`                      | `true`, `false`        |
| `[u]int[16/32/64]`          | `1234`, `5`, `-1`      |
| `float32/64`                | `1.234`, `1.0`         |
| `string`                    | `hello`, `t`           |
| `time.Time`                 | `2020-01-01T12:00:00Z` |
| slice, e.g. `[]int`         | `1,2,3`, `tag1,tag2`   |
| pointer, e.g. `*int`        | `1234`, or `nil`       |

For example, if the parameter is a query param and the type is `[]string` it might look like `?tags=tag1,tag2` in the URI. Query parameters also support specifying the same parameter multiple times by setting the `explode` tag, e.g. `query:"tags,explode"` would parse a query string like `?tags=tag1&tags=tag2` instead of a comma separated list. The comma separated list is faster and recommended for most use cases.

### Optional parameters

Any of the types above can be a pointer, in which case `nil` means the parameter was not provided in the request. This is useful when the zero value has meaning to your application, e.g. telling "the client did not filter" apart from "the client filtered on `false`":

```go title="code.go"
type MyInput struct {
	// nil, or a pointer to true/false.
	Active *bool `query:"active"`
	// nil, or a pointer to the tags that were sent.
	Tags *[]string `query:"tags"`
}
```

A few things to be aware of:

- Pointers work for path, query, header, and cookie parameters. Multipart form fields do not support them yet and panic at registration.
- **Pointers do not make a parameter optional.** Query, header, and cookie params are already optional by default; path params are always required. See [Optional / Required](./request-validation.md#optional-required).
- A parameter that is present but empty, e.g. `?tags=`, counts as **not provided** and leaves the pointer `nil`.
- A `default` tag always wins over `nil`, so a parameter with a default is never `nil`.
- Pointer parameters are documented as [nullable](./request-validation.md#nullable). Add `nullable:"false"` if you would rather the generated schema say e.g. `type: string` instead of `type: [string, "null"]`, since a parameter can never actually carry a literal `null` over the wire.
- Slices of pointers (`[]*string`) and multiple levels of indirection (`**string`) are not supported.

For cookies, the default behavior is to read the cookie _value_ from the request and convert it to one of the types above. If you want to access the entire cookie, you can use `http.Cookie` as the type instead:

```go title="code.go"
type MyInput struct {
	Session http.Cookie `cookie:"session"`
}
```

Then you can access e.g. `input.Session.Name` or `input.Session.Value`. Use `*http.Cookie` if you want `nil` when the cookie is absent.

### Custom wrapper types

Request parameters can be parsed into custom wrapper types, by implementing the [`ParamWrapper`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ParamWrapper) interface, which should give access to the wrapper field as a [`reflect.Value`](https://pkg.go.dev/reflect#Value). Huma parses that inner field exactly as if it were the parameter itself, so comma-separated lists, `explode`, time formats and validation all keep working.

Interface [`ParamReactor`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#ParamReactor) may optionally be implemented to define a callback to execute after a request parameter was parsed, which is where a wrapper can derive its own state from the parsed value.

To simply tell "not provided" apart from a zero value, reach for a [pointer](#optional-parameters) instead.
Wrapper types earn their keep when you want to compute something from the parsed value. For example, a set that de-duplicates a repeated parameter and answers membership in constant time:

```go
type Set[T comparable] struct {
	Values []T
	index  map[T]struct{}
}

// Document the parameter as the wrapped slice type.
func (s Set[T]) Schema(r huma.Registry) *huma.Schema {
	return huma.SchemaFromType(r, reflect.TypeFor[[]T]())
}

// Expose the slice for Huma to parse into.
// MUST have pointer receiver, and the field MUST be exported.
func (s *Set[T]) Receiver() reflect.Value {
	return reflect.ValueOf(s).Elem().Field(0)
}

// Once Huma has filled in Values, drop duplicates and build the index.
// MUST have pointer receiver.
func (s *Set[T]) OnParamSet(isSet bool, parsed any) {
	s.index = make(map[T]struct{}, len(s.Values))
	unique := s.Values[:0]
	for _, v := range s.Values {
		if _, ok := s.index[v]; ok {
			continue
		}
		s.index[v] = struct{}{}
		unique = append(unique, v)
	}
	s.Values = unique
}

func (s Set[T]) Has(v T) bool {
	_, ok := s.index[v]
	return ok
}

// Define request input with the wrapper type
type MyRequestInput struct {
	Tags Set[string] `query:"tags"`
}
```

The parameter is still documented as a plain array of strings, and for `?tags=b,a,b` the handler sees `Tags.Values` of `["b", "a"]` with `Tags.Has("a")` reporting true.

## Request Body

The special struct field `Body` will be treated as the input request body and can refer to any other type or you can embed a struct or slice inline. If the body is a pointer, then it is optional. All doc & validation tags are allowed on the body in addition to these tags:

| Tag           | Description               | Example                                  |
| ------------- | ------------------------- | ---------------------------------------- |
| `contentType` | Override the content type | `contentType:"application/my-type+json"` |
| `nameHint`    | Hint for the schema name  | `nameHint:"MyRequestBody"`                |
| `required`    | Mark the body as required | `required:"true"`                        |

`RawBody []byte` can also be used alongside `Body` to provide access to the `[]byte` used to validate & parse `Body`.

### Special Types

The following special types are supported out of the box:

| Type              | Schema                                      | Example                       |
| ----------------- | ------------------------------------------- | ----------------------------- |
| `time.Time`       | `{"type": "string", "format": "date-time"}` | `"2020-01-01T12:00:00Z"`      |
| `url.URL`         | `{"type": "string", "format": "uri"}`       | `"https://example.com"`       |
| `net.IP`          | `{"type": "string", "format": "ipv4"}`      | `"127.0.0.1"`                 |
| `netip.Addr`      | `{"type": "string", "format": "ip"}`        | `"127.0.0.1"` or `fe80::1`    |
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
		// Field content is unmarshalled and validated
		MyStruct 				  SomeStruct      `form:"my_struct" contentType:"application/json"`
		MyStructSlice 			  []SomeStruct    `form:"my_struct_slice" contentType:"application/json"`
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

Unlike other [parameters](#optional-parameters), form fields cannot be pointers — registering one panics. Use `huma.FormFile` and check its `IsSet` field for optional files, and a meaningful zero value for other fields.

Non-file fields in multipart form data can be unmarshalled from JSON and validated, by setting their content-type to `application/json`. Field content in the request must be valid JSON.


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
