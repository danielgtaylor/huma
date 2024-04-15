---
description: Level up your API with a generated Go SDK and client that uses it.
---

# Client SDKs

[Several tools](https://openapi.tools/#sdk) can be used to create SDKs from an OpenAPI spec. Let's use the [`oapi-codegen`](https://github.com/deepmap/oapi-codegen) Go code generator to create a Go SDK, and then build a client using that SDK.

## Add an OpenAPI Command

First, let's create a command to grab the OpenAPI spec so the service doesn't need to be running and you can generate the SDK as needed (e.g. as part of the API service release process).

```go title="main.go" linenums="1" hl_lines="69 75 86-96"
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

// ReviewInput represents the review operation request.
type ReviewInput struct {
	Body struct {
		Author  string `json:"author" maxLength:"10" doc:"Author of the review"`
		Rating  int    `json:"rating" minimum:"1" maximum:"5" doc:"Rating from 1 to 5"`
		Message string `json:"message,omitempty" maxLength:"100" doc:"Review message"`
	}
}

func addRoutes(api huma.API) {
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

	// Register POST /reviews
	huma.Register(api, huma.Operation{
		OperationID:   "post-review",
		Method:        http.MethodPost,
		Path:          "/reviews",
		Summary:       "Post a review",
		Tags:          []string{"Reviews"},
		DefaultStatus: http.StatusCreated,
	}, func(ctx context.Context, i *ReviewInput) (*struct{}, error) {
		// TODO: save review in data store.
		return nil, nil
	})
}

func main() {
	var api huma.API

	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api = humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		addRoutes(api)

		// Tell the CLI how to start your server.
		hooks.OnStart(func() {
			fmt.Printf("Starting server on port %d...\n", options.Port)
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), router)
		})
	})

	// Add a command to print the OpenAPI spec.
	cli.Root().AddCommand(&cobra.Command{
		Use:   "openapi",
		Short: "Print the OpenAPI spec",
		Run: func(cmd *cobra.Command, args []string) {
			// Use downgrade to return OpenAPI 3.0.3 YAML since oapi-codegen doesn't
			// support OpenAPI 3.1 fully yet. Use `.YAML()` instead for 3.1.
			b, _ := api.OpenAPI().DowngradeYAML()
			fmt.Println(string(b))
		},
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
```

## Generate the SDK

First, grab the OpenAPI spec. Then install and use the generator to create the SDK.

{{ asciinema("../../terminal/build-sdk.cast", rows="14") }}

## Build the Client

Next, we can use the SDK by writing a small client script.

```go title="client/client.go"
package main

import (
	"context"
	"fmt"

	"github.com/my-user/my-api/sdk"
)

func main() {
	ctx := context.Background()

	// Initialize an SDK client.
	client, _ := sdk.NewClientWithResponses("http://localhost:8888")

	// Make the greeting request.
	greeting, err := client.GetGreetingWithResponse(ctx, "world")
	if err != nil {
		panic(err)
	}

	if greeting.StatusCode() > 200 {
		panic(greeting.ApplicationproblemJSONDefault)
	}

	// Everything was successful, so print the message.
	fmt.Println(greeting.JSON200.Message)
}
```

## Run the Client

Now you're ready to run the client:

{{ asciinema("../../terminal/sdk-client.cast", rows="8") }}

## Review

Congratulations! You just learned:

-   How to install an SDK generator
-   How to generate a Go SDK for your API
-   How to build a client using the SDK to call the API

## Dive Deeper

Want to learn more about OpenAPI tooling like SDK generators and how to use them? Check these out next:

-   SDK Generators
    -   [`oapi-codegen`](https://github.com/deepmap/oapi-codegen)
    -   [OpenAPI Generator](https://openapi-generator.tech/)
-   OpenAPI Tool Directories
    -   [openapi.tools](https://openapi.tools/)
    -   [tools.openapis.org](https://tools.openapis.org/)
