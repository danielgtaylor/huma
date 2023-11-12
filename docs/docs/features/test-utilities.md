---
description: Test your API with the built-in test utilities.
---

# Test Utilities

Huma includes a [`humatest`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/humatest) package to make it easier to write tests for your API.

## Creating a Test API

The first step is to create a test API instance. This is a router-agnostic API instance that you can register routes against and then make requests against.

```go title="code.go"
import (
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestMyAPI(t *testing.T) {
	router, api := humatest.New(t)
}
```

## Registering Routes

The test API is the same as any other API, and you can register routes to it the same as you would your main API instance.

```go title="code.go" hl_lines="4-5"
func TestMyAPI(t *testing.T) {
	router, api := humatest.New(t)

	// Register routes...
	addRoutes(api)
}
```

## Making Requests

Once you have registered your routes, you can make requests against the test API instance. The `Get`, `Post`, `Put`, `Patch`, and `Delete` convenience methods are available on the test API object.

```go title="code.go" hl_lines="7-8 10-16"
func TestMyAPI(t *testing.T) {
	router, api := humatest.New(t)

	// Register routes...
	addRoutes(api)

	// Make a GET request
	resp := api.Get("/some/path?foo=bar")

	// Make a PUT request
	resp = api.Put("/some/path",
		"My-Header: abc123",
		map[string]any{
			"author": "daniel",
			"rating": 5,
		})
}
```

The request convenience methods take a URL path followed by any number of optional arguments. If the argument is a string, it is treated as a header, if it is an `io.Reader` is is treated as the raw body, otherwise it is marshalled as JSON and used as the request body.

## Assertions

The request convenience methods return a `*httptest.ResponseRecorder` instance from the standard library. You can use the `Code` and `Body` fields to check the response status code and body.

```go title="code.go"
if resp.Code != http.StatusOK {
	t.Fail("Unexpected status code", resp.Code)
}

if !strings.Contains(resp.Body.String(), "some text") {
	t.Fail("Unexpected response body", resp.Body.String())
}
```

Use whatever assertion library you want to make these checks. [`stretchr/testify`](https://github.com/stretchr/testify) is popular and easy to use.

## Dive Deeper

-   Tutorial
    -   [Writing Tests](../tutorial/writing-tests.md)
-   Reference
    -   [`humatest`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/humatest)
-   External Links
    -   [Go testing](https://pkg.go.dev/testing)
