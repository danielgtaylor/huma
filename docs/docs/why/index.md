---
description: Learn how Huma's proven production-ready technology can help your team ship APIs faster and with fewer bugs.
---

# Why Huma?

## Production Ready

Huma is a proven production-ready technology that has been used by large successful companies and products with millions of customers in the live streaming video space for years.

<!--
<div style="text-align: center;">
	<img src="../wbd.png" width="50%"/>
	<br/>
	<img src="../max.png" width="22%" style="margin-right: 2%"/> <img src="../cnn.svg" width="6%" style="margin-right: 2%"/> <img src="../march-madness.svg" width="15%" style="margin-right: 2%"/> <img src="../br.svg" width="19%">
</div>
-->

Huma is fast to learn, easy to use, performant, and lets your organization ship APIs and related tooling like interactive documentation, CLIs and SDKs faster and with fewer bugs caused by human-error and manual processes.

!!! quote "Daniel (Engineer @ Warner Bros Discovery)"

    Huma has been vital for quickly building consistent, standards-compliant, well-documented APIs and generated clients & SDKs for our live media streaming control plane services for configuring and running live news and sporting event channels. Teams have been able to ship faster and with fewer bugs since switching to Huma.

## Compatibility

Huma is broadly compatible with the libraries and tools your organization is already using. It is a micro-framework meant to level up your team's API development experience without getting in your way.

### Huma ❤️ Standards

Huma is built on top of open industry standards like [OpenAPI](https://www.openapis.org/), [JSON Schema](https://json-schema.org/), and dozens of RFCs and industry best practices.

This results in broad compatibility with other tools & systems, as well as the ability to generate and automate many pieces of your workflow, including client SDK generation, documentation, and more.

Well-known and understood standards means developers can get up to speed faster and spend less time learning new concepts. A new team can adopt Go and Huma and be productive in a matter of days.

[:material-arrow-right: Config & OpenAPI](../features/openapi-generation.md) <br/>
[:material-arrow-right: JSON Schema & Registry](../features/json-schema-registry.md) <br/>
[:material-arrow-right: Serialization](../features/response-serialization.md) <br/>
[:material-arrow-right: PATCH formats](../features/auto-patch.md)

### :fontawesome-brands-golang: Go is Awesome

Huma is built on Go, which is an easy to learn, performant, and extremely powerful [Top 10](https://www.tiobe.com/tiobe-index/go/) programming language.

Huma is built on top of idiomatic Go conventions and utilizes standard library concepts like `io.Reader`, `io.Writer`, `http.Request`, and more. This means that you can use many existing libraries with or alongside Huma.

Be sure to check out the [benchmarks](./benchmarks.md)!

#### Routers

Huma is router-agnostic and includes support for a handful of popular routers and their middleware your organization may already be using today:

-   [BunRouter](https://bunrouter.uptrace.dev/)
-   [chi](https://github.com/go-chi/chi)
-   [gin](https://gin-gonic.com/)
-   [Go 1.22+ `http.ServeMux`](https://pkg.go.dev/net/http@master#ServeMux)
-   [gorilla/mux](https://github.com/gorilla/mux)
-   [httprouter](https://github.com/julienschmidt/httprouter)
-   [Fiber](https://gofiber.io/)

Huma meets you where you are and levels up your API and team.

## Extensibility

Huma can be extended to support all your use-cases.

### Middleware

Flexible router-specific or router-agnostic middleware enables you to extend basic functionality with auth, metrics, traces, and more.

[:material-arrow-right: Middleware](../features/middleware.md)

### Validation

Huma has built-in support for validating input parameters and models using JSON Schema and/or custom Go code using resolvers, which can extend the built-in validation to do anything you want and returns exhaustive errors back to the user.

[:material-arrow-right: Request Validation](../features/request-validation.md) <br/>
[:material-arrow-right: Resolvers](../features/request-resolvers.md)

### OpenAPI & Schemas

The OpenAPI & JSON Schema generation is completely customizable & extensible. Huma provides low-level access and the ability to override or augment any generated specs and schemas.

[:material-arrow-right: Configuration & OpenAPI](../features/openapi-generation.md) <br/>
[:material-arrow-right: JSON Schema & Registry](../features/json-schema-registry.md)

## Guardrails

Huma provides guardrails & automation to keep your team and your services running as smoothly as possible, based on years of hard-learned lessons from many teams of engineers with a variety of skills and experience levels running and maintaining production systems at scale for millions of users.

-   Service documentation that can't get out of date
-   Strongly-typed models & handlers with compile-time checks
-   Automatic validation of input parameters and models
-   Automatic serialization of responses based on client-driven content-negotiation
-   Supports automatic CLI & SDK generation

[:material-arrow-right: Start the tutorial now](../tutorial/installation.md)
