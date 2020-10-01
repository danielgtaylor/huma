package main

import (
	"github.com/istreamlabs/huma"
	"github.com/istreamlabs/huma/cli"
	"github.com/istreamlabs/huma/responses"
)

func main() {
	app := cli.NewRouter("Minimal Example", "1.0.0")

	app.Resource("/").Get("get-root", "Get a short text message",
		responses.OK().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		ctx.Header().Set("Content-Type", "text/plain")
		ctx.Write([]byte("Hello, world"))
	})

	app.Run()
}
