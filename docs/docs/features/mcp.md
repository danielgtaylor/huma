---
description: Expose operations as Model Context Protocol (MCP) tools for LLM clients.
---

# Model Context Protocol (MCP)

## MCP { .hidden }

The [`mcp`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/mcp) package exposes your operations as [Model Context Protocol](https://modelcontextprotocol.io/) tools, so LLM clients like Claude or ChatGPT can call them. Each tool call is sent back through your API's router as a normal request, so validation, middleware, and auth all apply exactly as they do for any other client.

```go title="code.go"
import "github.com/danielgtaylor/huma/v2/mcp"

// ...

// Attach before registering operations, and register against the returned API.
api, _ = mcp.Attach(api, mcp.Options{})

// Opt an operation in as a tool.
huma.Get(api, "/greeting/{name}", greet, mcp.Tool())
```

MCP is now served at `POST /mcp`. Tool arguments are generated from the operation's parameters and request body, and the tool name defaults to the operation ID.

!!! info "Hidden Operations"

    Operations registered against the returned API can be exposed even when `Hidden`, which keeps them out of the OpenAPI document. This is why `Attach` should be called first.

## Choosing Tools

Nothing is exposed unless you opt in. Use `mcp.Tool()` with `huma.Get`, `huma.Post`, etc., or `mcp.WithTool(op)` with `huma.Register`. Both accept `mcp.WithName` and `mcp.WithDescription` to override the defaults.

To expose everything instead, set `ExposeVisible` and/or `ExposeHidden` in the options, and opt individual operations out with `mcp.Exclude()`, or `mcp.WithExclude(op)` with `huma.Register`. Keep in mind that any operation added later becomes a tool too. Streaming (SSE) operations and ones whose request body is not JSON are skipped, since they can't work as tool calls.

## Authentication

The `Authorization` header from the MCP request is forwarded onto each tool call, so your existing auth middleware already protects every tool. What the package adds is discovery, so a client can find out where to sign in:

-   `ProtectedResource` serves [OAuth 2.0 Protected Resource Metadata](https://www.rfc-editor.org/rfc/rfc9728) naming your authorization server.
-   `server.SetChallenge(ctx)` sets a `WWW-Authenticate` header pointing at it. Call it from your middleware before writing a `401`.

Huma never validates or issues tokens; that stays with your middleware and authorization server, as described in [OAuth 2.0 & JWT](../how-to/oauth2-jwt.md).

{% raw %}
```go title="code.go" hl_lines="2-7 17"
api, server := mcp.Attach(api, mcp.Options{
	PublicURL: "https://api.example.com",
	ProtectedResource: &mcp.ProtectedResourceMetadata{
		AuthorizationServers: []string{"https://auth.example.com"},
	},
	// Require auth on the MCP endpoint itself.
	Security: []map[string][]string{{"bearer": {}}},
})

api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
	if len(ctx.Operation().Security) == 0 {
		next(ctx)
		return
	}

	if !validToken(ctx.Header("Authorization")) {
		server.SetChallenge(ctx)
		huma.WriteErr(api, ctx, http.StatusUnauthorized, "Unauthorized")
		return
	}

	next(ctx)
})
```
{% endraw %}

!!! warning "Token Audience"

    Your middleware **must** reject tokens that were not issued for this API's MCP endpoint (`PublicURL` plus the MCP path), otherwise a token minted for another service could be used to call your tools. Set `PublicURL` in production, as the default derives it from the client-controlled `Host` header.

Use `Filter` to hide tools a caller can't use, e.g. admin tools for regular users. It only affects which tools are offered; each call still runs through the operation's own middleware.

See the [mcp-auth example](https://github.com/danielgtaylor/huma/tree/main/examples/mcp-auth) for a full runnable version.

## Dive Deeper

-   Reference
    -   [`mcp.Attach`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/mcp#Attach)
    -   [`mcp.Options`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/mcp#Options)
    -   [`mcp.Tool`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/mcp#Tool)
    -   [`mcp.Server.SetChallenge`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/mcp#Server.SetChallenge)
-   External Links
    -   [Model Context Protocol](https://modelcontextprotocol.io/)
    -   [MCP Authorization](https://modelcontextprotocol.io/specification/latest/basic/authorization)
