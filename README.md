# Huma REST API Framework

[![CI](https://github.com/danielgtaylor/huma/workflows/CI/badge.svg?branch=master)](https://github.com/danielgtaylor/huma/actions?query=workflow%3ACI+branch%3Amaster++) [![codecov](https://codecov.io/gh/danielgtaylor/huma/branch/master/graph/badge.svg)](https://codecov.io/gh/danielgtaylor/huma) [![Docs](https://godoc.org/github.com/danielgtaylor/huma?status.svg)](https://pkg.go.dev/github.com/danielgtaylor/huma?tab=doc) [![Go Report Card](https://goreportcard.com/badge/github.com/danielgtaylor/huma)](https://goreportcard.com/report/github.com/danielgtaylor/huma)

A modern, simple, fast & opinionated REST API framework for Go. The goals of this project are to provide:

- A modern REST API backend framework for Go developers
  - Described by [OpenAPI 3](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.3.md) & [JSON Schema](https://json-schema.org/)
  - First class support for middleware, JSON, and other features
- Documentation that can't get out of date
- High-quality developer tooling

Features include:

- Declarative interface on top of [Gin](https://github.com/gin-gonic/gin)
  - Operation & model documentation
  - Request params (path, query, or header)
  - Request body
  - Responses (including errors)
- Annotated Go types for input and output models
  - Automatic input model validation
- Documentation generation using [Redoc](https://github.com/Redocly/redoc)
- Generates OpenAPI JSON for access to a rich ecosystem of tools
  - Mocks with [API Sprout](https://github.com/danielgtaylor/apisprout)
  - SDKs with [OpenAPI Generator](https://github.com/OpenAPITools/openapi-generator)
  - CLIs with [OpenAPI CLI Generator](https://github.com/danielgtaylor/openapi-cli-generator)
  - And [plenty](https://openapi.tools/) [more](https://apis.guru/awesome-openapi3/category.html)

This project was inspired by [FastAPI](https://fastapi.tiangolo.com/), [Gin](https://github.com/gin-gonic/gin), and countless others.

## Concepts & Example

REST APIs are composed of operations against resources and can include descriptions of various inputs and possible outputs. Huma uses standard Go types and a declarative API to capture those descriptions in order to provide a combination of a simple interface and idiomatic code leveraging Go's strong typing.

Let's start by building Huma's equivalent of a "Hello, world" program. First, you'll need to know a few basic things:

- What's this API called?
- How will you call the hello operation?
- What will the response of our hello operation look like?

You use Huma concepts to answer those questions, and then write your operation's handler function. Below is the full working example:

```go
package main

import (
	"net/http"

	"github.com/danielgtaylor/huma"
)

func main() {
	// Create a new router and give our API a title and version.
	r := huma.NewRouter(&huma.OpenAPI{
		Title:   "My API",
		Version: "1.0.0",
	})

	// Create the "hello" operation via `GET /hello`.
	r.Register(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/hello",
		Description: "Basic hello world",
		// Every response definition includes the HTTP status code to return, the
		// content type to use, and a description for documentation.
		Responses: []*huma.Response{
			huma.ResponseText(http.StatusOK, "Successful hello response"),
		},
		// The Handler is the operation's implementation. In this example, we
		// are just going to return the string "hello", but you could fetch
		// data from your datastore or do other things here.
		Handler: func() string {
			return "Hello, world"
		},
	})

	// Start the server on http://localhost:8888/
	r.Run("0.0.0.0:8888")
}
```

Save this file as `hello/main.go`. Run it and then try to access the API with [HTTPie](https://httpie.org/) (or curl):

```sh
# Grab reflex to enable reloading the server on code changes:
$ go get github.com/cespare/reflex

# Run the server
$ reflex -s go run hello/main.go

# Make the request (in another tab)
$ http :8888/hello
HTTP/1.1 200 OK
Content-Length: 5
Content-Type: text/plain
Date: Mon, 09 Mar 2020 04:28:13 GMT

Hello, world
```

The server works and responds as expected. Nothing too interesting here, so let's change that.

### Parameters

Huma supports three types of parameters:

- Required path parameters, e.g. `/things/{thingId}`
- Optional query string parameters, e.g. `/things?q=filter`
- Optional header parameters, e.g. `X-MyHeader: my-value`

Optional parameters require a default value.

Make the hello operation take an optional `name` query parameter with a default of `world`. Add a new `huma.QueryParam` to the operation and then update the handler function to take a `name` argument.

```go
r.Register(&huma.Operation{
	Method:      http.MethodGet,
	Path:        "/hello",
	Description: "Basic hello world",
	Params: []*huma.Param{
		huma.QueryParam("name", "Who to greet", "world"),
	},
	Responses: []*huma.Response{
		huma.ResponseText(http.StatusOK, "Successful hello response"),
	},
	Handler: func(name string) string {
		return "Hello, " + name
	},
})
```

Try making another request after saving the file (the server should automatically restart):

```sh
# Make the request without a name
$ http :8888/hello
HTTP/1.1 200 OK
Content-Length: 13
Content-Type: text/plain
Date: Mon, 09 Mar 2020 04:35:42 GMT

Hello, world

# Make the request with a name
$ http :8888/hello?name=Daniel
HTTP/1.1 200 OK
Content-Length: 13
Content-Type: text/plain
Date: Mon, 09 Mar 2020 04:35:42 GMT

Hello, Daniel
```

Nice work! Notice that `name` was a Go `string`, but it could also have been another type like `int` and it will get parsed and validated appropriately before your handler function is called.

Operating on strings is fun but let's throw some JSON into the mix next.

### Request & Response Models

Update the response to use JSON by defining a model. Models are annotated Go structures which you've probably seen before when marshalling and unmarshalling to/from JSON. Create a silly one that contains just the hello message:

```go
// HelloResponse returns the message for the hello operation.
type HelloResponse struct {
	Message string `json:"message" description:"Greeting message"`
}
```

This uses Go struct field tags to add additional information and will generate JSON-Schema for you. With just a couple small changes you now will have a JSON API:

1. Change the response type to JSON
2. Return an instance of `HelloResponse` in the handler

```go
r.Register(&huma.Operation{
	Method:      http.MethodGet,
	Path:        "/hello",
	Description: "Basic hello world",
	Params: []*huma.Param{
		huma.QueryParam("name", "Who to greet", "world"),
	},
	Responses: []*huma.Response{
		huma.ResponseJSON(http.StatusOK, "Successful hello response"),
	},
	Handler: func(name string) *HelloResponse {
		return &HelloResponse{
			Message: "Hello, " + name,
		}
	},
})
```

Try saving the file and making another request:

```sh
# Make the request and get a JSON response
$ http :8888/hello
HTTP/1.1 200 OK
Content-Length: 27
Content-Type: application/json; charset=utf-8
Date: Mon, 09 Mar 2020 05:00:14 GMT

{
    "message": "Hello, world"
}
```

Great! But that's not all! Take a look at two more automatically-generated routes. The first shows you documentation about your API, while the second is the OpenAPI 3 spec file you can use to integrate with other tooling to generate client SDKs, CLI applications, and more.

- Documenation: http://localhost:8888/docs
- OpenAPI 3 spec: http://localhost:8888/openapi.json

For the docs, you should see something like this:

<img width="1367" alt="Documentation  screenshot" src="https://user-images.githubusercontent.com/106826/76184508-746dfb00-6189-11ea-9e7d-f21ac58a2d19.png">

Request models are essentially the same. Just define an extra input argument to the handler funtion and you get automatic loading and validation.

More docs coming soon.
