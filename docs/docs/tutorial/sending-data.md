---
description: Level up your API by accepting review data from the user, complete with built-in validation.
---

# Sending Data

Let's level up our API and accept some data from the user.

```title="API Design"
Request:
POST /reviews
{
	"author": "Daniel",
	"rating": 5,
	"message": "Some custom review message"
}

Response: 201 Created
```

## Put Review

Add a new operation to our API that allows users to submit reviews of our product.

```go title="main.go" linenums="1" hl_lines="28-35 60-71"
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

## Calling the API

Make a request to the API:

{{ asciinema("../../terminal/post-review.cast", rows="8") }}

You can also try sending invalid data, and see how you get exhaustive errors back from your API. Omit the `author` body field and use a rating outside the range of valid values:

{{ asciinema("../../terminal/post-review-error.cast", rows="28") }}

## Review

Congratulations! You just learned:

-   How to add a new operation to your API
-   How to set a default status code for an operation
-   How to accept data from the user
-   How built-in validation returns errors to the user
-   How to use the `omitempty` struct tag to make fields optional

## Dive Deeper

Want to learn more about how sending data works? Check these out next:

-   [Request Inputs](../features/request-inputs.md)
-   [Validation](../features/request-validation.md)
-   [Resolvers](../features/request-resolvers.md)
-   [Limits](../features/request-limits.md)
