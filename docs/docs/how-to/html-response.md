---
description: Return rendered HTML from your API using libraries like templ or gomponents.
---

# HTML Response

While Huma is primarily focused on providing REST APIs (JSON, CBOR, etc.), it is easy to return rendered HTML for some or all of your endpoints. This can be useful for server-side rendering (SSR), simple dashboards, or integrating with tools like [HTMX](https://htmx.org/).

## Basic HTML Response

To return HTML, you can use a response struct with a `[]byte` body and set the `Content-Type` header to `text/html`.

```go title="main.go"
type MyHTMLOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

huma.Register(api, huma.Operation{
	OperationID: "get-html",
	Method:      http.MethodGet,
	Path:        "/html",
}, func(ctx context.Context, input *struct{}) (*MyHTMLOutput, error) {
	resp := &MyHTMLOutput{
		ContentType: "text/html",
		Body:        []byte("<html><body><h1>Hello World</h1></body></html>"),
	}
	return resp, nil
})
```

## Using Templ

[Templ](https://templ.guide/) is a popular type-safe HTML templating language for Go. You can use it with Huma by rendering the component to a buffer or using Huma's streaming support.

### Rendering to a Buffer

```go title="main.go"
// hello.templ
// package main
// templ Hello(name string) {
//   <div>Hello, { name }</div>
// }

func GetHello(ctx context.Context, input *struct{ Name string `query:"name"` }) (*MyHTMLOutput, error) {
	component := Hello(input.Name)
	
	buf := new(bytes.Buffer)
	if err := component.Render(ctx, buf); err != nil {
		return nil, huma.Error500InternalServerError("Error rendering template", err)
	}

	return &MyHTMLOutput{
		ContentType: "text/html",
		Body:        buf.Bytes(),
	}, nil
}
```

### Streaming with Templ

For larger templates, you might want to stream the response directly to the client.

```go title="main.go"
huma.Register(api, huma.Operation{
	OperationID: "get-html-stream",
	Method:      http.MethodGet,
	Path:        "/html-stream",
}, func(ctx context.Context, input *struct{}) (*huma.StreamResponse, error) {
	return &huma.StreamResponse{
		Body: func(hctx huma.Context) {
			hctx.SetHeader("Content-Type", "text/html")
			Hello("World").Render(ctx, hctx.BodyWriter())
		},
	}, nil
})
```

## Using Gomponents

[Gomponents](https://www.gomponents.com/) allows you to build HTML components in pure Go.

```go title="main.go"
import "github.com/maragudk/gomponents"
import "github.com/maragudk/gomponents/html"

func GetGomponents(ctx context.Context, input *struct{}) (*MyHTMLOutput, error) {
	node := Doctype(
		HTML(
			Head(TitleEl(Text("Gomponents & Huma"))),
			Body(H1(Text("Hello from Gomponents"))),
		),
	)

	buf := new(bytes.Buffer)
	if err := node.Render(buf); err != nil {
		return nil, huma.Error500InternalServerError("Error rendering components", err)
	}

	return &MyHTMLOutput{
		ContentType: "text/html",
		Body:        buf.Bytes(),
	}, nil
}
```

## Content Negotiation for HTML

You can also use Huma's content negotiation to return either JSON or HTML depending on the client's `Accept` header. See [Serialization](../features/response-serialization.md) and [Transformers](../features/response-transformers.md) for more advanced use cases.

## Full Example

You can find a full runnable example of an HTML endpoint in the [examples/html](https://github.com/danielgtaylor/huma/tree/main/examples/html) directory of the Huma repository.
