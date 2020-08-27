package main

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/cli"
	"github.com/danielgtaylor/huma/middleware"
	"github.com/danielgtaylor/huma/responses"
)

// Item tracks the price of a good.
type Item struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Price   float32 `json:"price"`
	IsOffer bool    `json:"is_offer,omitempty"`
}

type Input struct {
	AuthInfo string
	ID       int `path:"id"`
}

func (i *Input) Resolve(ctx huma.Context, r *http.Request) {
	i.AuthInfo = strings.Split(r.Header.Get("Authorization"), " ")[0]
}

func main() {
	app := cli.New(huma.New("Benchmark", "1.0.0"))
	app.Middleware(middleware.Recovery, middleware.ContentEncoding)

	app.Resource("/items", "id").Get("get", "Huma benchmark test",
		responses.OK().Headers("x-authinfo").Model(Item{}),
	).Run(func(ctx huma.Context, input Input) {
		ctx.Header().Set("x-authinfo", input.AuthInfo)
		ctx.WriteModel(http.StatusOK, &Item{
			ID:      input.ID,
			Name:    "Hello",
			Price:   1.24,
			IsOffer: false,
		})
	})

	app.Run()
}
