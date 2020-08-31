package main

import (
	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/cli"
	"github.com/danielgtaylor/huma/responses"
)

func routes(r *huma.Router) {
	// Register a single test route that returns a text/plain response.
	r.Resource("/test").Get("test", "Test route",
		responses.OK().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		ctx.Write([]byte("Hello, test!"))
	})
}

func main() {
	// Create the router.
	app := cli.NewRouter("Test", "1.0.0")

	// Register routes.
	routes(app.Router)

	// Run the service.
	app.Run()
}
