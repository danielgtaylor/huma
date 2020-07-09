package main

import (
	"net/http"

	"github.com/istreamlabs/huma"
)

// EchoResponse message which echoes a value.
type EchoResponse struct {
	Value string `json:"value" description:"The echoed back word"`
}

func main() {
	r := huma.NewRouter("My API", "1.0.0")

	r.Resource("/echo",
		huma.PathParam("word", "The word to echo back"),
		huma.QueryParam("greet", "Return a greeting", false),
		huma.ResponseError(http.StatusBadRequest, "Invalid input"),
		huma.ResponseJSON(http.StatusOK, "Successful echo response"),
	).Put("Echo back an input word",
		func(word string, greet bool) (*huma.ErrorModel, *EchoResponse) {
			if word == "test" {
				return &huma.ErrorModel{Detail: "Value not allowed: test"}, nil
			}

			v := word
			if greet {
				v = "Hello, " + word
			}

			return nil, &EchoResponse{Value: v}
		},
	)

	r.Run()
}
