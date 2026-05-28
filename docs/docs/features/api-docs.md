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

Each renderer takes its own options through `config.DocsRendererConfig`. Set it to any value that marshals to JSON (a `map[string]any` is easiest), and Huma writes it into the docs HTML. Scalar and SwaggerUI support it; Stoplight Elements ignores it.

If you'd rather control the HTML yourself, set `config.DocsPath` to an empty string to turn off the built-in docs, then register your own `/docs` route on the underlying router. The `DocsRenderer*` functions in [`api.go`](https://github.com/danielgtaylor/huma/blob/main/api.go) show what to return.

### Scalar Docs

[Scalar Docs](https://github.com/scalar/scalar#readme) provide a featureful and customizable API documentation experience that feels similar to Postman in your browser.

```go title="code.go"
router := chi.NewRouter()
config := huma.DefaultConfig("Docs Example", "1.0.0")
config.DocsRenderer = huma.DocsRendererScalar

// Optional. Scalar reads these from the `data-configuration` attribute. See
// https://github.com/scalar/scalar/blob/main/documentation/configuration.md
config.DocsRendererConfig = map[string]any{
	"theme":      "mars", // one of: default, alternate, moon, purple, solarized, ...
	"hideModels": true,   // hide the models section
}

api := humachi.New(router, config)
```

![Scalar Docs](./scalar.png)

### Stoplight Elements

[Stoplight Elements](https://stoplight.io/open-source/elements) is the default renderer, so you get it without setting `config.DocsRenderer` at all. It doesn't read `config.DocsRendererConfig`.

```go title="code.go"
router := chi.NewRouter()
config := huma.DefaultConfig("Docs Example", "1.0.0")

api := humachi.New(router, config)
```

![Stoplight Elements Stacked](./elements-stacked.png)

### SwaggerUI

[SwaggerUI](https://github.com/swagger-api/swagger-ui#readme) is an older but proven documentation generator that is widely used in the industry. It provides a more traditional API documentation experience.

```go title="code.go"
router := chi.NewRouter()
config := huma.DefaultConfig("Docs Example", "1.0.0")
config.DocsRenderer = huma.DocsRendererSwaggerUI

// Optional. These fields are merged into the SwaggerUIBundle config object. See
// https://swagger.io/docs/open-source-tools/swagger-ui/usage/configuration/
config.DocsRendererConfig = map[string]any{
	"defaultModelsExpandDepth": -1,   // hide the models section
	"tryItOutEnabled":          true, // enable "Try it out" by default
}

api := humachi.New(router, config)
```

![SwaggerUI](./swaggerui.png)

## Dive Deeper

-   Reference
    -   [`huma.Config`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) the API config
    -   [`huma.DefaultConfig`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultConfig) the default API config
