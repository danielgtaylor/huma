package huma_test

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		assert.Contains(t, resp.Body.String(), `data-configuration="`)
		assert.Contains(t, resp.Body.String(), `&#34;theme&#34;:&#34;mars&#34;`)
		assert.Contains(t, resp.Body.String(), `&#34;hideModels&#34;:true`)
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
				"persistAuthorization":     true,
			},
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		resp := api.Get("/docs")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Body.String(), `data-config="`)
		assert.Contains(t, resp.Body.String(), `&#34;defaultModelsExpandDepth&#34;:-1`)
		assert.Contains(t, resp.Body.String(), `&#34;tryItOutEnabled&#34;:true`)
		assert.Contains(t, resp.Body.String(), `&#34;persistAuthorization&#34;:true`)
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

func TestDocsAdapter(t *testing.T) {
	t.Run("Docs", func(t *testing.T) {
		docsAdapter := humatest.NewAdapter()
		_, api := humatest.New(t, huma.Config{
			OpenAPI: &huma.OpenAPI{
				Info: &huma.Info{Title: "Test API", Version: "1.0.0"},
				Servers: []*huma.Server{
					{URL: "https://example.com/api/v1"},
				},
			},
			DocsPath:    "/docs",
			DocsAdapter: docsAdapter,
			OpenAPIPath: "/openapi",
			Formats:     huma.DefaultFormats,
		})

		// The main adapter does not serve the docs or the spec.
		assert.Equal(t, http.StatusNotFound, api.Get("/docs").Code)
		assert.Equal(t, http.StatusNotFound, api.Get("/openapi.yaml").Code)

		w := httptest.NewRecorder()
		docsAdapter.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/docs", nil))
		assert.Equal(t, http.StatusOK, w.Code)
		// The server prefix applies to the main adapter only, so the spec URL is
		// relative to the docs page.
		assert.Contains(t, w.Body.String(), `apiDescriptionUrl="openapi.yaml"`)

		w = httptest.NewRecorder()
		docsAdapter.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("NestedDocsPath", func(t *testing.T) {
		for docsPath, specURL := range map[string]string{
			"/reference/docs": "../openapi.yaml",
			"/docs/":          "../openapi.yaml",
			"/a/b/docs":       "../../openapi.yaml",
		} {
			docsAdapter := humatest.NewAdapter()
			humatest.New(t, huma.Config{
				OpenAPI:     &huma.OpenAPI{Info: &huma.Info{Title: "Test API", Version: "1.0.0"}},
				DocsPath:    docsPath,
				DocsAdapter: docsAdapter,
				OpenAPIPath: "/openapi",
				Formats:     huma.DefaultFormats,
			})

			w := httptest.NewRecorder()
			docsAdapter.ServeHTTP(w, httptest.NewRequest(http.MethodGet, docsPath, nil))
			assert.Equal(t, http.StatusOK, w.Code, docsPath)
			assert.Contains(t, w.Body.String(), `apiDescriptionUrl="`+specURL+`"`, docsPath)
		}
	})

	t.Run("NestedSchemaRef", func(t *testing.T) {
		type Inner struct {
			Value string `json:"value"`
		}
		type Outer struct {
			Inner Inner `json:"inner"`
		}

		docsAdapter := humatest.NewAdapter()
		config := huma.DefaultConfig("Test API", "1.0.0")
		config.DocsAdapter = docsAdapter
		config.CreateHooks = []func(huma.Config) huma.Config{
			huma.DefaultSchemaLinkHook("/internal"),
		}
		_, api := humatest.New(t, config)

		huma.Get(api, "/outer", func(ctx context.Context, input *struct{}) (*struct{ Body Outer }, error) {
			return &struct{ Body Outer }{}, nil
		})

		// The nested ref is relative, so it works under any mount path.
		w := httptest.NewRecorder()
		docsAdapter.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/schemas/Outer.json", nil))
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"$ref":"Inner.json"`)
	})

	t.Run("Schemas", func(t *testing.T) {
		type Greeting struct {
			Message string `json:"message"`
		}

		docsAdapter := humatest.NewAdapter()
		config := huma.DefaultConfig("Test API", "1.0.0")
		config.Servers = []*huma.Server{{URL: "https://api.example.com/api/v1"}}
		config.DocsAdapter = docsAdapter
		config.CreateHooks = []func(huma.Config) huma.Config{
			huma.DefaultSchemaLinkHook("/internal"),
		}
		_, api := humatest.New(t, config)

		huma.Get(api, "/greeting", func(ctx context.Context, input *struct{}) (*struct{ Body Greeting }, error) {
			return &struct{ Body Greeting }{Body: Greeting{Message: "hello"}}, nil
		})

		// The hook prefix replaces the server prefix, because the docs adapter
		// serves the schemas.
		resp := api.Get("/greeting")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, `</internal/schemas/Greeting.json>; rel="describedBy"`, resp.Header().Get("Link"))
		assert.Contains(t, resp.Body.String(), `"$schema":"https://api.example.com/internal/schemas/Greeting.json"`)

		schema := api.OpenAPI().Components.Schemas.Map()["Greeting"]
		assert.Equal(t, []any{"https://api.example.com/internal/schemas/Greeting.json"}, schema.Properties["$schema"].Examples)

		// The docs adapter serves the schemas, the main adapter does not.
		assert.Equal(t, http.StatusNotFound, api.Get("/schemas/Greeting.json").Code)
		w := httptest.NewRecorder()
		docsAdapter.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/schemas/Greeting.json", nil))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("SchemaLinkPrefixRoot", func(t *testing.T) {
		config := huma.DefaultConfig("Test API", "1.0.0")
		config.Servers = []*huma.Server{{URL: "https://api.example.com/api/v1"}}
		config.DocsAdapter = humatest.NewAdapter()
		config.CreateHooks = []func(huma.Config) huma.Config{
			huma.DefaultSchemaLinkHook("/"),
		}
		_, api := humatest.New(t, config)

		huma.Get(api, "/greeting", func(ctx context.Context, input *struct{}) (*struct{ Body struct{} }, error) {
			return &struct{ Body struct{} }{}, nil
		})

		resp := api.Get("/greeting")
		assert.Equal(t, http.StatusOK, resp.Code)
		assert.Equal(t, `</schemas/Get-greetingResponse.json>; rel="describedBy"`, resp.Header().Get("Link"))
	})

	t.Run("SchemaLinkPrefixMissing", func(t *testing.T) {
		config := huma.DefaultConfig("Test API", "1.0.0")
		config.DocsAdapter = humatest.NewAdapter()
		assert.PanicsWithValue(t, `DocsAdapter serves the schemas, so DefaultSchemaLinkHook needs the path where DocsAdapter is mounted (use "/" for the root)`, func() {
			humatest.New(t, config)
		})
	})

	t.Run("SchemaLinkPrefixInvalid", func(t *testing.T) {
		assert.PanicsWithValue(t, "schema link path prefix must start with a slash: internal", func() {
			huma.DefaultSchemaLinkHook("internal")
		})
	})
}
