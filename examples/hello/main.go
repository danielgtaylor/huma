package main

import (
	"net/http"

	"github.com/danielgtaylor/huma"
)

func main() {
	// Create a new router and give our API a title and version.
	r := huma.NewRouter(&huma.OpenAPI{
		Title:   "My API",
		Version: "1.0.0",
	})

	// Create the "hello" operation via `GET /hello`.
	r.Resource("/hello").
		Text(http.StatusOK, "Success").
		Get("Basic hello world", func() string {
			return "Hello, world\n"
		})

	// Start the server on http://localhost:8888/
	r.Run()
}
