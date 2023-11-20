package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

type MyError struct {
	status  int
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

func (e *MyError) Error() string {
	return e.Message
}

func (e *MyError) GetStatus() int {
	return e.status
}

func main() {
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		details := make([]string, len(errs))
		for i, err := range errs {
			details[i] = err.Error()
		}
		return &MyError{
			status:  status,
			Message: message,
			Details: details,
		}
	}

	router := chi.NewMux()
	api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "get-error",
		Method:      http.MethodGet,
		Path:        "/error",
	}, func(ctx context.Context, i *struct{}) (*struct{}, error) {
		return nil, huma.Error404NotFound("not found", fmt.Errorf("some-other-error"))
	})

	http.ListenAndServe(":8888", router)
}
