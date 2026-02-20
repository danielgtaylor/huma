---
description: Customizing the generated API documentation using third-party tools like Stoplight Elements, Scalar Docs, and SwaggerUI.
---

# Generated API Documentation

## Generated API Docs { .hidden }

Huma uses the OpenAPI spec to generate interactive API documentation using third-party tools. By default, [Stoplight Elements](https://stoplight.io/open-source/elements) is used to render the documentation at the API's `config.DocsPath` which defaults to `/docs`.

You can switch to other documentation renderers using `config.DocsRenderer`. The following renderers are supported out of the box:

- `huma.DocsRendererStoplightElements` (default)
- `huma.DocsRendererScalar`
- `huma.DocsRendererSwaggerUI`

![Stoplight Elements](./elements.png)

!!! info "Disabling the Docs"

    You can disable the built-in documentation by setting `config.DocsPath` to an empty string. This allows you to provide your own documentation renderer if you wish.

!!! warning "Middleware Conflicts"

    Some middleware can interfere with the documentation renderer's ability to fetch the OpenAPI spec. For example, [go-chi/chi](https://github.com/go-chi/chi)'s `middleware.URLFormat` will rewrite URLs that end in `.json` or `.yaml` (e.g. `/openapi.json` -> `/openapi`), which can lead to 404 errors for the spec. If you encounter this, consider disabling that middleware or configuring it to skip the OpenAPI and documentation paths.

## Customizing Documentation

You can customize the generated documentation by providing your own renderer function to the API adapter or by using the underlying router directly.

### Scalar Docs

[Scalar Docs](https://github.com/scalar/scalar?tab=readme-ov-file#readme) provide a featureful and customizable API documentation experience that feels similar to Postman in your browser.

```go title="code.go"
router := chi.NewRouter()
config := huma.DefaultConfig("Docs Example", "1.0.0")
config.DocsPath = ""

api := humachi.New(router, config)

router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
	// Please also refer to the "DocsRendererScalar" renderer code inside api.go on what to return here
	csp := []string{
		"default-src 'none'",
		"base-uri 'none'",
		"connect-src 'self'",
		"form-action 'none'",
		"frame-ancestors 'none'",
		"sandbox allow-same-origin allow-scripts",
		"script-src 'unsafe-eval' https://unpkg.com/@scalar/api-reference@1.44.20/dist/browser/standalone.js", // TODO: Somehow drop 'unsafe-eval'
		"style-src 'unsafe-inline'", // TODO: Somehow drop 'unsafe-inline'
	}
	w.Header().Set("Content-Security-Policy", strings.Join(csp, "; "))
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="referrer" content="no-referrer">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>API Reference</title>
  </head>
  <body>
    <script id="api-reference" data-url="/openapi.json"></script>
    <script src="https://unpkg.com/@scalar/api-reference@1.44.20/dist/browser/standalone.js" crossorigin integrity="sha384-tMz7GAo6dMy55x9tLFtH+sHtogji6Scmb+feBR31TAHmvSPRUTboK9H3M5NFaP4R"></script>
  </body>
</html>`))
})
```

![Scalar Docs](./scalar.png)

### Stoplight Elements

You can customize the default docs by providing your own HTML so you can set the layout, styles, colors, etc as needed.

```go title="code.go"
router := chi.NewRouter()
config := huma.DefaultConfig("Docs Example", "1.0.0")
config.DocsPath = ""

api := humachi.New(router, config)

router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
	// Please refer to the "DocsRendererStoplightElements" renderer code inside api.go on what to return here
})
```

![Stoplight Elements Stacked](./elements-stacked.png)

### SwaggerUI

[SwaggerUI](https://github.com/swagger-api/swagger-ui#readme) is an older but proven documentation generator that is widely used in the industry. It provides a more traditional API documentation experience.

```go title="code.go"
router := chi.NewRouter()
config := huma.DefaultConfig("Docs Example", "1.0.0")
config.DocsPath = ""

api := humachi.New(router, config)

router.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
	// Please refer to the "DocsRendererSwaggerUI" renderer code inside api.go on what to return here
})
```

![SwaggerUI](./swaggerui.png)

## Dive Deeper

-   Reference
    -   [`huma.Config`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) the API config
    -   [`huma.DefaultConfig`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultConfig) the default API config
