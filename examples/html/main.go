package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/humacli"
)

type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

// HTMLOutput represents a response containing HTML.
type HTMLOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

func main() {
	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		// Create a new router & API.
		mux := http.NewServeMux()
		api := humago.New(mux, huma.DefaultConfig("HTML Example API", "1.0.0"))

		// Register a simple HTML endpoint.
		huma.Register(api, huma.Operation{
			OperationID: "get-html",
			Summary:     "Get a simple HTML page",
			Method:      http.MethodGet,
			Path:        "/",
			Responses: map[string]*huma.Response{
				"200": {
					Description: "HTML response",
					Content: map[string]*huma.MediaType{
						"text/html": {},
					},
				},
			},
		}, func(ctx context.Context, input *struct{}) (*HTMLOutput, error) {
			resp := &HTMLOutput{
				ContentType: "text/html",
				Body:        []byte("<html><body><h1>Hello from Huma!</h1><p>This is a rendered HTML response.</p></body></html>"),
			}
			return resp, nil
		})

		// Tell the CLI how to start your server.
		hooks.OnStart(func() {
			fmt.Printf("Starting server on port %d...\n", options.Port)
			fmt.Printf("Open http://localhost:%d/ in your browser\n", options.Port)
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), mux)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
