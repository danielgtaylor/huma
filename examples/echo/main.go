package main

import (
	"net/http"

	"github.com/danielgtaylor/huma"
)

// EchoResponse message which echoes a value.
type EchoResponse struct {
	Value string `json:"value" description:"The echoed back word"`
}

func main() {
	r := huma.NewRouter(&huma.OpenAPI{
		Title:   "My API",
		Version: "1.0.0",
	})

	r.Register(&huma.Operation{
		Method:      http.MethodPut,
		Path:        "/echo/{word}",
		Description: "Echo back an input word.",
		Params: []*huma.Param{
			huma.PathParam("word", "The word to echo back"),
			huma.QueryParam("greet", "Return a greeting", false),
		},
		Responses: []*huma.Response{
			huma.ResponseJSON(http.StatusOK, "Successful echo response"),
			huma.ResponseError(http.StatusBadRequest, "Invalid input"),
		},
		Handler: func(word string, greet bool) (*EchoResponse, *huma.ErrorModel) {
			if word == "test" {
				return nil, &huma.ErrorModel{Message: "Value not allowed: test"}
			}

			v := word
			if greet {
				v = "Hello, " + word
			}

			return &EchoResponse{Value: v}, nil
		},
	})

	r.Run("0.0.0.0:8888")
}
