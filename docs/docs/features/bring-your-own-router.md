---
description: Bring your own router & middleware to enable incremental adoption across a large number of organizations.
---

# BYOR (Bring Your Own Router)

## BYOR (Bring Your Own Router) { .hidden }

Huma is designed to be router-agnostic to enable incremental adoption in existing and new services across a large number of organizations. This means you can use any router you want, or even write your own. The only requirement is an implementation of a small [`huma.Adapter`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Adapter) interface. This is how Huma integrates with your router.

Adapters are in the [`adapters`](https://github.com/danielgtaylor/huma/tree/main/adapters) directory and named after the router they support. Many common routers are supported out of the box:

-   [chi](https://github.com/go-chi/chi) via [`humachi`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humachi)
-   [gin](https://gin-gonic.com/) via [`humagin`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humagin)
-   [gorilla/mux](https://github.com/gorilla/mux) via [`humamux`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humamux)
-   [httprouter](https://github.com/julienschmidt/httprouter) via [`humahttprouter`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humahttprouter)
-   [Fiber](https://gofiber.io/) via [`humafiber`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/adapters/humafiber)

!!! info "New Adapters"

    Writing your own adapter is quick and simple, and PRs are accepted for additional adapters to be built-in.

## Chi Example

Adapters are instantiated by wrapping your router and providing a Huma configuration object which describes the API. Here is a simple example using Chi:

```go title="main.go"
import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// Create your router.
router := chi.NewMux()

// Wrap the router with Huma to create an API instance.
api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

// Register your operations with the API.
// ...

// Start the server!
http.ListenAndServe(":8888", r)
```

For existing services using Chi v4, you can use `humachi.NewV4` instead.

## Dive Deeper

The adapter converts a router-specific request context like `http.Request` or `fiber.Ctx` into the router-agnostic `huma.Context`, which is then used to call your operation's handler function.

```mermaid
graph LR
	Request([Request])
	OperationHandler[Operation Handler]

	Request --> Router
	Router -->|http.Request\nfiber.Ctx\netc| huma.Adapter
	subgraph huma.API
		huma.Adapter -->|huma.Context| OperationHandler
	end
```

-   Features
    -   [Registering operations](./operations.md)
-   Reference
    -   [`huma.Context`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Context) a router-agnostic request/response context
    -   [`huma.Adapter`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Adapter) the router-agnostic adapter interface
    -   [`huma.API`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#API) the API instance
    -   [`huma.NewAPI`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#NewAPI) creates an API instance (called by adapters)
    -   [`huma.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Register) registers new operations
