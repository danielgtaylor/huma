---
description: Add hints for command-line clients like Restish to automatically configure themselves to talk to your API.
---

# CLI AutoConfig

Huma includes built-in support for an OpenAPI 3 extension that enables CLI auto-configuration. This allows tools like [Restish](https://rest.sh/) to automatically configure themselves to talk to your API with the correct endpoints, authentication mechanism, etc without the user needing to know anything about your API.

```go
o := api.OpenAPI()
o.Components.SecuritySchemes["my-scheme"] = &huma.SecurityScheme{
	Type: "oauth2",
	// ... security scheme definition ...
}
o.Extensions["x-cli-autoconfig"] = huma.AutoConfig{
	Security: "my-scheme",
	Params: map[string]string{
		"client_id": "abc123",
		"authorize_url": "https://example.tld/authorize",
		"token_url": "https://example.tld/token",
		"scopes": "read,write",
	}
}
```

See the [CLI AutoConfiguration](https://rest.sh/#/openapi?id=autoconfiguration) documentation for more info, including how to ask the user for custom parameters.
