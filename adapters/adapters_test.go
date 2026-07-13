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
	"github.com/danielgtaylor/huma/v2/adapters/humaflow"
	"github.com/danielgtaylor/huma/v2/adapters/humaflow/flow"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/adapters/humahttprouter"
	"github.com/danielgtaylor/huma/v2/adapters/humamux"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	fiberV2 "github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/gorilla/mux"
	"github.com/julienschmidt/httprouter"
	echoV4 "github.com/labstack/echo/v4"
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

	// nativeContext returns the underlying request context for an adapter, or nil
	// if the adapter can't be unwrapped. It is used to assert that a value set via
	// huma.WithValue propagates into the native request context that framework
	// middleware read. See https://github.com/danielgtaylor/huma/issues/859
	wrap := func(h huma.API, isFiber bool, nativeContext func(ctx huma.Context) context.Context) huma.API {
		h.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
			assert.Nil(t, ctx.TLS())
			v := ctx.Version()

			if !isFiber {
				assert.Equal(t, "HTTP/1.1", v.Proto)
				assert.Equal(t, 1, v.ProtoMajor)
				assert.Equal(t, 1, v.ProtoMinor)
			} else {
				// Fiber adapters (both v2 and v3) don't populate ProtoMajor/ProtoMinor
				assert.Contains(t, []string{"http", "HTTP/1.1"}, v.Proto)
			}

			// Set a request-scoped value that downstream native middleware must be
			// able to read from the underlying request context.
			ctx = huma.WithValue(ctx, key{}, "value")

			next(ctx)
		}, func(ctx huma.Context, next func(huma.Context)) {
			// Unwrapping must not panic even after the context was replaced via
			// WithValue, and the value must have propagated into the native
			// request context.
			var native context.Context
			assert.NotPanics(t, func() { native = nativeContext(ctx) })
			if native != nil {
				assert.Equal(t, "value", native.Value(key{}))
			}
			next(ctx)
		})
		return h
	}

	for _, adapter := range []struct {
		name string
		new  func() huma.API
	}{
		{"chi", func() huma.API {
			return wrap(humachi.New(chi.NewMux(), config()), false, func(ctx huma.Context) context.Context {
				r, _ := humachi.Unwrap(ctx)
				return r.Context()
			})
		}},
		{"echo", func() huma.API {
			return wrap(humaecho.New(echo.New(), config()), false, func(ctx huma.Context) context.Context {
				return humaecho.Unwrap(ctx).Request().Context()
			})
		}},
		{"echo-v4", func() huma.API {
			return wrap(humaecho.NewV4(echoV4.New(), config()), false, func(ctx huma.Context) context.Context {
				return humaecho.UnwrapV4(ctx).Request().Context()
			})
		}},
		{"fiber", func() huma.API {
			return wrap(humafiber.New(fiber.New(), config()), true, func(ctx huma.Context) context.Context {
				return humafiber.Unwrap(ctx).Context()
			})
		}},
		{"fiber-v2", func() huma.API {
			return wrap(humafiber.NewV2(fiberV2.New(), config()), true, func(ctx huma.Context) context.Context {
				return humafiber.UnwrapV2(ctx).UserContext()
			})
		}},
		{"go", func() huma.API {
			return wrap(humago.New(http.NewServeMux(), config()), false, func(ctx huma.Context) context.Context {
				r, _ := humago.Unwrap(ctx)
				return r.Context()
			})
		}},
		{"humaflow", func() huma.API {
			return wrap(humaflow.New(flow.New(), config()), false, func(ctx huma.Context) context.Context {
				r, _ := humaflow.Unwrap(ctx)
				return r.Context()
			})
		}},
		{"gin", func() huma.API {
			return wrap(humagin.New(gin.New(), config()), false, func(ctx huma.Context) context.Context {
				return humagin.Unwrap(ctx).Request.Context()
			})
		}},
		{"httprouter", func() huma.API {
			return wrap(humahttprouter.New(httprouter.New(), config()), false, func(ctx huma.Context) context.Context {
				r, _, _ := humahttprouter.Unwrap(ctx)
				return r.Context()
			})
		}},
		{"mux", func() huma.API {
			return wrap(humamux.New(mux.NewRouter(), config()), false, func(ctx huma.Context) context.Context {
				r, _ := humamux.Unwrap(ctx)
				return r.Context()
			})
		}},
		{"bunrouter", func() huma.API {
			return wrap(humabunrouter.New(bunrouter.New(), config()), false, func(ctx huma.Context) context.Context {
				r, _ := humabunrouter.Unwrap(ctx)
				return r.Context()
			})
		}},
		{"bunroutercompat", func() huma.API {
			return wrap(humabunrouter.NewCompat(bunrouter.New().Compat(), config()), false, func(ctx huma.Context) context.Context {
				// FIXME: humabunrouter.Unwrap doesn't work with compat mode, so the
				// native context can't be read here to assert propagation.
				return nil
			})
		}},
	} {
		t.Run(adapter.name, func(t *testing.T) {
			testAdapter(t, adapter.new())
		})
	}
}
