package huma_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlankConfig(t *testing.T) {
	adapter := humatest.NewAdapter()

	assert.NotPanics(t, func() {
		huma.NewAPI(huma.Config{}, adapter)
	})
}

// ExampleAdapter_handle demonstrates how to use the adapter directly
// instead of using the `huma.Register` convenience function to add a new
// operation and handler to the API.
//
// Note that you are responsible for defining all of the operation details,
// including the parameter and response definitions & schemas.
func ExampleAdapter_handle() {
	// Create an adapter for your chosen router.
	adapter := NewExampleAdapter()

	// Register an operation with a custom handler.
	adapter.Handle(&huma.Operation{
		OperationID: "example-operation",
		Method:      "GET",
		Path:        "/example/{name}",
		Summary:     "Example operation",
		Parameters: []*huma.Param{
			{
				Name:        "name",
				In:          "path",
				Description: "Name to return",
				Required:    true,
				Schema: &huma.Schema{
					Type: "string",
				},
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "OK",
				Content: map[string]*huma.MediaType{
					"text/plain": {
						Schema: &huma.Schema{
							Type: "string",
						},
					},
				},
			},
		},
	}, func(ctx huma.Context) {
		// Get the `name` path parameter.
		name := ctx.Param("name")

		// Set the response content type, status code, and body.
		ctx.SetHeader("Content-Type", "text/plain; charset=utf-8")
		ctx.SetStatus(http.StatusOK)
		ctx.BodyWriter().Write([]byte("Hello, " + name))
	})
}

func TestContextValue(t *testing.T) {
	_, api := humatest.New(t)

	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		orig := ctx
		// Make an updated context available to the handler.
		ctx = huma.WithValue(ctx, "foo", "bar")
		next(ctx)
		assert.Equal(t, http.StatusNoContent, ctx.Status())
		// The status the handler set must also be visible from the pre-fork
		// context, since logging / telemetry middleware hold on to that one and
		// can't know whether anything downstream forked again.
		assert.Equal(t, http.StatusNoContent, orig.Status())
	})

	// Register a simple hello world operation in the API.
	huma.Get(api, "/test", func(ctx context.Context, input *struct{}) (*struct{}, error) {
		assert.Equal(t, "bar", ctx.Value("foo"))
		return nil, nil
	})

	resp := api.Get("/test")
	assert.Equal(t, http.StatusNoContent, resp.Code)
}

func TestResponseContentTypeWithExtensions(t *testing.T) {
	_, api := humatest.New(t)

	type output struct {
		ContentType string `header:"Content-Type"`
		Body        struct {
			Foo string `json:"foo"`
		}
	}

	huma.Get(api, "/charset", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/json; charset=utf-8",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	huma.Get(api, "/suffix", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/problem+json",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	huma.Get(api, "/both", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/problem+json; charset=utf-8",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	assert.NotPanics(t, func() {
		resp := api.Get("/charset")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/json; charset=utf-8", resp.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"foo": "bar"}`, resp.Body.String())
	})

	assert.NotPanics(t, func() {
		resp := api.Get("/suffix")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/problem+json", resp.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"foo": "bar"}`, resp.Body.String())
	})

	assert.NotPanics(t, func() {
		resp := api.Get("/both")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "application/problem+json; charset=utf-8", resp.Header().Get("Content-Type"))
		assert.JSONEq(t, `{"foo": "bar"}`, resp.Body.String())
	})

	t.Run("UnmarshalSuffix", func(t *testing.T) {
		type input struct {
			Foo string `json:"foo"`
		}
		var v input
		err := api.Unmarshal("application/problem+json; charset=utf-8", []byte(`{"foo": "bar"}`), &v)
		require.NoError(t, err)
		assert.Equal(t, "bar", v.Foo)
	})

	tRunUnmarshalError := func(name, ct string) {
		t.Run(name, func(t *testing.T) {
			var v struct{}
			err := api.Unmarshal(ct, []byte(`{}`), &v)
			require.Error(t, err)
		})
	}

	tRunUnmarshalError("UnmarshalErrorMalformed", "application/json; charset=utf-8+wrong")

	huma.Get(api, "/malformed", func(ctx context.Context, input *struct{}) (*output, error) {
		return &output{
			ContentType: "application/json; charset=utf-8+wrong",
			Body: struct {
				Foo string `json:"foo"`
			}{Foo: "bar"},
		}, nil
	})

	t.Run("PanicOnMalformed", func(t *testing.T) {
		assert.Panics(t, func() {
			api.Get("/malformed")
		})
	})
}

func TestDocsRenderers(t *testing.T) {
	t.Run("DefaultRenderer", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:    "/docs",
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, "text/html", resp.Header().Get("Content-Type"))
		assert.Contains(t, resp.Body.String(), "@stoplight/elements")
	})

	t.Run("ScalarRenderer", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererScalar,
			OpenAPIPath:  "/openapi",
			Formats:      huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), "@scalar/api-reference")
		assert.Contains(t, resp.Body.String(), "@scalar/api-reference@1.66.1")

		csp := resp.Header().Get("Content-Security-Policy")
		assert.NotContains(t, csp, "unsafe-eval")
		assert.Contains(t, csp, "style-src 'unsafe-inline'")
		assert.Contains(t, csp, "font-src https://fonts.scalar.com")
		assert.Regexp(t, `script-src [^;]+ 'sha256-[A-Za-z0-9+/]+=*'`, csp)
		assert.NotContains(t, csp, "'nonce-")
	})

	t.Run("ScalarRendererConfig", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererScalar,
			DocsRendererConfig: map[string]any{
				"theme":      "mars",
				"hideModels": true,
			},
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), `Scalar.createApiReference('#app', `)
		assert.Contains(t, resp.Body.String(), `"theme":"mars"`)
		assert.Contains(t, resp.Body.String(), `"hideModels":true`)
		assert.Contains(t, resp.Body.String(), `"url":"/openapi.json"`)
	})

	t.Run("ScalarRendererMultiDocument", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererScalar,
			DocsRendererConfig: map[string]any{
				"sources": []map[string]any{
					{"url": "/openapi.json", "title": "Main"},
					{"url": "/other/openapi.json", "title": "Other"},
				},
			},
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		body := resp.Body.String()
		assert.Contains(t, body, `"sources":[`)
		assert.Contains(t, body, `"title":"Other"`)
		assert.NotContains(t, body, `"url":"/openapi.json","sources"`)
		assert.NotRegexp(t, `\{"sources":\[.*\],"url"`, body)
	})

	t.Run("ScalarRendererConfigNotObject", func(t *testing.T) {
		assert.Panics(t, func() {
			humatest.New(t, huma.Config{
				OpenAPI: &huma.OpenAPI{
					Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				},
				DocsPath:           "/docs",
				DocsRenderer:       huma.DocsRendererScalar,
				DocsRendererConfig: []string{"not", "an", "object"},
				OpenAPIPath:        "/openapi",
				Formats:            huma.DefaultFormats,
			})
		})
	})

	t.Run("ScalarRendererConfigInvalid", func(t *testing.T) {
		assert.Panics(t, func() {
			humatest.New(t, huma.Config{
				OpenAPI: &huma.OpenAPI{
					Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				},
				DocsPath:           "/docs",
				DocsRenderer:       huma.DocsRendererScalar,
				DocsRendererConfig: make(chan int),
				OpenAPIPath:        "/openapi",
				Formats:            huma.DefaultFormats,
			})
		})
	})

	t.Run("ScalarCustomScript", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:            "/docs",
			DocsRenderer:        huma.DocsRendererScalar,
			DocsScriptURL:       "https://cdn.example.com/scalar/standalone.js",
			DocsScriptIntegrity: "sha384-custom",
			OpenAPIPath:         "/openapi",
			Formats:             huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), `src="https://cdn.example.com/scalar/standalone.js"`)
		assert.Contains(t, resp.Body.String(), `integrity="sha384-custom"`)
		assert.NotContains(t, resp.Body.String(), "@scalar/api-reference")
		assert.Contains(t, resp.Header().Get("Content-Security-Policy"), "script-src https://cdn.example.com/scalar/standalone.js")
	})

	t.Run("ScalarCustomScriptNoIntegrity", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:      "/docs",
			DocsRenderer:  huma.DocsRendererScalar,
			DocsScriptURL: "https://cdn.example.com/scalar/standalone.js",
			OpenAPIPath:   "/openapi",
			Formats:       huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), `src="https://cdn.example.com/scalar/standalone.js"`)
		assert.NotContains(t, resp.Body.String(), "integrity=")
	})

	t.Run("ScalarCustomFontSrc", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererScalar,
			DocsFontSrc:  "'self' data:",
			OpenAPIPath:  "/openapi",
			Formats:      huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		csp := resp.Header().Get("Content-Security-Policy")
		assert.Contains(t, csp, "font-src 'self' data:")
		assert.NotContains(t, csp, "fonts.scalar.com")
	})

	t.Run("ScalarInvalidFontSrc", func(t *testing.T) {
		assert.Panics(t, func() {
			humatest.New(t, huma.Config{
				OpenAPI: &huma.OpenAPI{
					Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				},
				DocsPath:     "/docs",
				DocsRenderer: huma.DocsRendererScalar,
				DocsFontSrc:  "https://fonts.example.com; script-src 'unsafe-eval'",
				OpenAPIPath:  "/openapi",
				Formats:      huma.DefaultFormats,
			})
		})
	})

	t.Run("ScalarInvalidScriptURL", func(t *testing.T) {
		assert.Panics(t, func() {
			humatest.New(t, huma.Config{
				OpenAPI: &huma.OpenAPI{
					Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				},
				DocsPath:      "/docs",
				DocsRenderer:  huma.DocsRendererScalar,
				DocsScriptURL: "https://cdn.example.com/a.js; script-src 'unsafe-eval'",
				OpenAPIPath:   "/openapi",
				Formats:       huma.DefaultFormats,
			})
		})
	})

	t.Run("SwaggerUIRenderer", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererSwaggerUI,
			OpenAPIPath:  "/openapi",
			Formats:      huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), "swagger-ui-dist")
	})

	t.Run("SwaggerUIRendererConfig", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
			},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererSwaggerUI,
			DocsRendererConfig: map[string]any{
				"defaultModelsExpandDepth": -1,
				"tryItOutEnabled":          true,
			},
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), `data-config="`)
		assert.Contains(t, resp.Body.String(), `&#34;defaultModelsExpandDepth&#34;:-1`)
		assert.Contains(t, resp.Body.String(), `&#34;tryItOutEnabled&#34;:true`)
	})

	t.Run("SwaggerUIRendererConfigInvalid", func(t *testing.T) {
		assert.Panics(t, func() {
			humatest.New(t, huma.Config{
				OpenAPI: &huma.OpenAPI{
					Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				},
				DocsPath:           "/docs",
				DocsRenderer:       huma.DocsRendererSwaggerUI,
				DocsRendererConfig: make(chan int),
				OpenAPIPath:        "/openapi",
				Formats:            huma.DefaultFormats,
			})
		})
	})

	t.Run("APIPrefix", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				Servers: []*huma.Server{
					{URL: "https://example.com/api/v1"},
				},
			},
			DocsPath:    "/docs",
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		// Elements uses apiDescriptionUrl=".../api/v1/openapi.yaml"
		assert.Contains(t, resp.Body.String(), `apiDescriptionUrl="/api/v1/openapi.yaml"`)
	})

	t.Run("APIPrefixWithServerVars", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				Servers: []*huma.Server{
					{
						URL: "http://localhost:{port}/api/{version}",
						Variables: map[string]*huma.ServerVariable{
							"port": {
								Default: "8080",
							},
							"version": {
								Enum: []string{"v1", "v2"},
							},
						},
					},
				},
			},
			DocsPath:    "/docs",
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		// Elements uses apiDescriptionUrl=".../api/v1/openapi.yaml"
		assert.Contains(t, resp.Body.String(), `apiDescriptionUrl="/api/v1/openapi.yaml"`)
	})

	t.Run("APIPrefixRelative", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				Servers: []*huma.Server{
					{URL: "/api/v1"},
				},
			},
			DocsPath:    "/docs",
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), `apiDescriptionUrl="/api/v1/openapi.yaml"`)
	})

	t.Run("APIPrefixEmptyURL", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Servers: []*huma.Server{
					{URL: ""},
					{URL: "/api/v1"},
				},
			},
			DocsPath:    "/docs",
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), `apiDescriptionUrl="/api/v1/openapi.yaml"`)
	})

	t.Run("APIPrefixInvalidURL", func(t *testing.T) {
		assert.PanicsWithValue(t, "invalid server URL:  :invalid ( :invalid): parse \" :invalid\": first path segment in URL cannot contain colon", func() {
			humatest.New(t, huma.Config{
				OpenAPI: &huma.OpenAPI{
					Servers: []*huma.Server{
						{URL: " :invalid"},
						{URL: "/api/v2"},
					},
				},
				DocsPath:    "/docs",
				OpenAPIPath: "/openapi",
				Formats:     huma.DefaultFormats,
			})
		})
	})

	t.Run("ScalarNoTitle", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				// No Info or Title
			},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererScalar,
			OpenAPIPath:  "/openapi",
			Formats:      huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), "<title>Scalar in HTML</title>")
	})
	t.Run("ElementsNoTitle", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI:     &huma.OpenAPI{},
			DocsPath:    "/docs",
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), "<title>Elements in HTML</title>")
	})

	t.Run("SwaggerUINoTitle", func(t *testing.T) {
		_, api := humatest.New(t, huma.Config{
			OpenAPI:      &huma.OpenAPI{},
			DocsPath:     "/docs",
			DocsRenderer: huma.DocsRendererSwaggerUI,
			OpenAPIPath:  "/openapi",
			Formats:      huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), "<title>SwaggerUI in HTML</title>")
	})
	t.Run("UnknownRenderer", func(t *testing.T) {
		assert.Panics(t, func() {
			humatest.New(t, huma.Config{
				OpenAPI:      &huma.OpenAPI{},
				DocsPath:     "/docs",
				DocsRenderer: "unknown",
				OpenAPIPath:  "/openapi",
				Formats:      huma.DefaultFormats,
			})
		})
	})
}
