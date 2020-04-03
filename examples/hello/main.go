package main

import (
	"github.com/danielgtaylor/huma"
)

func main() {
	// Create a new router and give our API a title and version.
	r := huma.NewRouter(&huma.OpenAPI{
		Title:   "Hello API",
		Version: "1.0.0",
	})

	// Create the "hello" operation via `GET /hello`.
	r.Resource("/hello").Get("Basic hello world", func() string {
		return "Hello, world\n"
	})

	// Start the server on http://localhost:8888/
	r.Run()
}
