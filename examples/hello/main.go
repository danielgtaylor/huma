package main

import (
	"net/http"

	"github.com/danielgtaylor/huma"
)

func main() {
	r := huma.NewRouter(&huma.OpenAPI{
		Title:   "My API",
		Version: "1.0.0",
	})

	r.Register(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/hello",
		Description: "Basic hello world",
		Responses: []*huma.Response{
			huma.ResponseText(http.StatusOK, "Successful hello response"),
		},
		Handler: func() string {
			return "hello"
		},
	})

	r.Run("0.0.0.0:8888")
}
