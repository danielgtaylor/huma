package mcp_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/mcp"
)

func ExampleAttach() {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Greeting API", "1.0.0"))

	// Attach returns a drop-in API to register against; reassign so all
	// subsequent registrations flow through it (this is what lets Hidden
	// operations be exposed as tools). It also serves MCP over Streamable HTTP
	// at /mcp alongside the normal REST API.
	api, _ = mcp.Attach(api, mcp.Options{Name: "greeting", Version: "1.0.0"})

	type input struct {
		Name string `path:"name"`
	}
	type output struct {
		Body struct {
			Message string `json:"message"`
		}
	}

	// Opt this operation in as an MCP tool with mcp.Tool().
	huma.Get(api, "/greeting/{name}", func(_ context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.Message = "Hello, " + in.Name
		return out, nil
	}, mcp.Tool())

	// http.ListenAndServe(":8888", mux)
}

func ExampleServer_SetChallenge() {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Greeting API", "1.0.0"))

	const resource = "https://api.example.com/mcp"

	api, server := mcp.Attach(api, mcp.Options{
		Name:    "greeting",
		Version: "1.0.0",

		// Set in production: the resource identifier is what clients bind token
		// audience to, so it must not be derived from a client-supplied Host.
		PublicURL: "https://api.example.com",

		// Advertise where tokens come from, so connectors can run the OAuth flow
		// themselves. Serving that flow is the authorization server's job, not this
		// package's.
		ProtectedResource: &mcp.ProtectedResourceMetadata{
			AuthorizationServers: []string{"https://auth.example.com"},
			ScopesSupported:      []string{"mcp"},
		},

		// Required if you add "Cookie" to ForwardHeaders, and good practice anyway.
		AllowedOrigins: []string{"https://app.example.com"},
	})

	// Your own auth middleware. A 401 with the challenge lets connectors discover
	// the authorization server and sign in.
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		claims, err := verifyToken(ctx.Context(), strings.TrimPrefix(ctx.Header("Authorization"), "Bearer "))
		// Reject tokens minted for anything other than this resource, otherwise a
		// token issued for another service could drive these tools.
		if err != nil || !slices.Contains(claims.Audience, resource) {
			server.SetChallenge(ctx)
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Unauthorized")
			return
		}

		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), userIDKey{}, claims.Subject)))
	})

	type input struct {
		Name string `path:"name"`
	}
	type output struct {
		Body struct {
			Message string `json:"message"`
		}
	}

	huma.Get(api, "/greeting/{name}", func(ctx context.Context, in *input) (*output, error) {
		// ctx carries whatever the validate function attached.
		out := &output{}
		out.Body.Message = "Hello, " + in.Name
		return out, nil
	}, mcp.Tool())

	// http.ListenAndServe(":8888", mux)
}

type userIDKey struct{}

type tokenClaims struct {
	Audience []string
	Subject  string
}

// verifyToken stands in for whatever verifies your tokens, e.g. a JWKS-backed JWT
// parser or an introspection call.
func verifyToken(_ context.Context, token string) (*tokenClaims, error) {
	if token == "" {
		return nil, errors.New("empty token")
	}

	return &tokenClaims{Subject: "user-1", Audience: []string{"https://api.example.com/mcp"}}, nil
}
