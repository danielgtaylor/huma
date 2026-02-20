package humafiber

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	fiberV2 "github.com/gofiber/fiber/v2"
)

func BenchmarkHumaFiberV2(b *testing.B) {
	type GreetingInput struct {
		ID string `path:"id"`
	}

	type GreetingOutput struct {
		Body struct {
			Greeting string `json:"greeting"`
		}
	}

	r := fiberV2.New()
	api := NewV2(r, huma.DefaultConfig("Test API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "greet",
		Method:      http.MethodGet,
		Path:        "/foo/{id}",
	}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
		resp := &GreetingOutput{}
		resp.Body.Greeting = "Hello, " + input.ID
		return resp, nil
	})

	b.ResetTimer()
	b.ReportAllocs()
	req, _ := http.NewRequest(http.MethodGet, "/foo/123", nil)
	for i := 0; i < b.N; i++ {
		r.Test(req)
	}
}

func BenchmarkNotHumaV2(b *testing.B) {
	type GreetingOutput struct {
		Greeting string `json:"greeting"`
	}

	r := fiberV2.New()

	r.Get("/foo/:id", func(c *fiberV2.Ctx) error {
		return c.JSON(&GreetingOutput{"Hello, " + c.Params("id")})
	})

	b.ResetTimer()
	b.ReportAllocs()
	req, _ := http.NewRequest(http.MethodGet, "/foo/123", nil)
	for i := 0; i < b.N; i++ {
		r.Test(req)
	}
}
