---
description: An overview of Huma features and a deep dive on how to use them.
---

# Features

Huma is a modern, simple, fast & flexible micro framework for building HTTP REST/RPC APIs in Golang backed by OpenAPI 3 and JSON Schema. Pronounced IPA: [/'hjuːmɑ/](https://en.wiktionary.org/wiki/Wiktionary:International_Phonetic_Alphabet). The goals of this project are to provide:

-   A modern REST or HTTP RPC API backend framework for Go developers
    -   Described by [OpenAPI 3.1](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.1.0.md) & [JSON Schema](https://json-schema.org/)
-   Incremental adoption for teams with existing services
    -   Bring your own router, middleware, and logging/metrics
    -   Extensible OpenAPI & JSON Schema layer to document existing routes
-   Guard rails to prevent common mistakes
-   Documentation that can't get out of date
-   High-quality generated developer tooling

Features include:

-   Declarative interface on top of your router of choice:
    -   Operation & model documentation
    -   Request params (path, query, or header)
    -   Request body
    -   Responses (including errors)
    -   Response headers
-   JSON Errors using [RFC9457](https://tools.ietf.org/html/rfc9457) and `application/problem+json` by default (but can be changed)
-   Per-operation request size limits with sane defaults
-   [Content negotiation](https://developer.mozilla.org/en-US/docs/Web/HTTP/Content_negotiation) between server and client
    -   Support for JSON ([RFC 8259](https://tools.ietf.org/html/rfc8259)) and optional CBOR ([RFC 7049](https://tools.ietf.org/html/rfc7049)) content types via the `Accept` header with the default config.
-   Conditional requests support, e.g. `If-Match` or `If-Unmodified-Since` header utilities.
-   Optional automatic generation of `PATCH` operations that support:
    -   [RFC 7386](https://www.rfc-editor.org/rfc/rfc7386) JSON Merge Patch
    -   [RFC 6902](https://www.rfc-editor.org/rfc/rfc6902) JSON Patch
    -   [Shorthand](https://github.com/danielgtaylor/shorthand) patches
-   Annotated Go types for input and output models
    -   Generates JSON Schema from Go types
    -   Static typing for path/query/header params, bodies, response headers, etc.
    -   Automatic input model validation & error handling
-   Documentation generation using [Stoplight Elements](https://stoplight.io/open-source/elements)
-   Optional CLI built-in, configured via arguments or environment variables
    -   Set via e.g. `-p 8000`, `--port=8000`, or `SERVICE_PORT=8000`
    -   Startup actions & graceful shutdown built-in
-   Generates OpenAPI for access to a rich ecosystem of tools
    -   Mocks with [API Sprout](https://github.com/danielgtaylor/apisprout) or [Prism](https://stoplight.io/open-source/prism)
    -   SDKs with [OpenAPI Generator](https://github.com/OpenAPITools/openapi-generator) or [oapi-codegen](https://github.com/deepmap/oapi-codegen)
    -   CLI with [Restish](https://rest.sh/)
    -   And [plenty](https://openapi.tools/) [more](https://apis.guru/awesome-openapi3/category.html)
-   Generates JSON Schema for each resource using optional `describedby` link relation headers as well as optional `$schema` properties in returned objects that integrate into editors for validation & completion.

!!! info "Mascot"

    Hi there! I'm the happy Huma whale here to provide help. You'll see me leave helpful tips throughout the docs.

Official Go package documentation can always be found at [https://pkg.go.dev/github.com/danielgtaylor/huma/v2](https://pkg.go.dev/github.com/danielgtaylor/huma/v2). Read on for an introduction to the various features available in Huma.
