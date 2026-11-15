package mcp

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// ProtectedResourceMetadata is the OAuth 2.0 Protected Resource Metadata
// document (RFC 9728) advertised by an MCP server so that clients such as
// ChatGPT and Claude connectors can discover which authorization server(s) to
// use.
type ProtectedResourceMetadata struct {
	// Resource is the protected resource identifier (a URL). If empty, it is
	// derived per request from the request scheme + host and the MCP path (or
	// [Options.PublicURL] when set).
	Resource string `json:"resource,omitempty"`

	// AuthorizationServers lists the issuer identifier URLs of the authorization
	// servers that can issue tokens for this resource.
	AuthorizationServers []string `json:"authorization_servers,omitempty"`

	// JWKSURI is the URL of the resource server's JSON Web Key Set, if any.
	JWKSURI string `json:"jwks_uri,omitempty"`

	// ScopesSupported lists the OAuth scopes recognized by this resource.
	ScopesSupported []string `json:"scopes_supported,omitempty"`

	// BearerMethodsSupported lists how a bearer token may be presented, e.g.
	// "header". Defaults to {"header"} when served if left empty.
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`

	// ResourceName is a human-readable name for the resource.
	ResourceName string `json:"resource_name,omitempty"`

	// ResourceDocumentation is a URL with human-readable documentation.
	ResourceDocumentation string `json:"resource_documentation,omitempty"`

	// Extensions holds any additional metadata fields to include verbatim,
	// keeping the document open to registry parameters this struct does not name.
	Extensions map[string]any `json:"-"`
}

// marshalWithDefaults renders the metadata to JSON, filling in a per-request
// resource identifier and default bearer methods, and merging Extensions.
func (m *ProtectedResourceMetadata) marshalWithDefaults(resource string) ([]byte, error) {
	cp := *m
	if cp.Resource == "" {
		cp.Resource = resource
	}
	if len(cp.BearerMethodsSupported) == 0 {
		cp.BearerMethodsSupported = []string{"header"}
	}

	b, err := json.Marshal(cp)
	if err != nil || len(m.Extensions) == 0 {
		return b, err
	}

	// Extensions are siblings of the named fields, so merge through a map.
	out := map[string]any{}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	maps.Copy(out, m.Extensions)

	return json.Marshal(out)
}

// handleResourceMetadata serves the configured Protected Resource Metadata.
func (s *Server) handleResourceMetadata(ctx huma.Context) {
	body, err := s.opts.ProtectedResource.marshalWithDefaults(s.absURL(ctx, s.opts.Path))
	if err != nil {
		ctx.SetStatus(http.StatusInternalServerError)
		return
	}
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(http.StatusOK)
	ctx.BodyWriter().Write(body)
}

// challengedKey marks a tool call rejected with 401, so the transport can answer
// with a real challenge instead of a tool error string.
type challengedKey struct{}

// withChallengeFlag returns a context that collects unauthenticated tool calls,
// plus the flag it records into.
func withChallengeFlag(ctx context.Context) (context.Context, *bool) {
	challenged := new(bool)
	return context.WithValue(ctx, challengedKey{}, challenged), challenged
}

// recordChallenge notes an unauthenticated inner request. It is a no-op when no
// flag is present.
//
// Only 401 counts. A 403 means the caller authenticated but lacks authorization,
// which is a real tool error: rediscovery would not help, and connectors do not
// treat it as a reason to authenticate.
func recordChallenge(ctx context.Context, status int) {
	if status != http.StatusUnauthorized {
		return
	}

	if challenged, ok := ctx.Value(challengedKey{}).(*bool); ok {
		*challenged = true
	}
}

// SetChallenge sets an RFC 9728 WWW-Authenticate header pointing at the resource
// metadata document, so connectors can begin their OAuth flow. Call it from your
// auth middleware before writing the 401 as usual:
//
//	server.SetChallenge(ctx)
//	huma.WriteErr(api, ctx, http.StatusUnauthorized, "Unauthorized")
//
// Without [Options.ProtectedResource] the challenge is a bare "Bearer", which
// gives a connector nothing to discover.
func (s *Server) SetChallenge(ctx huma.Context) {
	challenge := "Bearer"
	if s.opts.ProtectedResource != nil {
		challenge += ` resource_metadata="` + s.absURL(ctx, s.opts.ResourceMetadataPath) + `"`
	}

	ctx.SetHeader("WWW-Authenticate", challenge)
}

// absURL builds an absolute URL from [Options.PublicURL], or from the request
// when unset (see [Options.PublicURL] for why that is unsafe in production).
func (s *Server) absURL(ctx huma.Context, path string) string {
	if s.opts.PublicURL != "" {
		return s.opts.PublicURL + path
	}
	scheme := "http"
	if ctx.TLS() != nil {
		scheme = "https"
	}
	return scheme + "://" + ctx.Host() + path
}
