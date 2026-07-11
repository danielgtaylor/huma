package humafiber

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	fiberV2 "github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

// TestWithValueShouldPropagateContextV2 ensures values set via huma.WithValue
// propagate into the underlying Fiber v2 context so native middleware can read
// them. See https://github.com/danielgtaylor/huma/issues/859
func TestWithValueShouldPropagateContextV2(t *testing.T) {
	r := fiberV2.New()
	app := NewV2(r, huma.DefaultConfig("Test", "1.0.0"))

	type (
		testInput  struct{}
		testOutput struct{}
		ctxKey     struct{}
	)

	ctxValue := "sentinelValue"

	huma.Register(app, huma.Operation{
		OperationID: "test",
		Path:        "/test",
		Method:      http.MethodGet,
		Middlewares: huma.Middlewares{
			func(ctx huma.Context, next func(huma.Context)) {
				ctx = huma.WithValue(ctx, ctxKey{}, ctxValue)
				next(ctx)
			},
			middlewareV2(func(next fiberV2.Handler) fiberV2.Handler {
				return func(c *fiberV2.Ctx) error {
					val, _ := c.UserContext().Value(ctxKey{}).(string)
					_, err := c.WriteString(val)
					return err
				}
			}),
		},
	}, func(ctx context.Context, input *testInput) (*testOutput, error) {
		return &testOutput{}, nil
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	resp, err := r.Test(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, ctxValue, string(out))
}

func middlewareV2(mw func(next fiberV2.Handler) fiberV2.Handler) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		fCtx := UnwrapV2(ctx)
		h := mw(func(c *fiberV2.Ctx) error {
			ctx := &fiberV2Wrapper{op: ctx.Operation(), orig: c, ctx: c.UserContext()}
			next(ctx)
			return nil
		})
		if err := h(fCtx); err != nil {
			panic(err)
		}
	}
}

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
