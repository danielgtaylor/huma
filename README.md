# Huma REST API Framework

A modern, simple & fast REST API framework for Go. The goals of this project are to provide:

- A modern REST API backend framework for Go
  - Described by [OpenAPI 3](https://github.com/OAI/OpenAPI-Specification/blob/master/versions/3.0.3.md) & [JSON Schema](https://json-schema.org/)
  - First class support for middleware, JSON, and other features
- Documentation that can't get out of date
- High-quality developer tooling

Features include:

- Declarative interface on top of [Gin](https://github.com/gin-gonic/gin)
  - Documentation
  - Params
  - Request body
  - Responses (including errors)
- Annotated Go types for input and output models
- Documentation generation using [Redoc](https://github.com/Redocly/redoc)

This project was inspired by [FastAPI](https://fastapi.tiangolo.com/), [Gin](https://github.com/gin-gonic/gin), and countless others.

## Concepts & Example

REST APIs are composed of operations against resources and can include descriptions of various inputs and possible outputs. Huma uses standard Go types and a declarative API to capture those descriptions in order to provide a combination of idiomatic code, strong typing, and a rich ecosystem of tools for docs, mocks, generated SDK clients, and generated CLIs.

See the `example/main.go` file for a simple example.
