// Package adapters_test runs basic verification tests on all adapters.
package adapters_test

import (
	"context"
	"fmt"
	"mime/multipart"
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

// adapters lists every adapter so each test covers all of them.
var adapters = []struct {
	name          string
	new           func(huma.Config) huma.API
	isFiber       bool
	nativeContext func(ctx huma.Context) context.Context
}{
	{"chi", func(c huma.Config) huma.API { return humachi.New(chi.NewMux(), c) }, false, func(ctx huma.Context) context.Context {
		r, _ := humachi.Unwrap(ctx)
		return r.Context()
	}},
	{"echo", func(c huma.Config) huma.API { return humaecho.New(echo.New(), c) }, false, func(ctx huma.Context) context.Context {
		return humaecho.Unwrap(ctx).Request().Context()
	}},
	{"echo-v4", func(c huma.Config) huma.API { return humaecho.NewV4(echoV4.New(), c) }, false, func(ctx huma.Context) context.Context {
		return humaecho.UnwrapV4(ctx).Request().Context()
	}},
	{"fiber", func(c huma.Config) huma.API { return humafiber.New(fiber.New(), c) }, true, func(ctx huma.Context) context.Context {
		return humafiber.Unwrap(ctx).Context()
	}},
	{"fiber-v2", func(c huma.Config) huma.API { return humafiber.NewV2(fiberV2.New(), c) }, true, func(ctx huma.Context) context.Context {
		return humafiber.UnwrapV2(ctx).UserContext()
	}},
	{"go", func(c huma.Config) huma.API { return humago.New(http.NewServeMux(), c) }, false, func(ctx huma.Context) context.Context {
		r, _ := humago.Unwrap(ctx)
		return r.Context()
	}},
	{"humaflow", func(c huma.Config) huma.API { return humaflow.New(flow.New(), c) }, false, func(ctx huma.Context) context.Context {
		r, _ := humaflow.Unwrap(ctx)
		return r.Context()
	}},
	{"gin", func(c huma.Config) huma.API { return humagin.New(gin.New(), c) }, false, func(ctx huma.Context) context.Context {
		return humagin.Unwrap(ctx).Request.Context()
	}},
	{"httprouter", func(c huma.Config) huma.API { return humahttprouter.New(httprouter.New(), c) }, false, func(ctx huma.Context) context.Context {
		r, _, _ := humahttprouter.Unwrap(ctx)
		return r.Context()
	}},
	{"mux", func(c huma.Config) huma.API { return humamux.New(mux.NewRouter(), c) }, false, func(ctx huma.Context) context.Context {
		r, _ := humamux.Unwrap(ctx)
		return r.Context()
	}},
	{"bunrouter", func(c huma.Config) huma.API { return humabunrouter.New(bunrouter.New(), c) }, false, func(ctx huma.Context) context.Context {
		r, _ := humabunrouter.Unwrap(ctx)
		return r.Context()
	}},
	{"bunroutercompat", func(c huma.Config) huma.API { return humabunrouter.NewCompat(bunrouter.New().Compat(), c) }, false, func(ctx huma.Context) context.Context {
		// FIXME: humabunrouter.Unwrap doesn't work with compat mode, so the
		// native context can't be read here to assert propagation.
		return nil
	}},
}

func TestAdapters(t *testing.T) {
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

			orig := ctx

			// Set a request-scoped value that downstream native middleware must be
			// able to read from the underlying request context.
			ctx = huma.WithValue(ctx, key{}, "value")

			next(ctx)

			// The status set by the handler must be observable from the context
			// this middleware started with, not just from the forked one.
			assert.Equal(t, http.StatusOK, orig.Status())
			assert.Equal(t, http.StatusOK, ctx.Status())
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

	for _, adapter := range adapters {
		t.Run(adapter.name, func(t *testing.T) {
			testAdapter(t, wrap(adapter.new(huma.DefaultConfig("Test", "1.0.0")), adapter.isFiber, adapter.nativeContext))
		})
	}
}

// TestAdaptersMultipartLimit checks that each adapter enforces MaxBodyBytes on
// multipart bodies, as each one parses the form itself.
func TestAdaptersMultipartLimit(t *testing.T) {
	const body = "--B\r\nContent-Disposition: form-data; name=\"f\"\r\n\r\nhello\r\n--B--\r\n"

	for _, adapter := range adapters {
		for _, tc := range []struct {
			name     string
			maxBytes int64
			status   int
		}{
			{"at-limit", int64(len(body)), http.StatusNoContent},
			{"over-limit", int64(len(body)) - 1, http.StatusRequestEntityTooLarge},
		} {
			t.Run(adapter.name+"/"+tc.name, func(t *testing.T) {
				api := adapter.new(huma.DefaultConfig("Test", "1.0.0"))
				huma.Register(api, huma.Operation{
					Method:       http.MethodPost,
					Path:         "/upload",
					MaxBodyBytes: tc.maxBytes,
				}, func(ctx context.Context, input *struct {
					RawBody multipart.Form
				}) (*struct{}, error) {
					assert.Equal(t, []string{"hello"}, input.RawBody.Value["f"])
					return nil, nil
				})

				resp := humatest.Wrap(t, api).Post("/upload",
					"Content-Type: multipart/form-data; boundary=B",
					strings.NewReader(body),
				)
				assert.Equal(t, tc.status, resp.Code, resp.Body.String())
			})
		}
	}
}
