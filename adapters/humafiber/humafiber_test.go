package humafiber

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
)

// TestNewContext covers the exported NewContext constructor. Unlike the other
// adapters, the Fiber adapter's Handle wraps the context to expose fasthttp
// user values, so it can't call NewContext itself; exercise it directly here.
func TestNewContext(t *testing.T) {
	app := fiber.New()
	op := &huma.Operation{OperationID: "test"}

	var got huma.Context
	app.Get("/", func(c fiber.Ctx) error {
		got = NewContext(op, c)
		return nil
	})

	if _, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil)); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("NewContext returned nil")
	}
	if got.Operation() != op {
		t.Fatalf("Operation() = %v, want %v", got.Operation(), op)
	}
}

func BenchmarkHumaFiber(b *testing.B) {
	type GreetingInput struct {
		ID string `path:"id"`
	}

	type GreetingOutput struct {
		Body struct {
			Greeting string `json:"greeting"`
		}
	}

	r := fiber.New()
	api := New(r, huma.DefaultConfig("Test API", "1.0.0"))

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

func BenchmarkNotHuma(b *testing.B) {
	type GreetingOutput struct {
		Greeting string `json:"greeting"`
	}

	r := fiber.New()

	r.Get("/foo/:id", func(c fiber.Ctx) error {
		return c.JSON(&GreetingOutput{"Hello, " + c.Params("id")})
	})

	b.ResetTimer()
	b.ReportAllocs()
	req, _ := http.NewRequest(http.MethodGet, "/foo/123", nil)
	for i := 0; i < b.N; i++ {
		r.Test(req)
	}
}
