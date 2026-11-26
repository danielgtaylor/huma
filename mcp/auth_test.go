package mcp_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/mcp"
)

const authCallBody = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"admin_thing","arguments":{"name":"n","count":1}}}`

func TestProtectedResourceMetadata(t *testing.T) {
	_, base := humatest.New(t)
	mcp.Attach(base, mcp.Options{
		Name:      "auth",
		Version:   "1.0.0",
		PublicURL: "https://api.example.com",
		ProtectedResource: &mcp.ProtectedResourceMetadata{
			AuthorizationServers: []string{"https://auth.example.com"},
			ScopesSupported:      []string{"mcp"},
			Extensions:           map[string]any{"custom_field": "x"},
		},
	})

	resp := base.Get("/.well-known/oauth-protected-resource/mcp")
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	// resource derived from PublicURL + MCP path.
	if doc["resource"] != "https://api.example.com/mcp" {
		t.Errorf("unexpected resource: %v", doc["resource"])
	}
	if servers, _ := doc["authorization_servers"].([]any); len(servers) != 1 || servers[0] != "https://auth.example.com" {
		t.Errorf("unexpected authorization_servers: %v", doc["authorization_servers"])
	}
	if methods, _ := doc["bearer_methods_supported"].([]any); len(methods) != 1 || methods[0] != "header" {
		t.Errorf("expected default bearer_methods_supported=[header], got %v", doc["bearer_methods_supported"])
	}
	if doc["custom_field"] != "x" {
		t.Errorf("expected merged extension, got %v", doc["custom_field"])
	}
}

func TestNoMetadataWhenUnconfigured(t *testing.T) {
	_, base := humatest.New(t)
	mcp.Attach(base, mcp.Options{Name: "n", Version: "1.0.0"})

	// Without ProtectedResource, the discovery endpoint must not exist.
	resp := base.Get("/.well-known/oauth-protected-resource/mcp")
	if resp.Code == 200 {
		t.Errorf("expected discovery endpoint to be absent, got 200: %s", resp.Body.String())
	}
}

func TestSetChallenge(t *testing.T) {
	_, base := humatest.New(t)

	// Middleware must be registered before Attach so the transport route (whose
	// middleware chain is captured at registration) includes it. The server is
	// forward-declared and captured by the closure, resolved at request time.
	var server *mcp.Server
	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		requiresAuth := false
		for _, scheme := range ctx.Operation().Security {
			if _, ok := scheme["oauth"]; ok {
				requiresAuth = true
			}
		}
		if requiresAuth && ctx.Header("Authorization") == "" {
			server.SetChallenge(ctx)
			_ = huma.WriteErr(base, ctx, http.StatusUnauthorized, "Unauthorized")
			return
		}
		next(ctx)
	})

	_, server = mcp.Attach(base, mcp.Options{
		Name:      "c",
		Version:   "1.0.0",
		PublicURL: "https://api.example.com",
		Security:  []map[string][]string{{"oauth": {"mcp"}}},
		ProtectedResource: &mcp.ProtectedResourceMetadata{
			AuthorizationServers: []string{"https://auth.example.com"},
		},
	})

	resp := base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if resp.Code != 401 {
		t.Fatalf("expected 401 challenge, got %d", resp.Code)
	}
	wantauth := `Bearer resource_metadata="https://api.example.com/.well-known/oauth-protected-resource/mcp"`
	if got := resp.Header().Get("WWW-Authenticate"); got != wantauth {
		t.Errorf("unexpected WWW-Authenticate:\n got %q\nwant %q", got, wantauth)
	}
}

// TestProtectedResourceUnderPrefix checks the discovery URLs built from the
// request include the prefix the API is served under, for any operation.
func TestProtectedResourceUnderPrefix(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.NewWithPrefix(mux, "/api", huma.DefaultConfig("Test", "1.0.0"))

	var server *mcp.Server
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if len(ctx.Operation().Security) > 0 && ctx.Header("Authorization") == "" {
			server.SetChallenge(ctx)
			_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "Unauthorized")
			return
		}
		next(ctx)
	})

	_, server = mcp.Attach(api, mcp.Options{
		Name:     "p",
		Version:  "1.0.0",
		Security: []map[string][]string{{"oauth": {"mcp"}}},
		ProtectedResource: &mcp.ProtectedResourceMetadata{
			AuthorizationServers: []string{"https://auth.example.com"},
		},
	})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)))
	want := `Bearer resource_metadata="http://example.com/api/.well-known/oauth-protected-resource/mcp"`
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("unexpected WWW-Authenticate:\n got %q\nwant %q", got, want)
	}

	huma.Register(api, huma.Operation{
		Method:   http.MethodGet,
		Path:     "/things/{id}",
		Security: []map[string][]string{{"oauth": {"mcp"}}},
	}, func(_ context.Context, _ *struct {
		ID string `path:"id"`
	},
	) (*struct{}, error) {
		return nil, nil
	})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/things/1", nil))
	if got := rec.Header().Get("WWW-Authenticate"); got != want {
		t.Errorf("unexpected WWW-Authenticate on a param route:\n got %q\nwant %q", got, want)
	}

	// A route matching several segments can't be lined up with the path, so it
	// gets no prefix rather than a wrong one.
	huma.Register(api, huma.Operation{
		Method:   http.MethodGet,
		Path:     "/files/{path...}",
		Security: []map[string][]string{{"oauth": {"mcp"}}},
	}, func(_ context.Context, _ *struct {
		Path string `path:"path"`
	},
	) (*struct{}, error) {
		return nil, nil
	})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/files/a/b/c", nil))
	if got, want := rec.Header().Get("WWW-Authenticate"), `Bearer resource_metadata="http://example.com/.well-known/oauth-protected-resource/mcp"`; got != want {
		t.Errorf("unexpected WWW-Authenticate on a wildcard route:\n got %q\nwant %q", got, want)
	}

	// Router-level middleware may have no operation.
	server.SetChallenge(humago.NewContext(nil, httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder()))

	// A path shorter than the route can't be lined up, so it gets no prefix.
	rec = httptest.NewRecorder()
	server.SetChallenge(humago.NewContext(&huma.Operation{Path: "/things/{id}"}, httptest.NewRequest(http.MethodGet, "/", nil), rec))
	if got, want := rec.Header().Get("WWW-Authenticate"), `Bearer resource_metadata="http://example.com/.well-known/oauth-protected-resource/mcp"`; got != want {
		t.Errorf("unexpected WWW-Authenticate for a short path:\n got %q\nwant %q", got, want)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/.well-known/oauth-protected-resource/mcp", nil))
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON (%d): %s", rec.Code, rec.Body.String())
	}
	if doc["resource"] != "http://example.com/api/mcp" {
		t.Errorf("unexpected resource: %v", doc["resource"])
	}
}

// authAPI builds an API whose tools require a bearer token, mirroring
// TestAuthHeaderForwarding but letting the caller opt into the MCP authorization
// surface so auth failures escalate to challenges.
func authAPI(t *testing.T, opts mcp.Options) humatest.TestAPI {
	t.Helper()

	_, base := humatest.New(t)

	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		needsAuth := false
		for _, scheme := range ctx.Operation().Security {
			if _, ok := scheme["apiKey"]; ok {
				needsAuth = true
			}
		}
		if needsAuth && ctx.Header("Authorization") != "Bearer secret" {
			ctx.SetStatus(http.StatusUnauthorized)
			ctx.BodyWriter().Write([]byte(`{"status":401,"detail":"missing api key"}`))
			return
		}
		next(ctx)
	})

	api, _ := mcp.Attach(base, opts)

	huma.Post(api, "/admin/thing", func(_ context.Context, in *createInput) (*createOutput, error) {
		out := &createOutput{}
		out.Body.Name = in.Body.Name
		return out, nil
	}, mcp.Tool(mcp.WithName("admin_thing")), func(o *huma.Operation) {
		o.Security = []map[string][]string{{"apiKey": {"admin"}}}
	})

	return base
}

// With ProtectedResource configured, a tool call rejected by the API's own auth
// middleware must surface as a transport-level 401 carrying the resource metadata
// URL. That pair is what makes a connector go and authenticate; reporting it
// inside a successful JSON-RPC result leaves the client with nothing to act on.
func TestToolAuthFailureEscalatesToChallenge(t *testing.T) {
	api := authAPI(t, mcp.Options{
		Name: "auth", Version: "1.0.0",
		PublicURL:         "https://api.example.com",
		ProtectedResource: &mcp.ProtectedResourceMetadata{AuthorizationServers: []string{"https://auth.example.com"}},
	})

	resp := api.Post("/mcp", strings.NewReader(authCallBody))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", resp.Code, resp.Body.String())
	}

	got := resp.Header().Get("WWW-Authenticate")
	want := `Bearer resource_metadata="https://api.example.com/.well-known/oauth-protected-resource/mcp"`
	if got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}

	// A valid token still reaches the tool.
	resp = api.Post("/mcp", strings.NewReader(authCallBody), "Authorization: Bearer secret")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 with a token, got %d: %s", resp.Code, resp.Body.String())
	}
}

// Without ProtectedResource the server has not opted into the authorization
// surface, so a rejected call stays an ordinary tool error and existing behaviour
// is unchanged.
func TestToolAuthFailureStaysToolErrorWithoutOptIn(t *testing.T) {
	api := authAPI(t, mcp.Options{Name: "auth", Version: "1.0.0"})

	out := rpc(t, api, authCallBody)
	if out["result"].(map[string]any)["isError"] != true {
		t.Fatalf("expected an isError tool result, got %v", out)
	}
}

// A 403 means the caller authenticated but is not permitted. Re-running discovery
// would not help, so it must not be escalated into a challenge.
func TestToolForbiddenIsNotEscalated(t *testing.T) {
	_, base := humatest.New(t)

	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if strings.HasPrefix(ctx.URL().Path, "/admin/") {
			ctx.SetStatus(http.StatusForbidden)
			return
		}
		next(ctx)
	})

	api, _ := mcp.Attach(base, mcp.Options{
		Name: "auth", Version: "1.0.0",
		ProtectedResource: &mcp.ProtectedResourceMetadata{AuthorizationServers: []string{"https://auth.example.com"}},
	})

	huma.Post(api, "/admin/thing", func(_ context.Context, in *createInput) (*createOutput, error) {
		return &createOutput{}, nil
	}, mcp.Tool(mcp.WithName("admin_thing")))

	out := rpc(t, base, authCallBody)
	if out["result"].(map[string]any)["isError"] != true {
		t.Fatalf("expected a 403 to remain a tool error, got %v", out)
	}
}

func TestChallengeFromMiddleware(t *testing.T) {
	type userKey struct{}

	_, base := humatest.New(t)

	api, server := mcp.Attach(base, mcp.Options{
		Name: "rb", Version: "1.0.0",
		PublicURL:         "https://api.example.com",
		ProtectedResource: &mcp.ProtectedResourceMetadata{AuthorizationServers: []string{"https://auth.example.com"}},
	})

	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Header("Authorization") != "Bearer secret" {
			server.SetChallenge(ctx)
			_ = huma.WriteErr(base, ctx, http.StatusUnauthorized, "Unauthorized")
			return
		}
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), userKey{}, "alice")))
	})

	var seen any
	huma.Post(api, "/who", func(ctx context.Context, in *createInput) (*createOutput, error) {
		seen = ctx.Value(userKey{})
		return &createOutput{}, nil
	}, mcp.Tool(mcp.WithName("who")))

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"who","arguments":{"name":"n","count":1}}}`

	// No token: challenged, with the discovery URL.
	resp := base.Post("/mcp", strings.NewReader(body))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a token, got %d", resp.Code)
	}
	if got := resp.Header().Get("WWW-Authenticate"); !strings.Contains(got, "resource_metadata=") {
		t.Errorf("WWW-Authenticate = %q, want a resource_metadata parameter", got)
	}

	// Bad token: also challenged.
	if resp := base.Post("/mcp", strings.NewReader(body), "Authorization: Bearer wrong"); resp.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for a bad token, got %d", resp.Code)
	}

	// Good token: the context the middleware attached reaches the handler.
	if resp := base.Post("/mcp", strings.NewReader(body), "Authorization: Bearer secret"); resp.Code != http.StatusOK {
		t.Fatalf("expected 200 with a good token, got %d: %s", resp.Code, resp.Body.String())
	}
	if seen != "alice" {
		t.Errorf("handler saw %v, want the identity attached by the middleware", seen)
	}
}

func TestResourceMetadataDerivedURL(t *testing.T) {
	_, base := humatest.New(t)
	mcp.Attach(base, mcp.Options{ProtectedResource: &mcp.ProtectedResourceMetadata{
		JWKSURI:               "https://auth.example.com/jwks",
		ResourceDocumentation: "https://docs.example.com",
		ResourceName:          "Example",
		Extensions:            map[string]any{"custom": true},
	}})

	body := base.Get("/.well-known/oauth-protected-resource/mcp", "Host: api.test").Body.String()
	for _, want := range []string{
		`"resource":"http://api.test/mcp"`,
		`"jwks_uri":"https://auth.example.com/jwks"`,
		`"resource_documentation":"https://docs.example.com"`,
		`"resource_name":"Example"`,
		`"custom":true`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %s in %s", want, body)
		}
	}
}

func TestResourceMetadataOverTLS(t *testing.T) {
	mux := http.NewServeMux()
	mcp.Attach(humago.New(mux, huma.DefaultConfig("TLS", "1.0.0")), mcp.Options{ProtectedResource: &mcp.ProtectedResourceMetadata{}})

	req := httptest.NewRequest(http.MethodGet, "https://api.test/.well-known/oauth-protected-resource/mcp", nil)
	req.TLS = &tls.ConnectionState{}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), `"resource":"https://api.test/mcp"`) {
		t.Errorf("expected an https resource, got %s", w.Body.String())
	}
}
