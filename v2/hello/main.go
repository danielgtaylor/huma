package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humafiber"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

type Options struct {
	Port int `doc:"Port to listen on." short:"p" default:"3000"`
}

type GreetingRequest struct {
	Name string `path:"name" example:"World" maxLength:"10"`
}

type GreetingResponse struct {
	Body struct {
		Message string `json:"message" example:"Hello, World!"`
	}
}

func main() {
	cli := huma.NewCLI(func(cli huma.CLI, options *Options) {
		router := fiber.New()
		router.Use(compress.New())
		router.Use(requestid.New())

		api := humafiber.New(router, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{
					Title:   "My API",
					Version: "1.0.0",
				},
			},
		})

		huma.Register(api, huma.Operation{
			OperationID: "hello",
			Method:      http.MethodGet,
			Path:        "/hello/{name}",
			Tags:        []string{"Greetings"},
			Errors:      []int{http.StatusBadRequest},
		}, func(ctx context.Context, input *GreetingRequest) (*GreetingResponse, error) {
			if input.Name == "err" {
				return nil, huma.Error400BadRequest("Bad name", &huma.ErrorDetail{
					Location: "path.name",
					Message:  "I do not like this name!",
					Value:    input.Name,
				})
			}

			resp := &GreetingResponse{}
			resp.Body.Message = "Hello, " + input.Name + "!"
			return resp, nil
		})

		cli.OnStart(func() {
			// Connect dependencies, e.g. grpc
			router.Listen(fmt.Sprintf(":%d", options.Port))
		})
	})

	cli.Run()
}
