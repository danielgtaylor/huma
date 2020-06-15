![Huma Rest API Framework](https://user-images.githubusercontent.com/106826/78105564-51102780-73a6-11ea-99ff-84d6c1b3e8df.png)

[![HUMA Powered](https://img.shields.io/badge/Powered%20By-HUMA-f40273)](https://huma.rocks/) [![CI](https://github.com/danielgtaylor/huma/workflows/CI/badge.svg?branch=master)](https://github.com/danielgtaylor/huma/actions?query=workflow%3ACI+branch%3Amaster++) [![codecov](https://codecov.io/gh/danielgtaylor/huma/branch/master/graph/badge.svg)](https://codecov.io/gh/danielgtaylor/huma) [![Docs](https://godoc.org/github.com/danielgtaylor/huma?status.svg)](https://pkg.go.dev/github.com/danielgtaylor/huma?tab=doc) [![Go Report Card](https://goreportcard.com/badge/github.com/danielgtaylor/huma)](https://goreportcard.com/report/github.com/danielgtaylor/huma)

A modern, simple, fast & opinionated REST API framework for Go with batteries included. Pronounced IPA: [/'hjuːmɑ/](https://en.wiktionary.org/wiki/Wiktionary:International_Phonetic_Alphabet). The goals of this project are to provide:

- A modern REST API backend framework for Go developers
  - Described by [OpenAPI 3](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.3.md) & [JSON Schema](https://json-schema.org/)
  - First class support for middleware, JSON, and other features
- Guard rails to prevent common mistakes
- Documentation that can't get out of date
- High-quality developer tooling

Features include:

- HTTP, HTTPS (TLS), and [HTTP/2](https://http2.github.io/) built-in
  - Let's Encrypt auto-updating certificates via `--autotls`
- Declarative interface on top of [Gin](https://github.com/gin-gonic/gin)
  - Operation & model documentation
  - Request params (path, query, or header)
  - Request body
  - Responses (including errors)
  - Response headers
- JSON Errors using [RFC7807](https://tools.ietf.org/html/rfc7807) and `application/problem+json`
- Default (optional) middleware
  - [RFC8631](https://tools.ietf.org/html/rfc8631) service description & docs links
  - Automatic recovery from panics with traceback & request logging
  - Automatically handle CORS headers
  - Structured logging middleware using [Zap](https://github.com/uber-go/zap)
  - Automatic handling of `Prefer: return=minimal` from [RFC 7240](https://tools.ietf.org/html/rfc7240#section-4.2)
- Per-operation request size limits & timeouts with sane defaults
- Annotated Go types for input and output models
  - Generates JSON Schema from Go types
  - Automatic input model validation & error handling
- Dependency injection for loggers, datastores, etc
- Documentation generation using [RapiDoc](https://mrin9.github.io/RapiDoc/), [ReDoc](https://github.com/Redocly/redoc), or [SwaggerUI](https://swagger.io/tools/swagger-ui/)
- CLI built-in, configured via arguments or environment variables
  - Set via e.g. `-p 8000`, `--port=8000`, or `SERVICE_PORT=8000`
  - Connection timeouts & graceful shutdown built-in
- Generates OpenAPI JSON for access to a rich ecosystem of tools
  - Mocks with [API Sprout](https://github.com/danielgtaylor/apisprout)
  - SDKs with [OpenAPI Generator](https://github.com/OpenAPITools/openapi-generator)
  - CLIs with [OpenAPI CLI Generator](https://github.com/danielgtaylor/openapi-cli-generator)
  - And [plenty](https://openapi.tools/) [more](https://apis.guru/awesome-openapi3/category.html)

This project was inspired by [FastAPI](https://fastapi.tiangolo.com/), [Gin](https://github.com/gin-gonic/gin), and countless others. Look at the [benchmarks](https://github.com/danielgtaylor/huma/tree/master/benchmark) to see how Huma compares.

Logo & branding designed by [Kari Taylor](https://www.kari.photography/).

# Concepts & Example

REST APIs are composed of operations against resources and can include descriptions of various inputs and possible outputs. For each operation you will typically provide info like:

- HTTP method & path (e.g. `GET /items/{id}`)
- User-friendly description text
- Input path, query, or header parameters
- Input request body model, if appropriate
- Response header names and descriptions
- Response status code, content type, and output model

Huma uses standard Go types and a declarative API to capture those descriptions in order to provide a combination of a simple interface and idiomatic code leveraging Go's speed and strong typing.

Let's start by taking a quick look at a note-taking REST API. You can list notes, get a note's contents, create or update notes, and delete notes from an in-memory store. Each of the operations is registered with the router and descibes its inputs and outputs. You can view the full working example below:

```go
package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/schema"
)

// NoteSummary is used to list notes. It does not include the (potentially)
// large note content.
type NoteSummary struct {
	ID      string    `json:"id" doc:"Note ID"`
	Created time.Time `json:"created" doc:"Created date/time as ISO8601"`
}

// Note records some content text for later reference.
type Note struct {
	Created time.Time `json:"created" readOnly:"true" doc:"Created date/time as ISO8601"`
	Content string    `json:"content" doc:"Note content"`
}

// We'll use an in-memory DB (a goroutine-safe map). Don't do this in
// production code!
var memoryDB = sync.Map{}

func main() {
	// Create a new router and give our API a title and version.
	r := huma.NewRouter("Notes API", "1.0.0",
		huma.DevServer("http://localhost:8888"),
	)

	notes := r.Resource("/v1/notes")
	notes.List("Returns a list of all notes", func() []*NoteSummary {
		// Create a list of summaries from all the notes.
		summaries := make([]*NoteSummary, 0)

		memoryDB.Range(func(k, v interface{}) bool {
			summaries = append(summaries, &NoteSummary{
				ID:      k.(string),
				Created: v.(*Note).Created,
			})
			return true
		})

		return summaries
	})

	// Set up a custom schema to limit identifier values.
	idSchema := schema.Schema{Pattern: "^[a-zA-Z0-9._-]{1,32}$"}

	// Add an `id` path parameter to create a note resource.
	note := notes.With(huma.PathParam("id", "Note ID", huma.Schema(idSchema)))

	notFound := huma.ResponseError(http.StatusNotFound, "Note not found")

	note.Put("Create or update a note", func(id string, n *Note) bool {
		// Set the created time to now and then save the note in the DB.
		n.Created = time.Now()
		memoryDB.Store(id, n)

		// Empty responses don't have a body, so you can just return `true`.
		return true
	})

	note.With(notFound).Get("Get a note by its ID",
		func(id string) (*huma.ErrorModel, *Note) {
			if n, ok := memoryDB.Load(id); ok {
				// Note with that ID exists!
				return nil, n.(*Note)
			}

			return &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}, nil
		},
	)

	note.With(notFound).Delete("Delete a note by its ID",
		func(id string) (*huma.ErrorModel, bool) {
			if _, ok := memoryDB.Load(id); ok {
				// Note with that ID exists!
				memoryDB.Delete(id)
				return nil, true
			}

			return &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}, false
		},
	)

	// Run the app!
	r.Run()
}
```

Save this file as `notes/main.go`. Run it and then try to access the API with [HTTPie-Go](https://github.com/nojima/httpie-go) (or curl):

```sh
# Grab reflex to enable reloading the server on code changes:
$ go get github.com/cespare/reflex

# Grab HTTPie-go for making requests
$ go get -u github.com/nojima/httpie-go/cmd/ht

# Run the server (default host/port is 0.0.0.0:8888, see --help for options)
$ reflex -s go run notes/main.go

# Make some requests (in another tab)
$ ht put :8888/v1/notes/test1 content="Some content for note 1"
HTTP/1.1 204 No Content
Date: Sat, 07 Mar 2020 22:22:06 GMT

$ ht put :8888/v1/notes/test2 content="Some content for note 2"
HTTP/1.1 204 No Content
Date: Sat, 07 Mar 2020 22:22:06 GMT

# Parameter validation works too!
$ ht put :8888/v1/notes/@bad content="Some content for an invalid note"
HTTP/1.1 400 Bad Request
Content-Length: 97
Content-Type: application/json; charset=utf-8
Date: Sat, 07 Mar 2020 22:22:06 GMT

{
    "errors": [
        "(root): Does not match pattern '^[a-zA-Z0-9._-]{1,32}$'"
    ],
    "message": "Invalid input"
}

# List all the notes
$ ht :8888/v1/notes
HTTP/1.1 200 OK
Content-Length: 122
Content-Type: application/json; charset=utf-8
Date: Sat, 07 Mar 2020 22:22:06 GMT

[
    {
        "created": "2020-03-07T22:22:06-07:00",
        "id": "test1"
    },
    {
        "created": "2020-03-07T22:22:06-07:00",
        "id": "test2"
    }
]
```

The server works and responds as expected. There are also some neat extras built-in. If you go to http://localhost:8888/docs in a browser, you will see auto-generated interactive documentation:

<img width="878" alt="Screen Shot 2020-03-31 at 11 22 55 PM" src="https://user-images.githubusercontent.com/106826/78105715-a9dfc000-73a6-11ea-8002-371024253daf.png">

The documentation is generated from the OpenAPI 3 spec file that is available at http://localhost:8888/openapi.json. You can also access this spec without running the server:

```sh
# Save the OpenAPI 3 spec to a file.
$ go run notes/main.go openapi notes.json
```

Combine the above with [openapi-cli-generator](https://github.com/danielgtaylor/openapi-cli-generator) and [huma-build](https://github.com/danielgtaylor/huma-build) and you get the following out of the box:

- Small, efficient deployment Docker image with your service
- Auto-generated service documentation
- Auto-generated SDKs for any language
- Auto-generated cross-platform zero-dependency CLI

# Documentation

Official Go package documentation can always be found at https://pkg.go.dev/github.com/danielgtaylor/huma. Below is an introduction to the various features available in Huma.

> :whale: Hi there! I'm the happy Huma whale here to provide help. You'll see me leave helpful tips down below.

## Constructors & Options

Huma uses the [functional options](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis) paradigm when creating a router, resource, operation, parameter, etc. Functional options were chosen due to an exponential explosion of constructor functions and the complexity of the problem space. They come with several advantages:

- Friendly APIs with sane defaults
- Extensible without breaking clients or polluting the global namespace with too many constructors
- Options are immutable, reusable, and composable

They are easy to use and look like this:

```go
// Add a parameter with an example
huma.PathParam("id", "Resource identifier", huma.Example("abc123"))
```

Most text editors will auto-complete and show only the available options, which is an improvement over e.g. accepting `interface{}`.

### Extending & Composition

Functional options can be wrapped to extend the set of available options. For example:

```go
// IDParam creates a new path parameter limited to characters and a length that
// is allowed for resource identifiers.
func IDParam(name, description string) huma.DependencyOption {
	s := schema.Schema{Pattern: "^[a-zA-Z0-9_-]{3,20}"}

	return huma.PathParam(name, description, huma.Schema(s))
}
```

You can also compose multiple options into one, e.g by using `huma.ResourceOptions(..)` or one of the other related functions:

```go
// CommonOptions sets up common options for every operation.
func CommonOptions() huma.ResourceOption {
	return huma.ResourceOptions(
		huma.Tags("some-tag"),
		huma.HeaderParam("customer", "Customer name", "", huma.Internal()),
		huma.ResponseError(http.StatusInternalServerError, "Server error"),
	)
}
```

## Resources

Huma APIs are composed of resources and sub-resources attached to a router. A resource refers to a unique URI on which operations can be performed. Huma resources can have dependencies, security requirements, parameters, response headers, and responses attached to them which are all applied to every operation and sub-resource.

```go
r := huma.NewRouter("My API", "1.0.0")

// Create a resource at a given path
notes := r.Resource("/notes")

// Create another resource that includes a path parameter: /notes/{id}
note := notes.With(huma.PathParam("id", "Note ID"))

// Create a sub-resource at /notes/{id}/likes
sub := note.SubResource("/likes")
```

The `With(...)` function is very powerful and can accept dependencies, security requirements, parameters, response headers, and response description options. It returns a copy of the resource with those values applied.

> :whale: Resources should be nouns, and plural if they return more than one item. Good examples: `/notes`, `/likes`, `/users`, `/videos`, etc.

## Operations

Operations perform an action on a resource using an HTTP method verb. The following verbs are available:

- Head
- List (alias for Get)
- Get
- Post
- Put
- Patch
- Delete
- Options

Operations can take dependencies, parameters, & request bodies and produce response headers and responses. These are each discussed in more detail below.

If you don't provide a response description, then one is generated for you based on the response type with the following rules:

- Boolean: If true, returns `HTTP 204 No Content`
- String: If not empty, returns `HTTP 200 OK` with content type `text/plain`
- Slice, map, struct pointer: If not `nil`, marshal to JSON and return `HTTP 200 OK` with content type `application/json`

If you need any customization beyond the above then you must provide a response description.

```go
r := huma.NewRouter("My API", "1.0.0")

// Create a resource
notes := r.Resource("/notes")

// Create the operation with an auto-generated response.
notes.Get("Get a list of all notes", func () []*NoteSummary {
	// Implementation goes here
})

// Manually provide the response. This is equivalent to the above, but allows
// you to add additional options like allowed response headers.
notes.With(
	huma.ResponseJSON(http.StatusOK, "Success"),
).Get("Get a list of all notes", func () []*NoteSummary {
	// Implementation goes here
})
```

> :whale: Operations map an HTTP action verb to a resource. You might `POST` a new note or `GET` a user. Sometimes the mapping is less obvious and you can consider using a sub-resource. For example, rather than unliking a post, maybe you `DELETE` the `/posts/{id}/likes` resource.

## Handler Functions

The basic structure of a Huma handler function looks like this, with most arguments being optional and dependent on the declaritively described operation:

```go
func (deps..., params..., requestModel) (headers..., responseModels...)
```

Dependencies, parameters, headers, and models are covered in more detail in the following sections. For now this gives an idea of how to write handler functions based on the inputs and outputs of your operation.

For example, the most basic "Hello world" that takes no parameters and returns a greeting message might look like this:

```go
func () string { return "Hello, world" }
```

Another example: you have an `id` parameter input and return a response model to be marshalled as JSON:

```go
func (id string) *MyModel { return &MyModel{ID: id} }
```

> :whale: Confused about what a handler should look like? Just run your service and it'll print out an approximate handler function when it panics.

## Parameters

Huma supports three types of parameters:

- Required path parameters, e.g. `/things/{thingId}`
- Optional query string parameters, e.g. `/things?q=filter`
- Optional header parameters, e.g. `X-MyHeader: my-value`

Optional parameters require a default value.

Here is an example of an `id` parameter:

```go
r.Resource("/notes",
	huma.PathParam("id", "Note ID"),
	huma.ResponseError(404, "Note was not found"),
	huma.ResponseJSON(200, "Success"),
).
Get("Get a note by its ID", func(id string) (*huma.ErrorModel, *Note) {
	// Implementation goes here
})
```

You can also declare parameters with additional validation logic by using the `schema` module:

```go
s := schema.Schema{
	MinLength: 1,
	MaxLength: 32,
}

huma.PathParam("id", "Note ID", huma.Schema(s))
```

Once a parameter is declared it will get parsed, validated, and then sent to your handler function. If parsing or validation fails, the client gets a 400-level HTTP error.

> :whale: If a proxy is providing e.g. authentication or rate-limiting and exposes additional internal-only information then use the internal parameters like `huma.HeaderParam("UserID", "Parsed user from the auth system", "nobody", huma.Internal())`. Internal parameters are never included in the generated OpenAPI 3 spec or documentation.

## Request & Response Models

Request and response models are just plain Go structs with optional tags to annotate additional validation logic for their fields. From the notes API example above:

```go
// Note records some content text for later reference.
type Note struct {
	Created time.Time `readOnly:"true"`
	Content string
}
```

The `Note` struct has two fields which will get serialized to JSON. The `Created` field has a special tag `readOnly` set which means it will not get used for write operations like `PUT /notes/{id}`.

This struct provides enough information to create JSON Schema for the OpenAPI 3 spec. You can provide as much or as little information and validation as you like.

### Request Model

Request models are used by adding a new input argument that is a pointer to a struct to your handler function as the last argument. For example:

```go
r.Resource("/notes", huma.PathParam("id", "Note ID")).
	Put("Create or update a note",
		// Handler without an input body looks like:
		func(id string) bool {
			// Implementation goes here
		},

		// Handler with an input body looks like:
		func(id string, note *Note) bool {
			// Implementation goes here
		},
	)
```

The presence of the `note *Note` argument tells Huma to parse the request body and validate it against the generated JSON Schema for the `Note` struct.

### Response Model

Response models are used by adding a response to the list of possible responses along with a new function return value that is a pointer to your struct. You can specify multiple different response models.

```go
r.Resource("/notes",
	huma.ResponseError(http.NotFound, "Not found"),
	huma.ResponseJSON(http.StatusOK, "Success")).
Get("Description", func() (*huma.ErrorModel, *Note) {
	// Implementation goes here
})
```

Whichever model is not `nil` will get sent back to the client.

Empty responses, e.g. a `204 No Content` or `304 Not Modified` are also supported by setting a `ContentType` of `""` (the default zero value). Use `huma.Response` paired with a simple boolean to return a response without a body. Passing `false` acts like `nil` for models and prevents that response from being sent.

```go
r.Resource("/notes",
	huma.Response(http.StatusNoContent, "This should have no body")).
Get("description", func() bool {
	return true
})
},
```

> :whale: In some cases Huma can [auto-generate a resonse model](#operations) for you.

### Model Tags

Go struct tags are used to annotate the model with information that gets turned into [JSON Schema](https://json-schema.org/) for documentation and validation.

The standard `json` tag is supported and can be used to rename a field and mark fields as optional using `omitempty`. The following additional tags are supported on model fields:

| Tag                | Description                               | Example                  |
| ------------------ | ----------------------------------------- | ------------------------ |
| `doc`              | Describe the field                        | `doc:"Who to greet"`     |
| `format`           | Format hint for the field                 | `format:"date-time"`     |
| `enum`             | A comma-separated list of possible values | `enum:"one,two,three"`   |
| `default`          | Default value                             | `default:"123"`          |
| `minimum`          | Minimum (inclusive)                       | `minimum:"1"`            |
| `exclusiveMinimum` | Minimum (exclusive)                       | `exclusiveMinimum:"0"`   |
| `maximum`          | Maximum (inclusive)                       | `maximum:"255"`          |
| `exclusiveMaximum` | Maximum (exclusive)                       | `exclusiveMaximum:"100"` |
| `multipleOf`       | Value must be a multiple of this value    | `multipleOf:"2"`         |
| `minLength`        | Minimum string length                     | `minLength:"1"`          |
| `maxLength`        | Maximum string length                     | `maxLength:"80"`         |
| `pattern`          | Regular expression pattern                | `pattern:"[a-z]+"`       |
| `minItems`         | Minimum number of array items             | `minItems:"1"`           |
| `maxItems`         | Maximum number of array items             | `maxItems:"20"`          |
| `uniqueItems`      | Array items must be unique                | `uniqueItems:"true"`     |
| `minProperties`    | Minimum number of object properties       | `minProperties:"1"`      |
| `maxProperties`    | Maximum number of object properties       | `maxProperties:"20"`     |
| `example`          | Example value                             | `example:"123"`          |
| `nullable`         | Whether `null` can be sent                | `nullable:"false"`       |
| `readOnly`         | Sent in the response only                 | `readOnly:"true"`        |
| `writeOnly`        | Sent in the request only                  | `writeOnly:"true"`       |
| `deprecated`       | This field is deprecated                  | `deprecated:"true"`      |

### Response Headers

Response headers must be defined before they can be sent back to a client. This includes several steps:

1. Describe the response header (name & description)
2. Specify which responses may send this header
3. Add the header to the handler function return values

For example:

```go
r.Resource("/notes",
	huma.ResponseHeader("expires", "Expiration date for this content"),
	huma.ResponseText(http.StatusOK, "Success", huma.Headers("expires"))
).Get("description", func() (string, string) {
	expires := time.Now().Add(7 * 24 * time.Hour).MarshalText()
	return expires, "Hello!"
})
```

You can make use of named return values with a naked return to disambiguate complex functions:

```go
func() (expires string, message string) {
	expires = time.Now().Add(7 * 24 * time.Hour).MarshalText()
	message = "Hello!"
	return
},
```

> :whale: If you forget to declare a response header for a particular response and then try to set it when returning that response it will **not** be sent to the client and an error will be logged.

## Dependencies

Huma includes a dependency injection system that can be used to pass additional arguments to operation handler functions. You can register global dependencies (ones that do not change from request to request) or contextual dependencies (ones that change with each request).

Global dependencies are created by just setting some value, while contextual dependencies are implemented using a function that returns the value of the form `func (deps..., params...) (headers..., *YourType, error)` where the value you want injected is of `*YourType` and the function arguments can be any previously registered dependency types or one of the hard-coded types:

- `huma.ConnDependency()` the current `http.Request` connection (returns `net.Conn`)
- `huma.ContextDependency()` the current `http.Request` context (returns `context.Context`)
- `huma.GinContextDependency()` the current Gin request context (returns `*gin.Context`)
- `huma.OperationIDDependency()` the current operation ID (returns `string`)

```go
// Register a new database connection dependency
db := huma.SimpleDependency(db.NewConnection())

// Register a new request logger dependency. This is contextual because we
// will print out the requester's IP address with each log message.
type MyLogger struct {
	Info: func(msg string),
}

logger := huma.Dependency(
	huma.GinContextDependency(),
	func(c *gin.Context) (*MyLogger, error) {
		return &MyLogger{
			Info: func(msg string) {
				fmt.Printf("%s [ip:%s]\n", msg, c.Request.RemoteAddr)
			},
		}, nil
	},
)

// Use them in any handler by adding them to both `Depends` and the list of
// handler function arguments.
r.Resource("/foo").With(
	db, logger
).Get("doc", func(db *db.Connection, log *MyLogger) string {
	log.Info("test")
	item := db.Fetch("query")
	return item.ID
})
```

When creating a new dependency you can use `huma.DependencyOptions` to group multiple options:

```go
logger := huma.Dependency(huma.DependencyOptions(
	huma.GinContextDependency(),
	huma.OperationIDDependency(),
), func (c *gin.Context, operationID string) (*MyLogger, error) {
	return ...
})
```

> :whale: Note that global dependencies cannot be functions. You can wrap them in a struct as a workaround if needed.

## Custom Gin

You can create a Huma router instance with a custom Gin instance. This lets you set up custom middleware, CORS configurations, logging, etc.

```go
// The following two are equivalent:
// Default settings:
r := huma.NewRouter("My API", "1.0.0")

// And manual settings:
g := gin.New()
g.Use(huma.Recovery())
g.Use(huma.LogMiddleware())
g.Use(cors.Default())
g.Use(huma.PreferMinimalMiddleware())
g.Use(huma.ServiceLinkMiddleware())
g.NoRoute(huma.Handler404())
r := huma.NewRouter("My API", "1.0.0", huma.WithGin(g))
```


## Custom CORS Handler

If you would like CORS preflight requests to allow specific headers, do the following:

```go
// CORS: Allow non-standard headers "Authorization" and "X-My-Header" in preflight requests
cfg := cors.DefaultConfig()
cfg.AllowAllOrigins = true
cfg.AllowHeaders = append(cfg.AllowHeaders, "Authorization", "X-My-Header")

// And manual settings:
r := huma.NewRouter("My API", "1.0.0", huma.CORSHandler(cors.New(cfg)))
```

## Custom HTTP Server

You can have full control over the `http.Server` that is created.

```go
// Set low timeouts to kick off slow clients.
s := &http.Server{
	ReadTimeout: 5 * time.Seconds,
	WriteTimeout: 5 * time.Seconds,
	Handler: r
}

r := huma.NewRouter("My API", "1.0.0", huma.HTTPServer(s))

r.Run()
```

### Timeouts, Deadlines, & Cancellation

By default, only a `ReadHeaderTimeout` of _10 seconds_ and an `IdleTimeout` of _15 seconds_ are set at the server level. This allows large request and response bodies to be sent without fear of timing out in the default config, as well as the use of WebSockets.

Set timeouts and deadlines on the request context and pass that along to libraries to prevent long-running handlers. For example:

```go
r.Resource("/timeout",
	huma.ContextDependency(),
).Get("timeout example", func(ctx context.Context) string {
	// Add a timeout to the context. No request should take longer than 2 seconds
	newCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Create a new request that will take 5 seconds to complete.
	req, _ := http.NewRequestWithContext(
		newCtx, http.MethodGet, "https://httpstat.us/418?sleep=5000", nil)

	// Make the request. This will return with an error because the context
	// deadline of 2 seconds is shorter than the request duration of 5 seconds.
	_, err := http.DefaultClient.Do(req)
	if err != nil {
		return err.Error()
	}

	return "success"
})
```

### Request Body Timeouts

By default any handler which takes in a request body parameter will have a read timeout of 15 seconds set on it. If set to nonzero for a handler which does **not** take a body, then the timeout will be set on the underlying connection before calling your handler.

When triggered, the server sends a 408 Request Timeout as JSON with a message containing the time waited.

```go
type Input struct {
	ID string
}

r := huma.NewRouter("My API", "1.0.0")

// Limit to 5 seconds
r.Resource("/foo", huma.BodyReadTimeout(5 * time.Second)).Post(
	"Create item", func(input *Input) string {
		return "Hello, " + input.ID
	})
```

You can also access the underlying TCP connection and set deadlines manually:

```go
r.Resource("/foo", huma.ConnDependency()).Get(func (conn net.Conn) string {
	// Set a new deadline on connection reads.
	conn.SetReadDeadline(time.Now().Add(600 * time.Second))

	// Read all the data from the request.
	data, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		panic(err)
	}

	// Do something with the data...
	return fmt.Sprintf("Read %d bytes", len(data))
})
```

> :whale: Set to `-1` in order to disable the timeout.

### Request Body Size Limits

By default each operation has a 1 MiB reqeuest body size limit.

When triggered, the server sends a 413 Request Entity Too Large as JSON with a message containing the maximum body size for this operation.

```go
r := huma.NewRouter("My API", "1.0.0")

// Limit set to 10 MiB
r.Resource("/foo", MaxBodyBytes(10 * 1024 * 1024)).Get(...)
```

> :whale: Set to `-1` in order to disable the check, allowing for unlimited request body size for e.g. large streaming file uploads.

## Logging

Huma provides a Zap-based contextual structured logger built-in. You can access it via the `huma.LogDependency()` which returns a `*zap.SugaredLogger`. It requires the use of the `huma.LogMiddleware(...)`, which is included by default. If you provide a custom Gin instance you should include the middleware.

```go
r.Resource("/test",
	huma.LogDependency(),
).Get("Logger test", func(log *zap.SugaredLogger) string {
	log.Info("I'm using the logger!")
	return "Hello, world"
})
```

## Customizing Logging

Logging is completely customizable.

```go
// Create your own logger, or use the Huma built-in:
l, err := huma.NewLogger()
if err != nil {
	panic(err)
}

// Update the logger somehow with your custom logic.
l = l.With(zap.String("some", "value"))

// Set up the router with the default settings and your custom logger.
g := gin.New()
g.Use(gin.Recovery())
g.Use(cors.Default())
g.Use(huma.LogMiddleware(l))

r := huma.NewRouter("My API", "1.0.0", huma.WithGin(g))
```

## Lazy-loading at Server Startup

You can register functions to run before the server starts, allowing for things like lazy-loading dependencies.

```go
var db *mongo.Client

r := huma.NewRouter("My API", "1.0.0",
	huma.PreStart(func() {
		// Connect to the datastore
		var err error
		db, err = mongo.Connect(context.Background(),
			options.Client().ApplyURI("..."))
	})
)
```

> :whale: This is especially useful for external dependencies and if any custom CLI commands are set up. For example, you may not want to require a database to run `my-service openapi my-api.json`.

## Changing the Documentation Renderer

You can choose between [RapiDoc](https://mrin9.github.io/RapiDoc/), [ReDoc](https://github.com/Redocly/redoc), or [SwaggerUI](https://swagger.io/tools/swagger-ui/) to auto-generate documentation. Simply set the documentation handler on the router:

```go
r := huma.NewRouter("My API", "1.0.0", huma.DocsHandler(huma.ReDocHandler("My API")))
```

> :whale: Pass a custom handler function to have even more control for branding or browser authentication.

## Custom OpenAPI Fields

You can set custom OpenAPI fields via the `Extra` field in the `OpenAPI` and `Operation` structs.

```go
r := huma.NewRouter("My API", "1.0.0", huma.Extra(map[string]interface{}{
	"x-something": "some-value",
}))
```

Use the OpenAPI hook for additional customization. It gives you a `*gab.Container` instance that represents the root of the OpenAPI document.

```go
func modify(openapi *gabs.Container) {
	openapi.Set("value", "paths", "/test", "get", "x-foo")
}

r := huma.NewRouter("My API", "1.0.0", huma.OpenAPIHook(modify))
```

> :whale: See the [OpenAPI 3 spec](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.2.md) for everything that can be set.

## Custom CLI Arguments

You can add additional CLI arguments, e.g. for additional logging tags. Use the `AddGlobalFlag` function along with the `viper` module to get the parsed value.

```go
r := huma.NewRouter("My API", "1.0.0",
	// Add a long arg (--env), short (-e), description & default
	huma.GlobalFlag("env", "e", "Environment", "local")
)

r.Resource("/current_env").Text(http.StatusOK, "Success").Get(
	"Return the current environment",
	func() string {
		// The flag is automatically bound to viper settings.
		return viper.GetString("env")
	},
)
```

Then run the service:

```sh
$ go run yourservice.go --env=prod
```

> :whale: Combine custom arguments with [customized logger setup](#customizing-logging) and you can easily log your cloud provider, environment, region, pod, etc with every message.

## Custom CLI Commands

You can access the root `cobra.Command` via `r.Root()` and add new custom commands via `r.Root().AddCommand(...)`. The `openapi` sub-command is one such example in the default setup.

> :whale: You can also overwite `r.Root().Run` to completely customize how you run the server.

## Middleware

You can make use of any Gin-compatible middleware via the `GinMiddleware()` router option.

```go
r := huma.NewRouter("My API", "1.0.0", huma.GinMiddleware(gin.Logger()))
```

## HTTP/2 Setup

TODO

## Testing

The Go standard library provides useful testing utilities and Huma routers implement the [`http.Handler`](https://golang.org/pkg/net/http/#Handler) interface they expect. Huma also provides a `humatest` package with utilities for creating test routers capable of e.g. capturing logs.

You can see an example in the [`examples/test`](https://github.com/danielgtaylor/huma/tree/master/examples/test) directory:

```go
package main

import "github.com/danielgtaylor/huma"

func routes(r *huma.Router) {
	// Register a single test route that returns a text/plain response.
	r.Resource("/test").Get("Test route", func() string {
		return "Hello, test!"
	})
}

func main() {
	// Create the router.
	r := huma.NewRouter("Test", "1.0.0")

	// Register routes.
	routes(r)

	// Run the service.
	r.Run()
}
```

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/humatest"
	"github.com/stretchr/testify/assert"
)

func TestHandler(t *testing.T) {
	// Set up the test router and register the routes.
	r := humatest.NewRouter(t)
	routes(r)

	// Make a request against the service.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	// Assert the response is as expected.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Hello, test!", w.Body.String())
}
```

# How it Works

Huma's philosophy is to make it harder to make mistakes by providing tools that reduce duplication and encourage practices which make it hard to forget to update some code.

An example of this is how handler functions **must** declare all headers that they return and which responses may send those headers. You simply **cannot** return from the function without considering the values of each of those headers. If you set one that isn't appropriate for the response you return, Huma will let you know.

How does it work? Huma asks that you give up one compile-time static type check for handler function signatures and instead let it be a runtime startup check. It's simple enough that even the most basic unit test will invoke the runtime check, giving you most of the security you would from static typing.

Using a small amount of reflection, Huma can verify the function signatures, inject dependencies and parameters, and handle responses and headers as well as making sure that they all match the declared operation.

By strictly enforcing this runtime interface you get several advantages. No more out of date API description. No more out of date documenatation. No more out of date SDKs or CLIs. Your entire ecosystem of tooling is driven off of one simple backend implementation. Stuff just works.

More docs coming soon.

> :whale: Thanks for reading!
