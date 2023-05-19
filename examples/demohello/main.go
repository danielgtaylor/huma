package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/gin-gonic/gin"
)

type Options struct {
	Port int `doc:"Port to listen on." short:"p" default:"3000"`
}

type GreetingRequest struct {
	Name string `path:"name" example:"World" maxLength:"10"`
}

func (r *GreetingRequest) Resolve(ctx huma.Context) []error {
	if strings.Contains(r.Name, "err") {
		return []error{&huma.ErrorDetail{
			Location: "path.name",
			Message:  "I do not like this name!",
			Value:    r.Name,
		}}
	}
	return nil
}

type GreetingResponse struct {
	Body struct {
		Message string `json:"message" example:"Hello, World!"`
	}
}

func main() {
	cli := huma.NewCLI(func(cli huma.CLI, options *Options) {
		// router := gin.Default()
		router := gin.New()
		// router := fiber.New()
		// router.Use(compress.New())

		// api := humafiber.New(router, huma.DefaultConfig("My API", "1.0.0"))
		api := humagin.New(router, huma.DefaultConfig("My API", "1.0.0"))

		huma.Register(api, huma.Operation{
			OperationID: "hello",
			Method:      http.MethodGet,
			Path:        "/hello/{name}",
			Tags:        []string{"Greetings"},
			Errors:      []int{http.StatusBadRequest},
		}, func(ctx context.Context, input *GreetingRequest) (*GreetingResponse, error) {
			resp := &GreetingResponse{}
			resp.Body.Message = "Hello, " + input.Name + "!"
			return resp, nil
		})

		cli.OnStart(func() {
			// Connect dependencies, if needed.
			// router.Listen(fmt.Sprintf(":%d", options.Port))
			router.Run(fmt.Sprintf(":%d", options.Port))
		})
	})

	cli.Run()
}
