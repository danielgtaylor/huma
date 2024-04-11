---
description: Use client-driven content-negotiation with default and custom formats to serialize response data.
---

# Serialization

When handler functions return Go objects, they will be serialized to bytes for transmission back to the client.

### Default Formats

The [`config.Formats`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) maps either a content type name or extension (suffix) to a `huma.Format` instance.

The default configuration for Huma includes support for JSON ([RFC 8259](https://tools.ietf.org/html/rfc8259)) and optionally CBOR ([RFC 7049](https://tools.ietf.org/html/rfc7049)) content types via the `Accept` header. This is done by registering the following content types using [`huma.DefaultJSONFormat`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultJSONFormat):

-   `application/json`
-   Anything ending with `+json`

CBOR support can be enabled by importing the `cbor` package, which adds [`cbor.DefaultCBORFormat`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/formats/cbor#DefaultCBORFormat) to the default list of formats:

```go title="main.go"
import (
    "github.com/danielgtaylor/huma/v2"

    _ "github.com/danielgtaylor/huma/v2/formats/cbor"
)
```

This adds the following content types:

-   `application/cbor`
-   Anything ending with `+cbor`

!!! info "Other Formats"

    You can easily add support for additional serialization formats, including binary formats like [Protobuf](https://protobuf.dev/) if desired.

## Custom Formats

Huma supports custom serialization formats by implementing the [`huma.Format`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Format) interface. Serialization formats are set on the API configuration at API creation time and selected by client-driven [content negotiation](#content-negotiation).

Writing a new format can be very simple, by just providing a marshal and unmarshal function:

```go title="code.go"
var DefaultJSONFormat = Format{
	Marshal: func(w io.Writer, v any) error {
		return json.NewEncoder(w).Encode(v)
	},
	Unmarshal: json.Unmarshal,
}
```

## Content Negotiation

Content negotiation allows clients to select the content type they are most comfortable working with when talking to the API. For request bodies, this uses the `Content-Type` header. For response bodies, it uses the `Accept` header. If none are present then JSON is usually selected as the default / preferred content type.

```sh title="Terminal"
# Send YAML as input using Restish
$ echo 'foo: bar' | \
	restish put api.rest.sh -H 'Content-Type:application/yaml'

# Get CBOR output from an API
$ restish api.rest.sh -H 'Accept:application/cbor'
HTTP/2.0 200 OK
Content-Length: 318
Content-Type: application/cbor
Etag: O7fTqWETqWI
...
```

See the [`negotiation`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/negotiation) package for more info.

## Dive Deeper

-   Reference
    -   [`huma.Config`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Config) the API config
    -   [`huma.DefaultConfig`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#DefaultConfig) the default API config
    -   [`huma.Format`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Format) to marshal/unmarshal data
-   External Links
    -   [RFC 8259](https://tools.ietf.org/html/rfc8259) JSON
    -   [RFC 7049](https://tools.ietf.org/html/rfc7049) CBOR
