![Huma Rest API Framework](https://user-images.githubusercontent.com/106826/76379557-9e502a80-630d-11ea-9c7d-f6426076a47c.png)

[![CI](https://github.com/danielgtaylor/huma/workflows/CI/badge.svg?branch=master)](https://github.com/danielgtaylor/huma/actions?query=workflow%3ACI+branch%3Amaster++) [![codecov](https://codecov.io/gh/danielgtaylor/huma/branch/master/graph/badge.svg)](https://codecov.io/gh/danielgtaylor/huma) [![Docs](https://godoc.org/github.com/danielgtaylor/huma?status.svg)](https://pkg.go.dev/github.com/danielgtaylor/huma?tab=doc) [![Go Report Card](https://goreportcard.com/badge/github.com/danielgtaylor/huma)](https://goreportcard.com/report/github.com/danielgtaylor/huma)

A modern, simple, fast & opinionated REST API framework for Go with batteries included. Pronounced IPA: [/'hjuːmɑ/](https://en.wiktionary.org/wiki/Wiktionary:International_Phonetic_Alphabet). The goals of this project are to provide:

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
  - Response headers
- Default middleware
  - Automatic recovery from panics
  - Automatically handle CORS headers
  - Structured logging middleware using [Zap](https://github.com/uber-go/zap)
- Annotated Go types for input and output models
  - Automatic input model validation
- Dependency injection for loggers, datastores, etc
- Documentation generation using [Redoc](https://github.com/Redocly/redoc)
- CLI built-in, configured via arguments or environment variables
- Generates OpenAPI JSON for access to a rich ecosystem of tools
  - Mocks with [API Sprout](https://github.com/danielgtaylor/apisprout)
  - SDKs with [OpenAPI Generator](https://github.com/OpenAPITools/openapi-generator)
  - CLIs with [OpenAPI CLI Generator](https://github.com/danielgtaylor/openapi-cli-generator)
  - And [plenty](https://openapi.tools/) [more](https://apis.guru/awesome-openapi3/category.html)

This project was inspired by [FastAPI](https://fastapi.tiangolo.com/), [Gin](https://github.com/gin-gonic/gin), and countless others. Look at the [benchmarks](https://github.com/danielgtaylor/huma/tree/master/benchmark) to see how Huma compares.

## Concepts & Example

REST APIs are composed of operations against resources and can include descriptions of various inputs and possible outputs. Huma uses standard Go types and a declarative API to capture those descriptions in order to provide a combination of a simple interface and idiomatic code leveraging Go's speed and strong typing.

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
		Title:   "Hello API",
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

	// Start the server.
	r.Run()
}
```

Save this file as `hello/main.go`. Run it and then try to access the API with [HTTPie](https://httpie.org/) (or curl):

```sh
# Grab reflex to enable reloading the server on code changes:
$ go get github.com/cespare/reflex

# Run the server (default host/port is 0.0.0.0:8888, see --help for options)
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

TODO: Request model example

#### Model Tags

The standard `json` tag is supported and can be used to rename a field and mark fields as optional using `omitempty`. The following additional tags are supported on model fields:

| Tag                | Description                               | Example                      |
| ------------------ | ----------------------------------------- | ---------------------------- |
| `description`      | Describe the field                        | `description:"Who to greet"` |
| `format`           | Format hint for the field                 | `format:"date-time"`         |
| `enum`             | A comma-separated list of possible values | `enum:"one,two,three"`       |
| `default`          | Default value                             | `default:"123"`              |
| `minimum`          | Minimum (inclusive)                       | `minimum:"1"`                |
| `exclusiveMinimum` | Minimum (exclusive)                       | `exclusiveMinimum:"0"`       |
| `maximum`          | Maximum (inclusive)                       | `maximum:"255"`              |
| `exclusiveMaximum` | Maximum (exclusive)                       | `exclusiveMaximum:"100"`     |
| `multipleOf`       | Value must be a multiple of this value    | `multipleOf:"2"`             |
| `minLength`        | Minimum string length                     | `minLength:"1"`              |
| `maxLength`        | Maximum string length                     | `maxLength:"80"`             |
| `pattern`          | Regular expression pattern                | `pattern:"[a-z]+"`           |
| `minItems`         | Minimum number of array items             | `minItems:"1"`               |
| `maxItems`         | Maximum number of array items             | `maxItems:"20"`              |
| `uniqueItems`      | Array items must be unique                | `uniqueItems:"true"`         |
| `minProperties`    | Minimum number of object properties       | `minProperties:"1"`          |
| `maxProperties`    | Maximum number of object properties       | `maxProperties:"20"`         |

### Dependencies

Huma includes a dependency injection system that can be used to pass additional arguments to operation handler functions. You can register global dependencies (ones that do not change from request to request) or contextual dependencies (ones that change with each request).

Global dependencies are created by just setting some value, while contextual dependencies are implemented using a function that returns the value of the form `func (deps..., params...) (headers..., *YourType, error)` where the value you want injected is of `*YourType` and the function arguments can be any previously registered dependency types or one of the hard-coded types:

- `huma.ContextDependency()` the current context (returns `*gin.Context`)
- `huma.OperationDependency()` the current operation (returns `*huma.Operation`)

```go
// Register a new database connection dependency
db := &huma.Dependency{
	Value: db.NewConnection(),
}

// Register a new request logger dependency. This is contextual because we
// will print out the requester's IP address with each log message.
type MyLogger struct {
	Info: func(msg string),
}

logger := &huma.Dependency{
	Dependencies: []*huma.Dependency{huma.ContextDependency()},
	Value: func(c *gin.Context) (*MyLogger, error) {
		return &MyLogger{
			Info: func(msg string) {
				fmt.Printf("%s [ip:%s]\n", msg, c.Request.RemoteAddr)
			},
		}, nil
	},
}

// Use them in any handler by adding them to both `Depends` and the list of
// handler function arguments.
r.Register(&huma.Operation{
	// ...
	Dependencies: []*huma.Dependency{db, logger},
	Handler: func(db *db.Connection, log *MyLogger) string {
		log.Info("test")
		item := db.Fetch("query")
		return item.ID
	}
})
```

Note that global dependencies cannot be functions. You can wrap them in a struct as a workaround.

## How it Works

Huma's philosophy is to make it harder to make mistakes by providing tools that reduce duplication and encourage practices which make it hard to forget to update some code.

An example of this is how handler functions **must** declare all headers that they return and which responses may send those headers. You simply **cannot** return from the function without considering the values of each of those headers. If you set one that isn't appropriate for the response you return, Huma will let you know.

How does it work? Huma asks that you give up one compile-time static type check for handler function signatures and instead let it be a runtime startup check. Using a small amount of reflection, Huma can then verify the function signatures, inject depdencies and parameters, and handle responses and headers as well as making sure that all matches the declared operation.

By strictly enforcing this runtime interface you get several advantages. No more out of date API description. No more out of date documenatation. No more out of date SDKs or CLIs. Your entire ecosystem of tooling is driven off of one simple backend implementation. Stuff just works.

More docs coming soon.
