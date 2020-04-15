package main

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Item tracks the price of a good.
type Item struct {
	ID      int     `json:"id"`
	Name    string  `json:"name"`
	Price   float32 `json:"price"`
	IsOffer bool    `json:"is_offer,omitempty"`
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.Use(huma.Recovery())
	g.Use(cors.Default())
	g.Use(huma.PreferMinimalMiddleware())

	r := huma.NewRouter("Benchmark", "1.0.0", huma.Gin(g))

	d := huma.Dependency(
		huma.HeaderParam("authorization", "Auth header", ""),
		func(auth string) (string, error) {
			return strings.Split(auth, " ")[0], nil
		},
	)

	r.Resource("/items", d,
		huma.PathParam("id", "The item's unique ID"),
		huma.ResponseHeader("x-authinfo", "..."),
		huma.ResponseJSON(http.StatusOK, "Successful hello response", huma.Headers("x-authinfo")),
	).Get("Huma benchmark test", func(authInfo string, id int) (string, *Item) {
		return authInfo, &Item{
			ID:      id,
			Name:    "Hello",
			Price:   1.24,
			IsOffer: false,
		}
	})

	r.Run()
}
