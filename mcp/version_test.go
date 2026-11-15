package mcp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/mcp"
)

func TestInitializeUnsupportedVersion(t *testing.T) {
	api, _ := setup(t)

	// initialize is the handshake-based revisions' entry point, so an unknown
	// version is answered with the newest of those, not the stateless one.
	result := rpc(t, api, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2099-01-01"}}`)["result"].(map[string]any)
	if result["protocolVersion"] != "2025-11-25" {
		t.Errorf("expected 2025-11-25, got %v", result["protocolVersion"])
	}
}

// statelessPost sends a stateless-revision request with the headers Streamable
// HTTP requires, unless overridden, and returns the status and decoded body.
func statelessPost(t *testing.T, api humatest.TestAPI, method string, params string, headers ...any) (int, map[string]any) {
	t.Helper()

	body := `{"jsonrpc":"2.0","id":1,"method":"` + method + `","params":` + params + `}`
	args := append([]any{strings.NewReader(body), "MCP-Protocol-Version: " + mcp.ProtocolVersion, "Mcp-Method: " + method}, headers...)
	resp := api.Post("/mcp", args...)

	var out map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON response: %v\n%s", err, resp.Body.String())
	}
	return resp.Code, out
}

const statelessMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

func TestStatelessServerDiscover(t *testing.T) {
	api, _ := setup(t)

	code, out := statelessPost(t, api, "server/discover", `{`+statelessMeta+`}`)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", code, out)
	}
	result := out["result"].(map[string]any)
	got, _ := json.Marshal(result["supportedVersions"])
	if string(got) != `["2026-07-28","2025-11-25","2025-06-18"]` {
		t.Errorf("unexpected supportedVersions %s", got)
	}
	if result["resultType"] != "complete" || result["cacheScope"] != "public" || result["ttlMs"] == nil {
		t.Errorf("missing result fields: %v", result)
	}
	if result["_meta"].(map[string]any)["io.modelcontextprotocol/serverInfo"].(map[string]any)["name"] != "test-mcp" {
		t.Errorf("missing serverInfo: %v", result["_meta"])
	}
}

func TestStatelessToolsCall(t *testing.T) {
	api, _ := setup(t)

	code, out := statelessPost(t, api, "tools/call", `{"name":"create_thing","arguments":{"name":"x","count":2},`+statelessMeta+`}`, "Mcp-Name: create_thing")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", code, out)
	}
	result := out["result"].(map[string]any)
	if result["resultType"] != "complete" || result["structuredContent"].(map[string]any)["name"] != "x" {
		t.Errorf("unexpected result: %v", result)
	}

	// Mcp-Name may arrive base64-encoded; it is compared once decoded.
	code, _ = statelessPost(t, api, "tools/call", `{"name":"create_thing","arguments":{"name":"x","count":2},`+statelessMeta+`}`, "Mcp-Name: =?base64?Y3JlYXRlX3RoaW5n?=")
	if code != http.StatusOK {
		t.Errorf("expected the encoded Mcp-Name accepted, got %d", code)
	}
}

func TestStatelessToolsListCacheScope(t *testing.T) {
	api, _ := setup(t)

	_, out := statelessPost(t, api, "tools/list", `{`+statelessMeta+`}`)
	if scope := out["result"].(map[string]any)["cacheScope"]; scope != "public" {
		t.Errorf("unfiltered list should be public, got %v", scope)
	}

	_, base := humatest.New(t)
	mcp.Attach(base, mcp.Options{Name: "f", Version: "1", Filter: func(context.Context, *huma.Operation) bool { return true }})
	_, out = statelessPost(t, base, "tools/list", `{`+statelessMeta+`}`)
	if scope := out["result"].(map[string]any)["cacheScope"]; scope != "private" {
		t.Errorf("filtered list should be private, got %v", scope)
	}
}

func TestStatelessRejections(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		params  string
		headers []any
		status  int
		code    float64
	}{
		{"missing meta", "tools/list", `{}`, nil, http.StatusBadRequest, -32602},
		{"missing capabilities", "tools/list", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}`, nil, http.StatusBadRequest, -32602},
		{"missing Mcp-Name", "tools/call", `{"name":"create_thing",` + statelessMeta + `}`, nil, http.StatusBadRequest, -32020},
		{"wrong Mcp-Name", "tools/call", `{"name":"create_thing",` + statelessMeta + `}`, []any{"Mcp-Name: admin_reset"}, http.StatusBadRequest, -32020},
		{"wrong Mcp-Method", "tools/list", `{` + statelessMeta + `}`, []any{"Mcp-Method: tools/call"}, http.StatusBadRequest, -32020},
		{"unsupported version", "tools/list", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2099-01-01","io.modelcontextprotocol/clientCapabilities":{}}}`, []any{"MCP-Protocol-Version: 2099-01-01"}, http.StatusBadRequest, -32022},
		{"ping removed", "ping", `{` + statelessMeta + `}`, nil, http.StatusNotFound, -32601},
		{"mismatched MCP-Protocol-Version", "tools/list", `{"_meta":{"io.modelcontextprotocol/protocolVersion":"2099-01-01","io.modelcontextprotocol/clientCapabilities":{}}}`, nil, http.StatusBadRequest, -32020},
		{"unterminated Mcp-Name sentinel", "tools/call", `{"name":"create_thing",` + statelessMeta + `}`, []any{"Mcp-Name: =?base64?Y3JlYXRlX3RoaW5n"}, http.StatusBadRequest, -32020},
		{"invalid Mcp-Name base64", "tools/call", `{"name":"create_thing",` + statelessMeta + `}`, []any{"Mcp-Name: =?base64?!!!?="}, http.StatusBadRequest, -32020},
		{"retired handshake version", "tools/list", `{}`, []any{"MCP-Protocol-Version: 2024-11-05"}, http.StatusBadRequest, -32022},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api, _ := setup(t)

			code, out := statelessPost(t, api, tc.method, tc.params, tc.headers...)
			if code != tc.status {
				t.Errorf("expected HTTP %d, got %d", tc.status, code)
			}
			errObj, ok := out["error"].(map[string]any)
			if !ok || errObj["code"].(float64) != tc.code {
				t.Fatalf("expected error %v, got %v", tc.code, out)
			}
			if tc.code == -32022 {
				got, _ := json.Marshal(errObj["data"].(map[string]any)["supported"])
				if string(got) != `["2026-07-28","2025-11-25","2025-06-18"]` {
					t.Errorf("expected the supported versions listed, got %s", got)
				}
			}
		})
	}
}
