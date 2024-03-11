package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/go-chi/chi/v5"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

// GreetingInput represents the greeting operation request.
type GreetingInput struct {
	Name string      `path:"name" maxLength:"30" example:"world" doc:"Name to greet"`
	Foo  http.Cookie `cookie:"foo"`
}

// GreetingOutput represents the greeting operation response.
type GreetingOutput struct {
	SetCookie []*http.Cookie `header:"Set-Cookie"`
	Body      struct {
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
			Summary:     "Get a greeting",
			Method:      http.MethodGet,
			Path:        "/greeting/{name}",
		}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
			fmt.Println("cookie foo is", input.Foo.Value)

			resp := &GreetingOutput{}
			resp.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)

			// Set some cookies:
			resp.SetCookie = []*http.Cookie{
				{
					Domain:  "example.com",
					Name:    "foo",
					Value:   "bar",
					Expires: time.Now().Add(24 * time.Hour),
				},
				{
					Name:  "baz",
					Value: "123",
				},
			}

			return resp, nil
		})

		// Tell the CLI how to start your router.
		hooks.OnStart(func() {
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), router)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
