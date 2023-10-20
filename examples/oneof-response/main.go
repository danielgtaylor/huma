// This example show how to respond to API requests with different versions of
// the response body. Try the following requests:
//
//	# Get the latest version of the response
//	restish get :8888/greeting/oneof
//
//	# Get the old version of the response
//	restish get :8888/greeting/oneof -H X-Old-Version:true
package main

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" default:"8888"`
}

// GreetingInput represents the greeting operation request.
type GreetingInput struct {
	Name       string `path:"name" doc:"Name to greet"`
	OldVersion bool   `header:"X-Old-Version" doc:"Use the old version of the API response"`
}

// GreetingOutput represents the greeting operation response.
type GreetingOutput struct {
	Body any
}

// GreetingBody is the body of the response for the latest version of the API.
type GreetingBody struct {
	Message string `json:"message"`
}

// GreetingBodyOld is the body of the response for the old version of the API.
type GreetingBodyOld struct {
	Msg string `json:"msg"`
}

func main() {
	// Create a CLI app which takes a port option.
	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// Create a schema for the output body.
		registry := api.OpenAPI().Components.Schemas
		schema := &huma.Schema{
			OneOf: []*huma.Schema{
				registry.Schema(reflect.TypeOf(GreetingBody{}), true, ""),
				registry.Schema(reflect.TypeOf(GreetingBodyOld{}), true, ""),
			},
		}

		// Register GET /greeting/{name}
		huma.Register(api, huma.Operation{
			OperationID: "get-greeting",
			Summary:     "Get a greeting",
			Method:      http.MethodGet,
			Path:        "/greeting/{name}",
			Responses: map[string]*huma.Response{
				"200": {
					Content: map[string]*huma.MediaType{
						"application/json": {
							Schema: schema,
						},
					},
				},
			},
		}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
			resp := &GreetingOutput{}
			msg := fmt.Sprintf("Hello, %s!", input.Name)

			// Set the output body based on what the user has requested.
			if input.OldVersion {
				resp.Body = GreetingBodyOld{Msg: msg}
			} else {
				resp.Body = GreetingBody{Message: msg}
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
