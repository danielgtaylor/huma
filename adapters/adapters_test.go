// Package adapters_test runs basic verification tests on all adapters.
package adapters_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humabunrouter"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/adapters/humahttprouter"
	"github.com/danielgtaylor/huma/v2/adapters/humamux"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/mux"
	"github.com/julienschmidt/httprouter"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/uptrace/bunrouter"
)

type key struct{}

// Test the various input types (path, query, header, body).
type TestInput struct {
	Group   string `path:"group"`
	Verbose bool   `query:"verbose"`
	Auth    string `header:"Authorization"`
	Body    struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
}

// Test outputs (headers, body).
type TestOutput struct {
	MyHeader string `header:"MyHeader"`
	Body     struct {
		Message string `json:"message"`
	}
}

func testHandler(ctx context.Context, input *TestInput) (*TestOutput, error) {
	resp := &TestOutput{}
	resp.MyHeader = "my-value"
	resp.Body.Message = fmt.Sprintf("Hello, %s <%s>! (%s, %v, %s)", input.Body.Name, input.Body.Email, input.Group, input.Verbose, input.Auth)
	return resp, nil
}

func testAdapter(t *testing.T, api huma.API) {
	t.Helper()

	methods := []string{http.MethodPut, http.MethodPost}

	// Test two operations with the same path but different methods
	for _, method := range methods {
		huma.Register(api, huma.Operation{
			OperationID: method + "-test",
			Method:      method,
			Path:        "/{group}",
		}, testHandler)
	}

	// Make test calls
	for _, method := range methods {
		testAPI := humatest.Wrap(t, api)
		resp := testAPI.Do(method, "/foo",
			"Host: localhost",
			"Authorization: Bearer abc123",
			strings.NewReader(`{"name": "Daniel", "email": "daniel@example.com"}`),
		)

		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "my-value", resp.Header().Get("MyHeader"))
		assert.JSONEq(t, `{
		"$schema": "http://localhost/schemas/TestOutputBody.json",
		"message": "Hello, Daniel <daniel@example.com>! (foo, false, Bearer abc123)"
	}`, resp.Body.String())
	}
}

func TestAdapters(t *testing.T) {
	config := func() huma.Config {
		return huma.DefaultConfig("Test", "1.0.0")
	}

	wrap := func(h huma.API, isFiber bool, unwrapper func(ctx huma.Context)) huma.API {
		h.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
			assert.Nil(t, ctx.TLS())
			v := ctx.Version()

			if !isFiber {
				assert.Equal(t, "HTTP/1.1", v.Proto)
				assert.Equal(t, 1, v.ProtoMajor)
				assert.Equal(t, 1, v.ProtoMinor)
			} else {
				assert.Equal(t, "http", v.Proto)
			}

			// Make sure huma.WithValue works correctly
			ctx = huma.WithContext(ctx, context.WithValue(ctx.Context(), key{}, "value"))

			next(ctx)
		}, func(ctx huma.Context, next func(huma.Context)) {
			// Make sure the Unwrap func does not panic even when the context is wrapped by WithContext
			assert.NotPanics(t, func() { unwrapper(ctx) })
			next(ctx)
		})
		return h
	}

	for _, adapter := range []struct {
		name string
		new  func() huma.API
	}{
		{"chi", func() huma.API {
			return wrap(humachi.New(chi.NewMux(), config()), false, func(ctx huma.Context) { humachi.Unwrap(ctx) })
		}},
		{"echo", func() huma.API {
			return wrap(humaecho.New(echo.New(), config()), false, func(ctx huma.Context) { humaecho.Unwrap(ctx) })
		}},
		{"fiber", func() huma.API {
			return wrap(humafiber.New(fiber.New(), config()), true, func(ctx huma.Context) { humafiber.Unwrap(ctx) })
		}},
		{"go", func() huma.API {
			return wrap(humago.New(http.NewServeMux(), config()), false, func(ctx huma.Context) { humago.Unwrap(ctx) })
		}},
		{"gin", func() huma.API {
			return wrap(humagin.New(gin.New(), config()), false, func(ctx huma.Context) { humagin.Unwrap(ctx) })
		}},
		{"httprouter", func() huma.API {
			return wrap(humahttprouter.New(httprouter.New(), config()), false, func(ctx huma.Context) { humahttprouter.Unwrap(ctx) })
		}},
		{"mux", func() huma.API {
			return wrap(humamux.New(mux.NewRouter(), config()), false, func(ctx huma.Context) { humamux.Unwrap(ctx) })
		}},
		{"bunrouter", func() huma.API {
			return wrap(humabunrouter.New(bunrouter.New(), config()), false, func(ctx huma.Context) { humabunrouter.Unwrap(ctx) })
		}},
		{"bunroutercompat", func() huma.API {
			return wrap(humabunrouter.NewCompat(bunrouter.New().Compat(), config()), false, func(ctx huma.Context) {
				// FIXME: humabunrouter.Unwrap(ctx) doesn't work with compat mode
			})
		}},
	} {
		t.Run(adapter.name, func(t *testing.T) {
			testAdapter(t, adapter.new())
		})
	}
}
