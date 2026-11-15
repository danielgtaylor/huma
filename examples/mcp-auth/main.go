// This example shows how to serve an authenticated MCP server, so a connector
// like ChatGPT or Claude can discover how to authenticate and then call your
// operations as tools.
//
// Huma serves the discovery metadata and challenges unauthenticated requests.
// Issuing tokens is your authorization server's job, and verifying them is your
// middleware's: see verifyToken below.
//
//	# Discovery, which must work before the client has a token.
//	curl localhost:8888/.well-known/oauth-protected-resource/mcp
//
//	# No token: 401 pointing at that document.
//	curl -i -X POST localhost:8888/mcp -d '{"jsonrpc":"2.0","id":1,"method":"ping"}'
//
//	# List and call tools.
//	curl -X POST localhost:8888/mcp -H 'Authorization: Bearer demo-token' \
//		-d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
//
//	curl -X POST localhost:8888/mcp -H 'Authorization: Bearer demo-token' \
//		-d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"greet","arguments":{"name":"world"}}}'
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/danielgtaylor/huma/v2/mcp"
	"github.com/go-chi/chi/v5"
)

// Options for the CLI.
type Options struct {
	AuthServerURL string `doc:"Issuer URL of the authorization server" default:"http://localhost:9000"`

	Port int `doc:"Port to listen on" default:"8888"`

	// Set this in production. The resource identifier built from it is what clients
	// bind token audience to, so it must not come from the request's Host header.
	PublicURL string `doc:"Public base URL of this API" default:"http://localhost:8888"`
}

type userKey struct{}

type claims struct {
	Audience []string
	Subject  string
}

// verifyToken stands in for your own verification: a JWKS-backed JWT parse, an
// introspection call, or a database lookup. It must report the audience.
func verifyToken(_ context.Context, token string) (*claims, error) {
	if token != "demo-token" {
		return nil, errors.New("unknown token")
	}

	return &claims{Subject: "user-1", Audience: []string{"http://localhost:8888/mcp"}}, nil
}

type GreetingInput struct {
	Name string `path:"name" maxLength:"30" example:"world"`
}

type GreetingOutput struct {
	Body struct {
		For     string `json:"for" doc:"Authenticated user the greeting was for"`
		Message string `json:"message" doc:"Greeting message"`
	}
}

func main() {
	cli := humacli.New(func(hooks humacli.Hooks, options *Options) {
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("Greeting API", "1.0.0"))

		resource := options.PublicURL + "/mcp"

		// Register operations against the returned API, or Hidden ones will not
		// become tools.
		api, server := mcp.Attach(api, mcp.Options{
			Name:      "greeting",
			Version:   "1.0.0",
			PublicURL: options.PublicURL,
			ProtectedResource: &mcp.ProtectedResourceMetadata{
				AuthorizationServers: []string{options.AuthServerURL},
				ScopesSupported:      []string{"mcp"},
				ResourceName:         "Greeting API",
			},
			Security:       []map[string][]string{{"bearer": {"mcp"}}},
			AllowedOrigins: []string{options.PublicURL},
			ExposeVisible:  true,
		})

		// Authenticates the MCP transport and every operation with a Security
		// requirement. The challenge tells connectors where to sign in.
		api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
			if len(ctx.Operation().Security) == 0 {
				next(ctx)
				return
			}

			c, err := verifyToken(ctx.Context(), strings.TrimPrefix(ctx.Header("Authorization"), "Bearer "))
			// Required: a token issued for another service must not drive these
			// tools. Huma cannot check this, as it never inspects tokens.
			if err != nil || !slices.Contains(c.Audience, resource) {
				server.SetChallenge(ctx)
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Unauthorized")
				return
			}

			next(huma.WithContext(ctx, context.WithValue(ctx.Context(), userKey{}, c.Subject)))
		})

		huma.Register(api, mcp.WithTool(huma.Operation{
			OperationID: "get-greeting",
			Summary:     "Greet a user by name",
			Method:      http.MethodGet,
			Path:        "/greeting/{name}",
			Security:    []map[string][]string{{"bearer": {"mcp"}}},
		}, mcp.WithName("greet")), func(ctx context.Context, in *GreetingInput) (*GreetingOutput, error) {
			out := &GreetingOutput{}
			out.Body.Message = "Hello, " + in.Name
			out.Body.For, _ = ctx.Value(userKey{}).(string)

			return out, nil
		})

		// Hidden keeps an operation out of the OpenAPI document. Marking it with
		// WithTool still exposes it as a tool.
		huma.Register(api, mcp.WithTool(huma.Operation{
			OperationID: "reset",
			Summary:     "Reset the service",
			Method:      http.MethodPost,
			Path:        "/admin/reset",
			Security:    []map[string][]string{{"bearer": {"mcp"}}},
			Hidden:      true,
		}, mcp.WithName("admin_reset")), func(ctx context.Context, _ *struct{}) (*GreetingOutput, error) {
			out := &GreetingOutput{}
			out.Body.Message = "reset"
			out.Body.For, _ = ctx.Value(userKey{}).(string)

			return out, nil
		})

		s := http.Server{
			Addr:    fmt.Sprintf(":%d", options.Port),
			Handler: router,
		}

		hooks.OnStart(func() {
			fmt.Printf("MCP on %s/mcp, try Bearer demo-token\n", options.PublicURL)
			s.ListenAndServe()
		})

		hooks.OnStop(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.Shutdown(ctx)
		})
	})

	cli.Run()
}
