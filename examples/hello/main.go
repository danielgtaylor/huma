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
	r.Register(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/hello",
		Description: "Basic hello world",
		// Every response definition includes the HTTP status code to return, the
		// content type to use, and a description for documentation.
		Responses: []*huma.Response{
			huma.ResponseText(http.StatusOK, "Successful hello response"),
		},
		// The Handler is the operation's implementation. In this example, we
		// are just going to return the string "hello", but you could fetch
		// data from your datastore or do other things here.
		Handler: func() string {
			return "Hello, world"
		},
	})

	// Start the server on http://localhost:8888/
	r.Run("0.0.0.0:8888")
}
