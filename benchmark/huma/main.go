package main

import (
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma"
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
	g.Use(gin.Recovery())

	r := huma.NewRouterWithGin(g, &huma.OpenAPI{
		Title:   "My API",
		Version: "1.0.0",
	})

	d := &huma.Dependency{
		Params: []*huma.Param{
			huma.HeaderParam("authorization", "Auth header", ""),
		},
		Value: func(auth string) (string, error) {
			return strings.Split(auth, " ")[0], nil
		},
	}

	r.Resource("/items").Get(&huma.Operation{
		Description:  "FastAPI benchmark test",
		Dependencies: []*huma.Dependency{d},
		Params: []*huma.Param{
			huma.PathParam("id", "The item's unique ID"),
		},
		ResponseHeaders: []*huma.ResponseHeader{
			huma.Header("x-authinfo", "..."),
		},
		Responses: []*huma.Response{
			huma.ResponseJSON(http.StatusOK, "Successful hello response", "x-authinfo"),
		},
		Handler: func(authInfo string, id int) (string, *Item) {
			return authInfo, &Item{
				ID:      id,
				Name:    "Hello",
				Price:   1.24,
				IsOffer: false,
			}
		},
	})

	r.Run()
}
