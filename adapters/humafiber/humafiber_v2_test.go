package humafiber

import (
	"bytes"
	"compress/gzip"
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

// TestFiberV2EachHeaderAndCookie ensures EachHeader yields one callback per
// header value (not one per byte) so header-based helpers like huma.ReadCookie
// continue to work under the Fiber v2 adapter. Regression test for #1055.
func TestFiberV2EachHeaderAndCookie(t *testing.T) {
	r := fiberV2.New()
	api := NewV2(r, huma.DefaultConfig("Test API", "1.0.0"))

	var authValue string
	var authCallbacks int
	var cookieValue string
	var cookieErr error
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx.EachHeader(func(name, value string) {
			if name == "Authorization" {
				authCallbacks++
				authValue = value
			}
		})
		cookie, err := huma.ReadCookie(ctx, "session")
		cookieErr = err
		if cookie != nil {
			cookieValue = cookie.Value
		}
		next(ctx)
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-root",
		Method:      http.MethodGet,
		Path:        "/",
	}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		return &struct{}{}, nil
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Set("Authorization", "Bearer abc")
	req.Header.Set("Cookie", "session=xyz")
	_, err := r.Test(req)
	require.NoError(t, err)

	require.Equal(t, 1, authCallbacks, "EachHeader should fire once per header value, not once per byte")
	require.Equal(t, "Bearer abc", authValue)
	require.NoError(t, cookieErr)
	require.Equal(t, "xyz", cookieValue)
}

// TestFiberV2BodyReaderDecompresses ensures BodyReader returns the decompressed
// request body (Body) rather than the raw compressed bytes (BodyRaw), so
// handlers receive usable payloads when Content-Encoding is set. Regression
// test for #1055.
func TestFiberV2BodyReaderDecompresses(t *testing.T) {
	r := fiberV2.New()
	api := NewV2(r, huma.DefaultConfig("Test API", "1.0.0"))

	type Body struct {
		Name string `json:"name"`
	}
	huma.Register(api, huma.Operation{
		OperationID: "echo",
		Method:      http.MethodPost,
		Path:        "/echo",
	}, func(ctx context.Context, in *struct{ Body Body }) (*struct{ Body Body }, error) {
		return &struct{ Body Body }{Body: in.Body}, nil
	})

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write([]byte(`{"name":"gzipped"}`))
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	req, _ := http.NewRequest(http.MethodPost, "http://example.com/echo", &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	resp, err := r.Test(req)
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "compressed body should decompress and parse: %s", body)
	require.Contains(t, string(body), `"name":"gzipped"`)
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
