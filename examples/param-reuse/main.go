// This example shows how to reuse a parameter in the path and body.
//
//	# Example call
//	restish post :8888/reuse/leon
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" default:"8888"`
}

// ReusableParam is a reusable parameter that can go in the path or the body
// of a request or response. The same validation applies to both places.
type ReusableParam struct {
	User string `path:"user" json:"user" maxLength:"10"`
}

type MyResponse struct {
	Body struct {
		// Example use as a body field.
		ReusableParam
	}
}

func main() {
	// Create a CLI app which takes a port option.
	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		huma.Register(api, huma.Operation{
			OperationID: "reuse",
			Method:      http.MethodPost,
			Path:        "/reuse/{user}",
			Summary:     "Param re-use example",
		}, func(ctx context.Context, input *struct {
			// Example use as a path parameter.
			ReusableParam
		}) (*MyResponse, error) {
			resp := &MyResponse{}
			resp.Body.User = input.User
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
