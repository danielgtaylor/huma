---
description: Level up your API with runtime configuration from the environment or command-line options.
---

# Service Configuration

Huma includes a basic command-line and environment variable option parser that can be used to provide runtime configuration to your service. This lets you pass in things like the port the service runs on, which environment to tag logs with, secrets and endpoints for dependencies like databases, etc.

## Port Option

[Your first API](your-first-api.md#operation) can be updated to take an optional network port parameter like this:

```go title="main.go" linenums="1" hl_lines="16-19 29-30 51-59"
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/go-chi/chi/v5"

	_ "github.com/danielgtaylor/huma/v2/formats/cbor"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

// GreetingOutput represents the greeting operation response.
type GreetingOutput struct {
	Body struct {
		Message string `json:"message" example:"Hello, world!" doc:"Greeting message"`
	}
}

func main() {
	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// Register GET /greeting/{name}
		huma.Register(api, huma.Operation{
			OperationID: "get-greeting",
			Method:      http.MethodGet,
			Path:        "/greeting/{name}",
			Summary:     "Get a greeting",
			Description: "Get a greeting for a person by name.",
			Tags:        []string{"Greetings"},
		}, func(ctx context.Context, input *struct{
			Name string `path:"name" maxLength:"30" example:"world" doc:"Name to greet"`
		}) (*GreetingOutput, error) {
			resp := &GreetingOutput{}
			resp.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)
			return resp, nil
		})

		// Tell the CLI how to start your server.
		hooks.OnStart(func() {
			fmt.Printf("Starting server on port %d...\n", options.Port)
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), router)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
```

Just like requests and responses, the CLI options are defined using a custom struct with a field for each option. Once defined, all that's left is to wrap your service startup code and then run the CLI from your `main` function.

## Passing Options

Options can be passed explicitly as command-line arguments to the service or they can be provided by environment variables prefixed with `SERVICE_`. For example, to run the service on port 8000:

```bash
# Example passing command-line args
$ go run main.go --port=8000

# Short arguments are also supported
$ go run main.go -p 8000

# Example passing by environment variables
$ SERVICE_PORT=8000 go run main.go
```

!!! warning "Precedence"

    If both environment variable and command-line arguments are present, then command-line arguments take priority.

## Review

Congratulations! You just learned:

-   How to add a CLI option to your service
-   How to pass options via command-line arguments or environment variables

## Dive Deeper

Want to learn more about how the CLI works and how to use it? Check these out next:

-   [CLI intro](../features/cli.md)
-   [CLI option types & tags](../features/cli.md#custom-options)
-   [CLI custom commands](../features/cli.md#custom-commands)
