package main

import "github.com/istreamlabs/huma"

func routes(r *huma.Router) {
	// Register a single test route that returns a text/plain response.
	r.Resource("/test").Get("Test route", func() string {
		return "Hello, test!"
	})
}

func main() {
	// Create the router.
	r := huma.NewRouter("Test", "1.0.0")

	// Register routes.
	routes(r)

	// Run the service.
	r.Run()
}
