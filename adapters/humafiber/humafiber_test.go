package humafiber

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/iotest"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/gofiber/fiber/v3"
)

// TestSSE exercises the Fiber-specific streaming path (the StreamBody hook /
// fasthttp SetBodyStreamWriter) so Server-Sent Events flush to the client
// instead of failing with "unable to flush". Regression test for #888.
func TestSSE(t *testing.T) {
	app := fiber.New()
	api := New(app, huma.DefaultConfig("Test", "1.0.0"))

	type Message struct {
		Text string `json:"text"`
	}

	sse.Register(api, huma.Operation{
		OperationID: "sse",
		Method:      http.MethodGet,
		Path:        "/sse",
	}, map[string]any{
		"message": Message{},
	}, func(ctx context.Context, input *struct{}, send sse.Sender) {
		_ = send.Data(Message{Text: "hello"})
		_ = send.Comment("heartbeat")
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "http://example.com/sse", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := "data: {\"text\":\"hello\"}\n\n: heartbeat\n\n"
	if string(body) != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

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

// TestServeHTTP checks ServeHTTP, which autopatch and humatest use: it waits
// for a handler slower than the 1s default timeout of Fiber's Test, and a
// request Test can't send gets a plain 500 instead of a panic.
func TestServeHTTP(t *testing.T) {
	api := humatest.Wrap(t, New(fiber.New(), huma.DefaultConfig("Test", "1.0.0")))
	huma.Get(api, "/slow", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		time.Sleep(1100 * time.Millisecond)
		return nil, nil
	})

	if resp := api.Get("/slow"); resp.Code != http.StatusNoContent {
		t.Fatalf("slow handler: status = %d, want 204", resp.Code)
	}

	w := httptest.NewRecorder()
	api.Adapter().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/slow", iotest.ErrReader(errors.New("read failed"))))
	if w.Code != http.StatusInternalServerError || w.Body.String() != "Internal Server Error\n" {
		t.Fatalf("unreadable body: got %d %q, want 500 %q", w.Code, w.Body.String(), "Internal Server Error\n")
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
