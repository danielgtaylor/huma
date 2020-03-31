![Huma Rest API Framework](https://user-images.githubusercontent.com/106826/76379557-9e502a80-630d-11ea-9c7d-f6426076a47c.png)

[![CI](https://github.com/danielgtaylor/huma/workflows/CI/badge.svg?branch=master)](https://github.com/danielgtaylor/huma/actions?query=workflow%3ACI+branch%3Amaster++) [![codecov](https://codecov.io/gh/danielgtaylor/huma/branch/master/graph/badge.svg)](https://codecov.io/gh/danielgtaylor/huma) [![Docs](https://godoc.org/github.com/danielgtaylor/huma?status.svg)](https://pkg.go.dev/github.com/danielgtaylor/huma?tab=doc) [![Go Report Card](https://goreportcard.com/badge/github.com/danielgtaylor/huma)](https://goreportcard.com/report/github.com/danielgtaylor/huma)

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
- Default (optional) middleware
  - Automatic recovery from panics
  - Automatically handle CORS headers
  - Structured logging middleware using [Zap](https://github.com/uber-go/zap)
- Annotated Go types for input and output models
  - Generates JSON Schema from Go types
  - Automatic input model validation & error handling
- Dependency injection for loggers, datastores, etc
- Documentation generation using [RapiDoc](https://mrin9.github.io/RapiDoc/), [ReDoc](https://github.com/Redocly/redoc), or [SwaggerUI](https://swagger.io/tools/swagger-ui/)
- CLI built-in, configured via arguments or environment variables
  - Set via e.g. `-p 8000`, `--port=8000`, or `SERVICE_PORT=8000`
- Generates OpenAPI JSON for access to a rich ecosystem of tools
  - Mocks with [API Sprout](https://github.com/danielgtaylor/apisprout)
  - SDKs with [OpenAPI Generator](https://github.com/OpenAPITools/openapi-generator)
  - CLIs with [OpenAPI CLI Generator](https://github.com/danielgtaylor/openapi-cli-generator)
  - And [plenty](https://openapi.tools/) [more](https://apis.guru/awesome-openapi3/category.html)

This project was inspired by [FastAPI](https://fastapi.tiangolo.com/), [Gin](https://github.com/gin-gonic/gin), and countless others. Look at the [benchmarks](https://github.com/danielgtaylor/huma/tree/master/benchmark) to see how Huma compares.

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
)

// NoteSummary is used to list notes. It does not include the (potentially)
// large note content.
type NoteSummary struct {
	ID      string
	Created time.Time
}

// Note records some content text for later reference.
type Note struct {
	Created time.Time `readOnly:"true"`
	Content string
}

// We'll use an in-memory DB (a map) and protect it with a lock. Don't do
// this in production code!
var memoryDB = make(map[string]*Note, 0)
var dbLock = sync.Mutex{}

func main() {
	// Create a new router and give our API a title and version.
	r := huma.NewRouter(&huma.OpenAPI{
		Title:   "Notes API",
		Version: "1.0.0",
	})

	r.Register(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/notes",
		Description: "Returns a list of all notes",
		Responses: []*huma.Response{
			huma.ResponseJSON(http.StatusOK, "Successful hello response"),
		},
		Handler: func() []*NoteSummary {
			dbLock.Lock()
			defer dbLock.Unlock()

			// Create a list of summaries from all the notes.
			summaries := make([]*NoteSummary, 0, len(memoryDB))

			for k, v := range memoryDB {
				summaries = append(summaries, &NoteSummary{
					ID:      k,
					Created: v.Created,
				})
			}

			return summaries
		},
	})

	// idParam defines the note's ID as part of the URL path.
	idParam := huma.PathParam("id", "Note ID", &huma.Schema{
		Pattern: "^[a-zA-Z0-9._-]{1,32}$",
	})

	r.Register(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/notes/{id}",
		Description: "Get a single note by its ID",
		Params:      []*huma.Param{idParam},
		Responses: []*huma.Response{
			huma.ResponseJSON(200, "Success"),
			huma.ResponseError(404, "Note was not found"),
		},
		Handler: func(id string) (*Note, *huma.ErrorModel) {
			dbLock.Lock()
			defer dbLock.Unlock()

			if note, ok := memoryDB[id]; ok {
				// Note with that ID exists!
				return note, nil
			}

			return nil, &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}
		},
	})

	r.Register(&huma.Operation{
		Method:      http.MethodPut,
		Path:        "/notes/{id}",
		Description: "Creates or updates a note",
		Params:      []*huma.Param{idParam},
		Responses: []*huma.Response{
			huma.ResponseEmpty(204, "Successfully created or updated the note"),
		},
		Handler: func(id string, note *Note) bool {
			dbLock.Lock()
			defer dbLock.Unlock()

			// Set the created time to now and then save the note in the DB.
			note.Created = time.Now()
			memoryDB[id] = note

			// Empty responses don't have a body, so you can just return `true`.
			return true
		},
	})

	r.Register(&huma.Operation{
		Method:      http.MethodDelete,
		Path:        "/notes/{id}",
		Description: "Deletes a note",
		Params:      []*huma.Param{idParam},
		Responses: []*huma.Response{
			huma.ResponseEmpty(204, "Successfuly deleted note"),
			huma.ResponseError(404, "Note was not found"),
		},
		Handler: func(id string) (bool, *huma.ErrorModel) {
			dbLock.Lock()
			defer dbLock.Unlock()

			if _, ok := memoryDB[id]; ok {
				// Note with that ID exists!
				delete(memoryDB, id)
				return true, nil
			}

			return false, &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}
		},
	})

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
$ ht put :8888/notes/test1 content="Some content for note 1"
HTTP/1.1 204 No Content
Date: Sat, 07 Mar 2020 22:22:06 GMT

$ ht put :8888/notes/test2 content="Some content for note 2"
HTTP/1.1 204 No Content
Date: Sat, 07 Mar 2020 22:22:06 GMT

# Parameter validation works too!
$ ht put :8888/notes/@bad content="Some content for an invalid note"
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
$ ht :8888/notes
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

<img width="986" alt="Screen Shot 2020-03-28 at 11 25 53 PM" src="https://user-images.githubusercontent.com/106826/77842804-8d941700-714b-11ea-8bf6-8e6a63af4a2c.png">

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

## Handler Functions

The basic structure of a Huma handler function looks like this, with most arguments being optional and dependent on the declaritively described operation:

```go
func (deps..., params..., requestModel) (headers..., responseModels...)
```

Dependencies, parameterss, headers, and models are covered in more detail in the following sections. For now this gives an idea of how to write handler functions based on the inputs and outputs of your operation.

For example, the most basic "Hello world" that takes no parameters and returns a greeting message might look like this:

```go
func () string { return "Hello, world" }
```

Another example: you have an `id` parameter input and return a response model to be marshalled as JSON:

```go
func (id string) *MyModel { return &MyModel{ID: id} }
```

## Parameters

Huma supports three types of parameters:

- Required path parameters, e.g. `/things/{thingId}`
- Optional query string parameters, e.g. `/things?q=filter`
- Optional header parameters, e.g. `X-MyHeader: my-value`

Optional parameters require a default value.

Here is an example of an `id` parameter:

```go
r.Register(&huma.Operation{
	Method:      http.MethodGet,
	Path:        "/notes/{id}",
	Description: "Get a single note by its ID",
	Params:      []*huma.Param{
		huma.PathParam("id", "Note ID"),
	},
	Responses: []*huma.Response{
		huma.ResponseJSON(200, "Success"),
		huma.ResponseError(404, "Note was not found"),
	},
	Handler: func(id string) (*Note, *huma.ErrorModel) {
		// Implementation goes here
	},
})
```

You can also declare parameters with additional validation logic:

```go
huma.PathParam("id", "Note ID", &huma.Schema{
	MinLength: 1,
	MaxLength: 32,
})
```

Once a parameter is declared it will get parsed, validated, and then sent to your handler function. If parsing or validation fails, the client gets a 400-level HTTP error.

If a proxy is providing e.g. authentication or rate-limiting and exposes additional internal-only information then use the internal parameters like `huma.HeaderParamInternal("UserID", "Parsed user from the auth system", "nobody")`. Internal parameters are never included in the generated OpenAPI 3 spec.

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
r.Register(&huma.Operation{
	Method:      http.MethodPut,
	Path:        "/notes/{id}",
	Description: "Create or update a note",
	Params:      []*huma.Param{
		huma.PathParam("id", "Note ID"),
	},
	Responses: []*huma.Response{
		huma.ResponseEmpty(204, "Success"),
	},

	// Handler without an input body looks like:
	Handler: func(id string) bool {
		// Implementation goes here
	},

	// Handler with an input body looks like:
	Handler: func(id string, note *Note) bool {
		// Implementation goes here
	},
})
```

The presence of the `note *Note` argument tells Huma to parse the request body and validate it against the generated JSON Schema for the `Note` struct.

### Response Model

Response models are used by adding a response to the list of possible responses along with a new function return value that is a pointer to your struct. You can specify multiple different response models.

```go
Responses: []*huma.Response{
	huma.ResponseJSON(200, "Success"),
	huma.ResponseError(404, "Not found"),
},
Handler: func() (*Note, *huma.ErrorModel) {
	// Implementation goes here
},
```

Whichever model is not `nil` will get sent back to the client.

Empty responses, e.g. a `204 No Content` or `304 Not Modified` are also supported. Use `huma.ResponseEmpty` paired with a simple boolean to return a response without a body. Passing `false` acts like `nil` for models and prevents that response from being sent.

```go
Responses: []*huma.Response{
	huma.ResponseEmpty(204, "This should have no body"),
},
Handler: func() bool {
	// Implementation goes here
	return true
},
```

### Model Tags

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
| `example`          | Example value                             | `example:"123"`              |
| `nullable`         | Whether `null` can be sent                | `nullable:"false"`           |
| `readOnly`         | Sent in the response only                 | `readOnly:"true"`            |
| `writeOnly`        | Sent in the request only                  | `writeOnly:"true"`           |
| `deprecated`       | This field is deprecated                  | `deprecated:"true"`          |

### Response Headers

Response headers must be defined before they can be sent back to a client. This includes several steps:

1. Describe the response header (name & description)
2. Specify which responses may send this header
3. Add the header to the handler function return values

For example:

```go
ResponseHeaders: []*huma.ResponseHeader{
	huma.Header("expires", "Expiration date for this content"),
},
Responses: []*huma.Response{
	huma.ResponseText(200, "Success"),
},
Handler: func() (string, string) {
	expires := time.Now().Add(7 * 24 * time.Hour).MarshalText()
	return expires, "Hello!"
},
```

You can make use of named return values with a naked return to disambiguate complex functions:

```go
Handler: func() (expires string, message string) {
	expires = time.Now().Add(7 * 24 * time.Hour).MarshalText()
	message = "Hello!"
	return
},
```

## Dependencies

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

## Custom Gin

You can create a Huma router instance with a custom Gin instance. This lets you set up custom middleware, CORS configurations, logging, etc.

```go
// This is a shortcut:
r := huma.NewRouter(&huma.OpenAPI{
	Title:   "Notes API",
	Version: "1.0.0",
})

// For this:
g := gin.New()
g.Use(huma.Recovery())
g.Use(huma.LogMiddleware(nil, nil))
g.Use(cors.Default())
r := huma.NewRouterWithGin(g, &huma.OpenAPI{
	Title:   "Notes API",
	Version: "1.0.0",
})
```

## Logging

Huma provides a Zap-based contextual structured logger built-in. You can access it via the `huma.LogDependency()` which returns a `*zap.SugaredLogger`. It requires the use of the `huma.LogMiddleware(...)`, which is included by default.

```go
r.Register(&huma.Operation{
	Method:      http.MethodGet,
	Path:        "/test",
	Description: "Test example",
	Dependencies: []*huma.Dependency{
		huma.LogDependency(),
	},
	Responses: []*huma.Response{
		huma.ResponseText(http.StatusOK, "Successful"),
	},
	Handler: func(log *zap.SugaredLogger) string {
		log.Info("I'm using the logger!")
		return "Hello, world"
	},
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
g.Use(huma.LogMiddleware(l, nil))
r := huma.NewRouterWithGin(g, &huma.OpenAPI{
	Title:   "Notes API",
	Version: "1.0.0",
})
```

## Lazy-loading at Server Startup

You can register functions to run before the server starts, allowing for things like lazy-loading dependencies. This is especially useful for external dependencies and if any custom CLI commands are set up.

```go
var db *mongo.Client

r := huma.NewRouter(&huma.OpenAPI{
	Title:   "Example API",
	Version: "1.0.0",
})

r.PreStart(func() {
	// Connect to the datastore
	var err error
	db, err = mongo.Connect(context.Background(),
		options.Client().ApplyURI("..."))
})
```

## Changing the Documentation Renderer

You can choose between [RapiDoc](https://mrin9.github.io/RapiDoc/), [ReDoc](https://github.com/Redocly/redoc), or [SwaggerUI](https://swagger.io/tools/swagger-ui/) to auto-generate documentation. Simply set the documentation handler on the router:

```go
r := huma.NewRouter(&huma.OpenAPI{
	Title:   "Example API",
	Version: "1.0.0",
})

r.SetDocsHandler(huma.ReDocHandler)
```

## Custom OpenAPI Fields

You can set custom OpenAPI fields via the `Extra` field in the `OpenAPI` and `Operation` structs.

```go
r := huma.NewRouter(&huma.OpenAPI{
	Title:   "Example API",
	Version: "1.0.0",
	Extra: map[string]interface{}{
		"x-something": "some-value",
	},
})
```

## Custom CLI Arguments

You can add additional CLI arguments, e.g. for additional logging tags. Use the `AddGlobalFlag` function along with the `viper` module to get the parsed value.

```go
r := huma.NewRouter(&huma.OpenAPI{
	Title:   "Example API",
	Version: "1.0.0",
})

r.AddGlobalFlag("env", "e", "Environment", "local")

r.Register(&huma.Operation{
	Method:      http.MethodGet,
	Path:        "/current_env",
	Description: "Return the current environment",
	Responses: []*huma.Response{
		huma.ResponseText(http.StatusOK, "Success"),
	},
	Handler: func() string {
		return viper.GetString("env")
	},
})
```

Then run the service:

```sh
$ go run yourservice.go --env=prod
```

## Custom CLI Commands

You can access the root `cobra.Command` via `r.Root()` and add new custom commands via `r.Root().AddCommand(...)`. The `openapi` sub-command is one such example in the default setup.

## Middleware

You can make use of any Gin-compatible middleware via the `Use()` router function.

```go
r := huma.NewRouter(&huma.OpenAPI{
	Title:   "Example API",
	Version: "1.0.0",
})

r.Use(gin.Logger())
```

## HTTP/2 Setup

TODO

# How it Works

Huma's philosophy is to make it harder to make mistakes by providing tools that reduce duplication and encourage practices which make it hard to forget to update some code.

An example of this is how handler functions **must** declare all headers that they return and which responses may send those headers. You simply **cannot** return from the function without considering the values of each of those headers. If you set one that isn't appropriate for the response you return, Huma will let you know.

How does it work? Huma asks that you give up one compile-time static type check for handler function signatures and instead let it be a runtime startup check. It's simple enough that even the most basic unit test will invoke the runtime check, giving you most of the security you would from static typing.

Using a small amount of reflection, Huma can verify the function signatures, inject dependencies and parameters, and handle responses and headers as well as making sure that they all match the declared operation.

By strictly enforcing this runtime interface you get several advantages. No more out of date API description. No more out of date documenatation. No more out of date SDKs or CLIs. Your entire ecosystem of tooling is driven off of one simple backend implementation. Stuff just works.

More docs coming soon.
