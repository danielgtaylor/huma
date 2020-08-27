package main

import (
	"net/http"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/cli"
	"github.com/danielgtaylor/huma/responses"
)

func main() {
	app := cli.NewRouter("Minimal Example", "1.0.0")

	app.Resource("/").Get("get-root", "Get a short text message",
		responses.String(http.StatusOK),
	).Run(func(ctx huma.Context) {
		ctx.Write([]byte("Hello, world"))
		// panic("foo")
	})

	app.Run()
}
