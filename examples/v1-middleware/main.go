// An example showing how to use Huma v1 middleware in a Huma v2 project.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/danielgtaylor/huma/middleware"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/go-chi/chi"

	_ "github.com/danielgtaylor/huma/v2/formats/cbor"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" default:"8888"`
}

// GreetingInput represents the greeting operation request.
type GreetingInput struct {
	Name string `path:"name" doc:"Name to greet"`
}

// GreetingOutput represents the greeting operation response.
type GreetingOutput struct {
	Body struct {
		Message string `json:"message" doc:"Greeting message" example:"Hello, world!"`
	}
}

func main() {
	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, opts *Options) {
		// Create a Chi v4.x.x router instance.
		router := chi.NewMux()

		// Attach the Huma v1 middleware to the router.
		router.Use(middleware.DefaultChain)

		// Create the API
		config := huma.DefaultConfig("My API", "1.0.0")
		api := humachi.NewV4(router, config)

		// Register GET /greeting/{name}
		huma.Register(api, huma.Operation{
			OperationID: "get-greeting",
			Summary:     "Get a greeting",
			Method:      http.MethodGet,
			Path:        "/greeting/{name}",
		}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
			resp := &GreetingOutput{}
			resp.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)
			return resp, nil
		})

		srv := &http.Server{
			Addr:    fmt.Sprintf("%s:%d", "localhost", opts.Port),
			Handler: router,
		}

		// Tell the CLI how to start your router.
		hooks.OnStart(func() {
			// Start the server
			log.Printf("Server is running with: host:%v port:%v\n", "localhost", opts.Port)

			err := srv.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				log.Fatalf("listen: %s\n", err)
			}
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
