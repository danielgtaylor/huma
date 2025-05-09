<a href="#">
	<picture>
		<source media="(prefers-color-scheme: dark)" srcset="https://huma.rocks/huma-dark.png" />
		<source media="(prefers-color-scheme: light)" srcset="https://huma.rocks/huma.png" />
		<img alt="Huma Logo" src="https://huma.rocks/huma.png" />
	</picture>
</a>

[![HUMA Powered](https://img.shields.io/badge/Powered%20By-HUMA-f40273)](https://huma.rocks/) [![CI](https://github.com/danielgtaylor/huma/workflows/CI/badge.svg?branch=main)](https://github.com/danielgtaylor/huma/actions?query=workflow%3ACI+branch%3Amain++) [![codecov](https://codecov.io/gh/danielgtaylor/huma/branch/main/graph/badge.svg)](https://codecov.io/gh/danielgtaylor/huma) [![Docs](https://godoc.org/github.com/danielgtaylor/huma/v2?status.svg)](https://pkg.go.dev/github.com/danielgtaylor/huma/v2?tab=doc) [![Go Report Card](https://goreportcard.com/badge/github.com/danielgtaylor/huma/v2)](https://goreportcard.com/report/github.com/danielgtaylor/huma/v2)

[**🌎中文文档**](./README_CN.md)

- [What is huma?](#intro)
- [Install](#install)
- [Example](#example)
- [Documentation](#documentation)

<a name="intro"></a>
A modern, simple, fast & flexible micro framework for building HTTP REST/RPC APIs in Go backed by OpenAPI 3 and JSON Schema. Pronounced IPA: [/'hjuːmɑ/](https://en.wiktionary.org/wiki/Wiktionary:International_Phonetic_Alphabet). The goals of this project are to provide:

- Incremental adoption for teams with existing services
  - Bring your own router (including Go 1.22+), middleware, and logging/metrics
  - Extensible OpenAPI & JSON Schema layer to document existing routes
- A modern REST or HTTP RPC API backend framework for Go developers
  - Described by [OpenAPI 3.1](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.1.0.md) & [JSON Schema](https://json-schema.org/)
- Guard rails to prevent common mistakes
- Documentation that can't get out of date
- High-quality generated developer tooling

Features include:

- Declarative interface on top of your router of choice:
  - Operation & model documentation
  - Request params (path, query, header, or cookie)
  - Request body
  - Responses (including errors)
  - Response headers
- JSON Errors using [RFC9457](https://datatracker.ietf.org/doc/html/rfc9457) and `application/problem+json` by default (but can be changed)
- Per-operation request size limits with sane defaults
- [Content negotiation](https://developer.mozilla.org/en-US/docs/Web/HTTP/Content_negotiation) between server and client
  - Support for JSON ([RFC 8259](https://tools.ietf.org/html/rfc8259)) and optionally CBOR ([RFC 7049](https://tools.ietf.org/html/rfc7049)) content types via the `Accept` header with the default config.
- Conditional requests support, e.g. `If-Match` or `If-Unmodified-Since` header utilities.
- Optional automatic generation of `PATCH` operations that support:
  - [RFC 7386](https://www.rfc-editor.org/rfc/rfc7386) JSON Merge Patch
  - [RFC 6902](https://www.rfc-editor.org/rfc/rfc6902) JSON Patch
  - [Shorthand](https://github.com/danielgtaylor/shorthand) patches
- Annotated Go types for input and output models
  - Generates JSON Schema from Go types
  - Static typing for path/query/header params, bodies, response headers, etc.
  - Automatic input model validation & error handling
- Documentation generation using [Stoplight Elements](https://stoplight.io/open-source/elements)
- Optional CLI built-in, configured via arguments or environment variables
  - Set via e.g. `-p 8000`, `--port=8000`, or `SERVICE_PORT=8000`
  - Startup actions & graceful shutdown built-in
- Generates OpenAPI for access to a rich ecosystem of tools
  - Mocks with [API Sprout](https://github.com/danielgtaylor/apisprout) or [Prism](https://stoplight.io/open-source/prism)
  - SDKs with [OpenAPI Generator](https://github.com/OpenAPITools/openapi-generator) or [oapi-codegen](https://github.com/deepmap/oapi-codegen)
  - CLI with [Restish](https://rest.sh/)
  - And [plenty](https://openapi.tools/) [more](https://apis.guru/awesome-openapi3/category.html)
- Generates JSON Schema for each resource using optional `describedby` link relation headers as well as optional `$schema` properties in returned objects that integrate into editors for validation & completion.

This project was inspired by [FastAPI](https://fastapi.tiangolo.com/). Logo & branding designed by Kari Taylor.

## Sponsors

A big thank you to our current & former sponsors!

<div>
					<img width="1000" height="0" />
					<a href="https://zuplo.link/huma-gh ">
					<picture>
							<!-- <source media="(prefers-color-scheme: dark)" srcset="docs/zuplo-dark.png"> -->
							<img src="https://github.com/user-attachments/assets/aace5aa7-32bd-45cf-a8f8-2e352feaf017" alt="Zuplo" width="260" align="right">
					</picture>
					</a>
					<h3>Zuplo: Scale, Protect, and Productize your Huma API</h3>
					<p>
							Our API Gateway allows you to secure your API, scale it globally, generate documentation from your OpenAPI, and monetize your users.
					</p>
					<a href="https://zuplo.link/huma-gh ">Start for Free</a>
</div>
<hr/>

- [@bclements](https://github.com/bclements)
- [@bekabaz](https://github.com/bekabaz)
- [@victoraugustolls](https://github.com/victoraugustolls)
- [@phoenixtechnologies-io](https://github.com/phoenixtechnologies-io)
- [@chenjr0719](https://github.com/chenjr0719)
- [@vinogradovkonst](https://github.com/vinogradovkonst)
- [@miyamo2](https://github.com/miyamo2)
- [@nielskrijger](https://github.com/nielskrijger)

## Testimonials

> This is by far my favorite web framework for Go. It is inspired by FastAPI, which is also amazing, and conforms to many RFCs for common web things ... I really like the feature set, the fact that it [can use] Chi, and the fact that it is still somehow relatively simple to use. I've tried other frameworks and they do not spark joy for me. - [Jeb_Jenky](https://www.reddit.com/r/golang/comments/zhitcg/comment/izmg6vk/?utm_source=reddit&utm_medium=web2x&context=3)

> After working with #Golang for over a year, I stumbled upon Huma, the #FastAPI-inspired web framework. It’s the Christmas miracle I’ve been hoping for! This framework has everything! - [Hana Mohan](https://twitter.com/unamashana/status/1733088066053583197)

> I love Huma. Thank you, sincerely, for this awesome package. I’ve been using it for some time now and it’s been great! - [plscott](https://www.reddit.com/r/golang/comments/1aoshey/comment/kq6hcpd/?utm_source=reddit&utm_medium=web2x&context=3)

> Thank you Daniel for Huma. Superbly useful project and saves us a lot of time and hassle thanks to the OpenAPI gen — similar to FastAPI in Python. - [WolvesOfAllStreets](https://www.reddit.com/r/golang/comments/1aqj99d/comment/kqfqcml/?utm_source=reddit&utm_medium=web2x&context=3)

> Huma is wonderful, I've started working with it recently, and it's a pleasure, so thank you very much for your efforts 🙏 - [callmemicah](https://www.reddit.com/r/golang/comments/1b32ts4/comment/ksvr9h7/?utm_source=reddit&utm_medium=web2x&context=3)

> It took us 3 months to build our platform in Python with FastAPI, SQL Alchemy and only 3 weeks to rewrite it in Go with Huma and SQL C. Things just work and I seldomly have to debug where in Python I spent a majority of my time debugging. - [Bitclick\_](https://www.reddit.com/r/golang/comments/1cj2znb/comment/l2e4u6y/)

> Look at Huma, it's great. A nice slim layer on top of stdlib mux/chi and automatic body and parameter serialization, kinda feels like doing dotnet web APIs, but forces you to actually design request and response structs, which is great imo. - [Kirides](https://www.reddit.com/r/golang/comments/1fnn5c2/comment/lokuvpo/)

# Install

Install via `go get`. Note that Go 1.23 or newer is required.

```sh
# After: go mod init ...
go get -u github.com/danielgtaylor/huma/v2
```

# Example

Here is a complete basic hello world example in Huma, that shows how to initialize a Huma app complete with CLI, declare a resource operation, and define its handler function.

```go
package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/go-chi/chi/v5"

	_ "github.com/danielgtaylor/huma/v2/formats/cbor"
)

// Options for the CLI. Pass `--port` or set the `SERVICE_PORT` env var.
type Options struct {
	Port int `help:"Port to listen on" short:"p" default:"8888"`
}

// GreetingOutput represents the greeting operation response.
type GreetingOutput struct {
	Body struct {
		Message string `json:"message" example:"Hello, world!" doc:"Greeting message"`
	}
}

func main() {
	// Create a CLI app which takes a port option.
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// Add the operation handler to the API.
		huma.Get(api, "/greeting/{name}", func(ctx context.Context, input *struct{
			Name string `path:"name" maxLength:"30" example:"world" doc:"Name to greet"`
		}) (*GreetingOutput, error) {
			resp := &GreetingOutput{}
			resp.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)
			return resp, nil
		})

		// Tell the CLI how to start your router.
		hooks.OnStart(func() {
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), router)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
```

> [!TIP]
> Replace `chi.NewMux()` → `http.NewServeMux()` and `humachi.New` → `humago.New` to use the standard library router from Go 1.22+. Just make sure your `go.mod` has `go 1.22` or newer listed in it. Everything else stays the same! Switch whenever you are ready.

You can test it with `go run greet.go` (optionally pass `--port` to change the default) and make a sample request using [Restish](https://rest.sh/) (or `curl`):

```sh
# Get the message from the server
$ restish :8888/greeting/world
HTTP/1.1 200 OK
...
{
	$schema: "http://localhost:8888/schemas/GreetingOutputBody.json",
	message: "Hello, world!"
}
```

Even though the example is tiny you can also see some generated documentation at http://localhost:8888/docs. The generated OpenAPI is available at http://localhost:8888/openapi.json or http://localhost:8888/openapi.yaml.

Check out the [Huma tutorial](https://huma.rocks/tutorial/installation/) for a step-by-step guide to get started.

# Documentation

See the [https://huma.rocks/](https://huma.rocks/) website for full documentation in a presentation that's easier to navigate and search then this README. You can find the source for the site in the `docs` directory of this repo.

Official Go package documentation can always be found at https://pkg.go.dev/github.com/danielgtaylor/huma/v2.

# Articles & Mentions

- [APIs in Go with Huma 2.0](https://dgt.hashnode.dev/apis-in-go-with-huma-20)
- [Reducing Go Dependencies: A case study of dependency reduction in Huma](https://dgt.hashnode.dev/reducing-go-dependencies)
- [Golang News & Libs & Jobs shared on Twitter/X](https://twitter.com/golangch/status/1752175499701264532)
- Featured in Go Weekly [#495](https://golangweekly.com/issues/495) & [#498](https://golangweekly.com/issues/498)
- [Bump.sh Deploying Docs from Huma](https://docs.bump.sh/guides/bump-sh-tutorials/huma/)
- Mentioned in [Composable HTTP Handlers Using Generics](https://www.willem.dev/articles/generic-http-handlers/)

Be sure to star the project if you find it useful!

<a href="https://star-history.com/#danielgtaylor/huma&Date">
	<picture>
		<source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=danielgtaylor/huma&type=Date&theme=dark" />
		<source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=danielgtaylor/huma&type=Date" />
		<img alt="Star History Chart" src="https://api.star-history.com/svg?repos=danielgtaylor/huma&type=Date" />
	</picture>
</a>
