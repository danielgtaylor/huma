---
description: Use byte slice responses to return images or other binary data.
---

# Image Response

## Image Response { .hidden }

Images or other encoded or binary responses can be returned by simply using a `[]byte` body and providing some additional information at operation registration time, such as the response body content type.

## Example

```go title="code.go" linenums="1" hl_lines="19-23 32-51"
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/go-chi/chi/v5"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

// ImageOutput represents the image operation response.
type ImageOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

func main() {
	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// Register GET /image
		huma.Register(api, huma.Operation{
			OperationID: "get-image",
			Summary:     "Get an image",
			Method:      http.MethodGet,
			Path:        "/image",
			Responses: map[string]*huma.Response{
				"200": {
					Description: "Image response",
					Content: map[string]*huma.MediaType{
						"image/jpeg": {},
					},
				},
			},
		}, func(ctx context.Context, input *struct{}) (*ImageOutput, error) {
			resp := &ImageOutput{}
			resp.ContentType = "image/png"
			resp.Body = []byte{ /* ... image bytes here ... */ }
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
