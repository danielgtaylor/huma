package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi"
)

// Options for the CLI.
type Options struct {
	Port int `doc:"Port to listen on." short:"p" default:"3000"`
}

// GreetingRequest is the input to the greeting endpoint. It takes a name to
// use in the greeting response.
type GreetingRequest struct {
	Name string `path:"name" example:"World" maxLength:"10"`
}

// Resolve allows custom validation of the input. In this example, if the
// name is `err` then we return an error instead of a greeting.
func (r *GreetingRequest) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	if strings.Contains(r.Name, "err") {
		return []error{&huma.ErrorDetail{
			Location: "path.name",
			Message:  "I do not like this name!",
			Value:    r.Name,
		}}
	}
	return nil
}

var _ huma.ResolverWithPath = (*GreetingRequest)(nil)

// GreetingResponse is the output from the greeting endpoint. It contains a
// message to greet the user with.
type GreetingResponse struct {
	Body struct {
		Message string `json:"message" example:"Hello, World!"`
	}
}

func main() {
	// Create the CLI, passing a function to be called with your custom options
	// after they have been parsed.
	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		router := chi.NewMux()

		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// Register the greeting operation.
		huma.Register(api, huma.Operation{
			OperationID: "get-greeting",
			Summary:     "Get a greeting",
			Method:      http.MethodGet,
			Path:        "/greeting/{name}",
		}, func(ctx context.Context, input *GreetingRequest) (*GreetingResponse, error) {
			resp := &GreetingResponse{}
			resp.Body.Message = "Hello, " + input.Name + "!"
			return resp, nil
		})

		// Tell the CLI how to start your router.
		hooks.OnStart(func() {
			// Connect dependencies, if needed.
			// ...

			// Start the server
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), router)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
