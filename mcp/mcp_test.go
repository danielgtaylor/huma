package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/mcp"
	"github.com/danielgtaylor/huma/v2/sse"
)

type greetingInput struct {
	Name     string `path:"name"`
	Greeting string `query:"greeting"`
}

type greetingOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

type createInput struct {
	Body struct {
		Name  string `json:"name"`
		Count int    `json:"count" minimum:"1"`
	}
}

type createOutput struct {
	Body struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
}

func setup(t *testing.T) (humatest.TestAPI, *mcp.Server) {
	t.Helper()
	_, base := humatest.New(t)

	api, server := mcp.Attach(base, mcp.Options{Name: "test-mcp", Version: "0.1.0"})

	huma.Get(api, "/greeting/{name}", func(_ context.Context, in *greetingInput) (*greetingOutput, error) {
		out := &greetingOutput{}
		greeting := in.Greeting
		if greeting == "" {
			greeting = "Hello"
		}
		out.Body.Message = greeting + ", " + in.Name
		return out, nil
	}, mcp.Tool())

	huma.Post(api, "/things", func(_ context.Context, in *createInput) (*createOutput, error) {
		out := &createOutput{}
		out.Body.ID = "thing-1"
		out.Body.Name = in.Body.Name
		out.Body.Count = in.Body.Count
		return out, nil
	}, mcp.Tool(mcp.WithName("create_thing")))

	// Hidden operation: absent from the OpenAPI document, still exposed as an
	// MCP tool because it opts in via mcp.Tool().
	huma.Post(api, "/admin/reset", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "reset complete"
		return out, nil
	}, mcp.Tool(mcp.WithName("admin_reset")), func(o *huma.Operation) {
		o.Hidden = true
	})

	// Not opted in: must not be exposed as a tool.
	huma.Get(api, "/secret", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "classified"
		return out, nil
	})

	return base, server
}

// rpc sends a single JSON-RPC message over the Streamable HTTP transport and
// returns the decoded response.
func rpc(t *testing.T, api humatest.TestAPI, body string) map[string]any {
	t.Helper()
	resp := api.Post("/mcp", strings.NewReader(body))
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON response: %v\n%s", err, resp.Body.String())
	}
	return out
}

func TestInitialize(t *testing.T) {
	api, _ := setup(t)
	out := rpc(t, api, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)

	result, ok := out["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", out)
	}
	info := result["serverInfo"].(map[string]any)
	if info["name"] != "test-mcp" || info["version"] != "0.1.0" {
		t.Errorf("unexpected serverInfo: %v", info)
	}
	if result["protocolVersion"] != "2025-06-18" {
		t.Errorf("unexpected protocolVersion: %v", result["protocolVersion"])
	}
}

func TestToolsList(t *testing.T) {
	api, _ := setup(t)
	out := rpc(t, api, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	result := out["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d: %v", len(tools), tools)
	}

	names := map[string]map[string]any{}
	for _, tRaw := range tools {
		tm := tRaw.(map[string]any)
		names[tm["name"].(string)] = tm
	}

	if _, ok := names["create_thing"]; !ok {
		t.Errorf("missing create_thing tool: %v", names)
	}

	// The greeting tool: path + query params become top-level properties.
	greeting, ok := names["get-greeting"]
	if !ok {
		// Operation ID may differ; find the greeting tool by its schema.
		for n, tm := range names {
			schema := tm["inputSchema"].(map[string]any)
			props := schema["properties"].(map[string]any)
			if _, ok := props["name"]; ok {
				if _, ok := props["greeting"]; ok {
					greeting = tm
					t.Logf("greeting tool is named %q", n)
				}
			}
		}
	}
	if greeting == nil {
		t.Fatalf("could not find greeting tool: %v", names)
	}

	schema := greeting["inputSchema"].(map[string]any)
	props := schema["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Errorf("greeting tool missing 'name' property: %v", props)
	}
	if _, ok := props["greeting"]; !ok {
		t.Errorf("greeting tool missing 'greeting' property: %v", props)
	}
	required := schema["required"].([]any)
	if len(required) != 1 || required[0] != "name" {
		t.Errorf("expected required=[name], got %v", required)
	}

	// The create tool body should be flattened into name/count.
	createSchema := names["create_thing"]["inputSchema"].(map[string]any)
	createProps := createSchema["properties"].(map[string]any)
	if _, ok := createProps["name"]; !ok {
		t.Errorf("create tool missing flattened 'name': %v", createProps)
	}
	if _, ok := createProps["count"]; !ok {
		t.Errorf("create tool missing flattened 'count': %v", createProps)
	}
}

func TestHiddenOperationExposedAsTool(t *testing.T) {
	base, _ := setup(t)

	// The hidden operation must not appear in the OpenAPI document...
	if item := base.OpenAPI().Paths["/admin/reset"]; item != nil {
		t.Errorf("hidden operation leaked into OpenAPI paths: %v", item)
	}

	// ...but it must be listed and callable as an MCP tool.
	list := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	found := false
	for _, tRaw := range list["result"].(map[string]any)["tools"].([]any) {
		if tRaw.(map[string]any)["name"] == "admin_reset" {
			found = true
		}
	}
	if !found {
		t.Fatal("hidden tool admin_reset not listed")
	}

	out := rpc(t, base, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"admin_reset","arguments":{}}}`)
	result := out["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("unexpected error calling hidden tool: %v", result)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "reset complete") {
		t.Errorf("unexpected hidden tool output: %s", text)
	}
}

func listToolNames(t *testing.T, base humatest.TestAPI) map[string]bool {
	t.Helper()
	out := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	names := map[string]bool{}
	for _, tRaw := range out["result"].(map[string]any)["tools"].([]any) {
		names[tRaw.(map[string]any)["name"].(string)] = true
	}
	return names
}

func TestExposeVisible(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "all", Version: "1.0.0", ExposeVisible: true})

	// No mcp.Tool() marker: exposed anyway in expose-visible mode.
	huma.Get(api, "/greeting/{name}", func(_ context.Context, in *greetingInput) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "hi " + in.Name
		return out, nil
	})

	// Explicitly excluded despite expose-visible mode.
	huma.Get(api, "/secret", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Exclude())

	// Hidden operation: NOT exposed, since only ExposeVisible is set.
	huma.Post(api, "/admin/reset", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, func(o *huma.Operation) { o.Hidden = true })

	// Still customizable via Tool() in expose-visible mode.
	huma.Post(api, "/things", func(_ context.Context, in *createInput) (*createOutput, error) {
		out := &createOutput{}
		out.Body.Name = in.Body.Name
		return out, nil
	}, mcp.Tool(mcp.WithName("make_thing")))

	// A stream never completes and a non-JSON body cannot be sent, so neither
	// is exposed, even when marked with Tool().
	sse.Register(api, huma.Operation{OperationID: "stream", Method: http.MethodGet, Path: "/stream"},
		map[string]any{"message": greetingOutput{}.Body}, func(_ context.Context, _ *struct{}, _ sse.Sender) {})
	sse.Register(api, mcp.WithTool(huma.Operation{OperationID: "marked-stream", Method: http.MethodGet, Path: "/marked-stream"}),
		map[string]any{"message": greetingOutput{}.Body}, func(_ context.Context, _ *struct{}, _ sse.Sender) {})
	huma.Put(api, "/upload", func(_ context.Context, _ *struct {
		RawBody []byte `contentType:"application/octet-stream"`
	},
	) (*struct{}, error) {
		return nil, nil
	})

	names := listToolNames(t, base)
	if !names["make_thing"] {
		t.Errorf("expected custom-named tool make_thing, got %v", names)
	}
	if len(names) != 2 {
		// greeting (auto) + make_thing (custom); secret excluded, hidden, stream
		// and upload not exposed.
		t.Errorf("expected 2 tools, got %d: %v", len(names), names)
	}
	for n := range names {
		if strings.Contains(n, "secret") {
			t.Errorf("excluded operation was exposed: %v", names)
		}
	}
}

func TestExposeVisibleDoesNotCoverHidden(t *testing.T) {
	_, base := humatest.New(t)
	// The two flags are independent axes: ExposeVisible alone must not reach Hidden
	// operations, which is what makes ExposeHidden a separate decision.
	api, _ := mcp.Attach(base, mcp.Options{Name: "h", Version: "1.0.0", ExposeVisible: true})

	huma.Post(api, "/admin/reset", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "ok"
		return out, nil
	}, func(o *huma.Operation) { o.Hidden = true })

	huma.Get(api, "/greeting/{name}", func(_ context.Context, _ *greetingInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	})

	names := listToolNames(t, base)
	if len(names) != 1 {
		t.Fatalf("expected only the visible op, got %d: %v", len(names), names)
	}
	for name := range names {
		if strings.Contains(name, "reset") {
			t.Errorf("hidden operation was exposed in bulk: %v", names)
		}
	}
}

// ExposeHidden bulk-exposes Hidden operations, the counterpart to ExposeVisible.
func TestExposeHidden(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "h", Version: "1.0.0", ExposeHidden: true})

	huma.Post(api, "/admin/reset", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "ok"
		return out, nil
	}, func(o *huma.Operation) { o.Hidden = true })

	// Visible operations are not covered, since ExposeVisible is false.
	huma.Get(api, "/greeting/{name}", func(_ context.Context, _ *greetingInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	})

	names := listToolNames(t, base)
	if len(names) != 1 {
		t.Fatalf("expected exactly 1 tool (the hidden op), got %d: %v", len(names), names)
	}
}

// Marking a Hidden operation with Tool exposes it without needing ExposeHidden,
// which is the per-operation alternative to the bulk flag.
func TestHiddenExposedWhenMarked(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "h", Version: "1.0.0"})

	huma.Post(api, "/admin/reset", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "ok"
		return out, nil
	}, mcp.Tool(mcp.WithName("admin_reset")), func(o *huma.Operation) { o.Hidden = true })

	if names := listToolNames(t, base); !names["admin_reset"] {
		t.Fatalf("expected a marked hidden op to be exposed, got %v", names)
	}
}

// A documented operation registered against the original API is still found
// through the OpenAPI document.
func TestToolsRecoveredWhenRegisteredOnOriginalAPI(t *testing.T) {
	_, base := humatest.New(t)

	// Note the deliberately discarded first return value.
	mcp.Attach(base, mcp.Options{Name: "orig", Version: "1.0.0"})

	huma.Get(base, "/greeting/{name}", func(_ context.Context, in *greetingInput) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "Hello, " + in.Name
		return out, nil
	}, mcp.Tool(mcp.WithName("greet")))

	if names := listToolNames(t, base); !names["greet"] {
		t.Fatalf("expected greet to be recovered from the OpenAPI document, got %v", names)
	}

	out := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"greet","arguments":{"name":"n"}}}`)
	if out["result"].(map[string]any)["isError"] == true {
		t.Fatalf("recovered tool call failed: %v", out)
	}
}

func TestWithToolRegister(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "reg", Version: "1.0.0"})

	// The huma.Register path: mark the operation with mcp.WithTool.
	huma.Register(api, mcp.WithTool(huma.Operation{
		OperationID: "create-ticket",
		Summary:     "Create a ticket",
		Method:      http.MethodPost,
		Path:        "/tickets",
	}, mcp.WithName("create_ticket")), func(_ context.Context, in *createInput) (*createOutput, error) {
		out := &createOutput{}
		out.Body.Name = in.Body.Name
		return out, nil
	})

	names := listToolNames(t, base)
	if !names["create_ticket"] {
		t.Fatalf("expected create_ticket tool from Register path, got %v", names)
	}

	out := rpc(t, base, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_ticket","arguments":{"name":"n","count":1}}}`)
	if out["result"].(map[string]any)["isError"] == true {
		t.Fatalf("create_ticket call errored: %v", out)
	}
}

func TestFilter(t *testing.T) {
	type roleKey struct{}

	_, base := humatest.New(t)

	// Auth middleware attaches the caller's role, as a real one would its user.
	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), roleKey{}, ctx.Header("X-Role"))))
	})

	// Offer a tool only to the roles its security requirement names.
	api, _ := mcp.Attach(base, mcp.Options{Name: "filter", Version: "1.0.0", Filter: func(ctx context.Context, op *huma.Operation) bool {
		role, _ := ctx.Value(roleKey{}).(string)
		for _, scheme := range op.Security {
			return slices.Contains(scheme["apiKey"], role)
		}
		return true
	}})

	huma.Get(api, "/public", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("public")))

	huma.Get(api, "/admin", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("admin")), func(o *huma.Operation) {
		o.Hidden = true
		o.Security = []map[string][]string{{"apiKey": {"admin"}}}
	})

	send := func(role string, body string) map[string]any {
		t.Helper()
		resp := base.Post("/mcp", strings.NewReader(body), "X-Role: "+role)
		var out map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
			t.Fatalf("invalid JSON response: %v\n%s", err, resp.Body.String())
		}
		return out
	}

	names := func(role string) []string {
		t.Helper()
		tools := send(role, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any)
		out := make([]string, 0, len(tools))
		for _, tool := range tools {
			out = append(out, tool.(map[string]any)["name"].(string))
		}
		slices.Sort(out)
		return out
	}

	if got := names("member"); !slices.Equal(got, []string{"public"}) {
		t.Errorf("member sees %v, want [public]", got)
	}
	if got := names("admin"); !slices.Equal(got, []string{"admin", "public"}) {
		t.Errorf("admin sees %v, want [admin public]", got)
	}

	// A filtered tool is unknown to the caller, not merely unlisted.
	call := send("member", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"admin","arguments":{}}}`)
	if errObj, ok := call["error"].(map[string]any); !ok || !strings.Contains(errObj["message"].(string), "unknown tool") {
		t.Errorf("member calling admin tool got %v, want unknown tool", call)
	}
}

func TestTransportStatusVisibleToMiddleware(t *testing.T) {
	_, base := humatest.New(t)

	// A request log reads the status after the handler, as AIDE's and MANRS's do.
	var status int
	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(ctx)
		if ctx.URL().Path == "/mcp" {
			status = ctx.Status()
		}
	})

	api, _ := mcp.Attach(base, mcp.Options{Name: "status", Version: "1.0.0"})
	huma.Get(api, "/greeting/{name}", func(_ context.Context, in *greetingInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool())

	rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	if status != http.StatusOK {
		t.Errorf("middleware saw status %d, want 200", status)
	}
}

func TestGroupComposition(t *testing.T) {
	_, base := humatest.New(t)
	api, server := mcp.Attach(base, mcp.Options{Name: "grp", Version: "1.0.0"})

	// Register through a group created from the wrapped API; the prefix should
	// apply and the operation should still be captured as a tool.
	grp := huma.NewGroup(api, "/v1")
	huma.Post(grp, "/things", func(_ context.Context, in *createInput) (*createOutput, error) {
		out := &createOutput{}
		out.Body.Name = in.Body.Name
		return out, nil
	}, mcp.Tool(mcp.WithName("grp_create")))
	huma.Get(grp, "", func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("grp_root")))
	_ = server

	if names := listToolNames(t, base); !names["grp_root"] {
		t.Errorf("expected the group's empty-path route as a tool, got %v", names)
	}

	out := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"grp_create","arguments":{"name":"boxed","count":1}}}`)
	result := out["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("group tool call errored: %v", result)
	}
	if result["structuredContent"].(map[string]any)["name"] != "boxed" {
		t.Errorf("unexpected group tool output: %v", result)
	}

	// The prefixed route must exist in the document.
	if base.OpenAPI().Paths["/v1/things"] == nil {
		t.Error("expected prefixed path /v1/things in OpenAPI")
	}
}

func TestUnknownTool(t *testing.T) {
	api, _ := setup(t)
	out := rpc(t, api, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	if _, ok := out["error"]; !ok {
		t.Fatalf("expected JSON-RPC error, got %v", out)
	}
}

func TestUnknownMethod(t *testing.T) {
	api, _ := setup(t)
	out := rpc(t, api, `{"jsonrpc":"2.0","id":6,"method":"bogus"}`)
	errObj, ok := out["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error, got %v", out)
	}
	if errObj["code"].(float64) != -32601 {
		t.Errorf("expected method-not-found code, got %v", errObj["code"])
	}
}

func TestNotificationNoResponse(t *testing.T) {
	api, _ := setup(t)
	resp := api.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	if resp.Code != 202 {
		t.Fatalf("expected 202 for notification, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestGetReturns405(t *testing.T) {
	api, _ := setup(t)
	resp := api.Get("/mcp")
	if resp.Code != 405 {
		t.Errorf("expected 405 for GET, got %d", resp.Code)
	}
}

// Middleware registered after Attach must still apply to the transport, otherwise
// Options.Security would declare protection that nothing enforces.
func TestMiddlewareAddedAfterAttachApplies(t *testing.T) {
	_, base := humatest.New(t)

	api, _ := mcp.Attach(base, mcp.Options{Name: "late", Version: "1.0.0"})

	// Registered only after Attach has already wired the transport route.
	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Header("Authorization") != "Bearer secret" {
			ctx.SetStatus(http.StatusUnauthorized)
			return
		}
		next(ctx)
	})

	huma.Post(api, "/thing", func(_ context.Context, in *createInput) (*createOutput, error) {
		return &createOutput{}, nil
	}, mcp.Tool(mcp.WithName("thing")))

	if resp := base.Post("/mcp", strings.NewReader(authCallBody)); resp.Code != http.StatusUnauthorized {
		t.Fatalf("middleware added after Attach did not apply: got %d", resp.Code)
	}

	resp := base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), "Authorization: Bearer secret")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 with a token, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestOriginValidation(t *testing.T) {
	ping := `{"jsonrpc":"2.0","id":1,"method":"ping"}`

	t.Run("RejectsDisallowedOrigin", func(t *testing.T) {
		_, base := humatest.New(t)
		mcp.Attach(base, mcp.Options{Name: "o", Version: "1.0.0", AllowedOrigins: []string{"https://app.example.com"}})

		if resp := base.Post("/mcp", strings.NewReader(ping), "Origin: https://evil.example.com"); resp.Code != http.StatusForbidden {
			t.Errorf("expected 403 for a disallowed Origin, got %d", resp.Code)
		}
	})

	t.Run("AllowsListedOrigin", func(t *testing.T) {
		_, base := humatest.New(t)
		mcp.Attach(base, mcp.Options{Name: "o", Version: "1.0.0", AllowedOrigins: []string{"https://app.example.com"}})

		if resp := base.Post("/mcp", strings.NewReader(ping), "Origin: https://app.example.com"); resp.Code != http.StatusOK {
			t.Errorf("expected 200 for a listed Origin, got %d", resp.Code)
		}
	})

	t.Run("AllowsMissingOrigin", func(t *testing.T) {
		// Non-browser clients send no Origin and cannot be made to attach ambient
		// credentials, so they are not what this check defends against.
		_, base := humatest.New(t)
		mcp.Attach(base, mcp.Options{Name: "o", Version: "1.0.0", AllowedOrigins: []string{"https://app.example.com"}})

		if resp := base.Post("/mcp", strings.NewReader(ping)); resp.Code != http.StatusOK {
			t.Errorf("expected 200 with no Origin, got %d", resp.Code)
		}
	})

	t.Run("UnconfiguredAllowsAny", func(t *testing.T) {
		_, base := humatest.New(t)
		mcp.Attach(base, mcp.Options{Name: "o", Version: "1.0.0"})

		if resp := base.Post("/mcp", strings.NewReader(ping), "Origin: https://anywhere.example.com"); resp.Code != http.StatusOK {
			t.Errorf("expected 200 when unconfigured, got %d", resp.Code)
		}
	})
}

func TestBodyTooLarge(t *testing.T) {
	api, _ := setup(t)

	resp := api.Post("/mcp", strings.NewReader(strings.Repeat(" ", 1024*1024+1)))
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", resp.Code)
	}
}

func TestBatchRejected(t *testing.T) {
	api, _ := setup(t)

	out := rpc(t, api, `[{"jsonrpc":"2.0","id":1,"method":"tools/list"}]`)
	if errObj, ok := out["error"].(map[string]any); !ok || errObj["code"].(float64) != -32600 {
		t.Errorf("expected invalid request, got %v", out)
	}
}

func TestNotificationNotExecuted(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "notify", Version: "1.0.0"})

	called := false
	huma.Post(api, "/act", func(_ context.Context, _ *struct{}) (*struct{}, error) {
		called = true
		return nil, nil
	}, mcp.Tool(mcp.WithName("act")))

	resp := base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"act","arguments":{}}}`))
	if resp.Code != http.StatusAccepted {
		t.Errorf("expected 202 for an id-less message, got %d: %s", resp.Code, resp.Body.String())
	}
	if called {
		t.Error("tool ran for a notification")
	}
}

func TestParseErrorHasNullID(t *testing.T) {
	api, _ := setup(t)

	resp := api.Post("/mcp", strings.NewReader(`{not json`))
	if !strings.Contains(resp.Body.String(), `"id":null`) {
		t.Errorf("expected id null on a parse error, got %s", resp.Body.String())
	}
}

func TestDuplicateToolNamePanics(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "dup", Version: "1.0.0"})

	handler := func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}
	huma.Get(api, "/a", handler, mcp.Tool(mcp.WithName("dup")))

	defer func() {
		if r := recover(); r == nil || !strings.Contains(fmt.Sprint(r), "duplicate MCP tool name dup") {
			t.Errorf("expected a duplicate tool name panic, got %v", r)
		}
	}()
	huma.Get(api, "/b", handler, mcp.Tool(mcp.WithName("dup")))
}

func TestToolsListStable(t *testing.T) {
	type input struct {
		Body struct {
			Zeta  string `json:"zeta"`
			Alpha string `json:"alpha"`
			Mid   string `json:"mid"`
		}
	}

	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "stable", Version: "1.0.0"})
	huma.Post(api, "/stable", func(_ context.Context, _ *input) (*struct{}, error) {
		return nil, nil
	}, mcp.Tool(mcp.WithName("stable")))

	list := func() string {
		return base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)).Body.String()
	}

	first := list()
	if !strings.Contains(first, `"required":["alpha","mid","zeta"]`) {
		t.Errorf("expected required fields in name order, got %s", first)
	}
	for range 5 {
		if got := list(); got != first {
			t.Fatalf("tools/list changed between calls:\n%s\n%s", first, got)
		}
	}
}

func TestAttachToGroupPanics(t *testing.T) {
	_, base := humatest.New(t)

	defer func() {
		if recover() == nil {
			t.Error("expected Attach to panic on a group")
		}
	}()
	mcp.Attach(huma.NewGroup(base, "/v1"), mcp.Options{})
}

func TestWithNameMultiPrefixGroup(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "grp", Version: "1.0.0"})

	grp := huma.NewGroup(api, "/v1", "/v2")
	huma.Get(grp, "/greeting/{name}", func(_ context.Context, in *greetingInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("greet")))

	names := listToolNames(t, base)
	if !names["v1-greet"] || !names["v2-greet"] || len(names) != 2 {
		t.Errorf("expected v1-greet and v2-greet, got %v", names)
	}
}

func TestDuplicateNameOnOriginalAPIPanics(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "dup", Version: "1.0.0"})
	handler := func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}
	huma.Get(api, "/a", handler, mcp.Tool(mcp.WithName("same")))

	defer func() {
		if recover() == nil {
			t.Error("expected a duplicate tool name panic")
		}
	}()
	huma.Get(base, "/b", handler, mcp.Tool(mcp.WithName("same")))
}

func TestToolSelection(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{ExposeVisible: true})

	handler := func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}
	huma.Get(api, "/described", handler, mcp.Tool(mcp.WithDescription("Custom description")))
	huma.Get(api, "/excluded", handler, mcp.Exclude())
	huma.Register(api, mcp.WithExclude(huma.Operation{OperationID: "registered-excluded", Method: http.MethodGet, Path: "/registered"}), handler)

	// Marking one operation must not mark others built from the same template.
	template := huma.Operation{Method: http.MethodGet, Metadata: map[string]any{}}
	excludedCopy, plainCopy := template, template
	excludedCopy.OperationID, excludedCopy.Path = "template-excluded", "/template-excluded"
	plainCopy.OperationID, plainCopy.Path = "template-plain", "/template-plain"
	huma.Register(api, mcp.WithExclude(excludedCopy), handler)
	huma.Register(api, plainCopy, handler)
	huma.Get(api, "/named", handler, mcp.Tool(mcp.WithName("bad name!"+strings.Repeat("x", 200))))

	list := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	tools := map[string]string{}
	for _, tRaw := range list["result"].(map[string]any)["tools"].([]any) {
		tm := tRaw.(map[string]any)
		tools[tm["name"].(string)], _ = tm["description"].(string)
	}

	if tools["get-described"] != "Custom description" {
		t.Errorf("expected the custom description, got %v", tools)
	}
	if _, ok := tools["get-excluded"]; ok {
		t.Error("expected Exclude to hide the operation")
	}
	if _, ok := tools["registered-excluded"]; ok {
		t.Error("expected Exclude to hide the registered operation")
	}
	if _, ok := tools["template-plain"]; !ok {
		t.Errorf("expected an operation sharing a template with an excluded one to stay exposed, got %v", tools)
	}
	if _, ok := tools["bad_name_"+strings.Repeat("x", 119)]; !ok {
		t.Errorf("expected a sanitized, truncated name, got %v", tools)
	}

	// Name and version default to the OpenAPI info.
	info := rpc(t, base, `{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}`)["result"].(map[string]any)["serverInfo"].(map[string]any)
	if info["name"] != base.OpenAPI().Info.Title || info["version"] != base.OpenAPI().Info.Version {
		t.Errorf("expected server info from the OpenAPI document, got %v", info)
	}
}

func TestMalformedRequests(t *testing.T) {
	api, _ := setup(t)

	if resp := api.Post("/mcp", strings.NewReader("")); resp.Code != http.StatusAccepted {
		t.Errorf("expected 202 for an empty body, got %d", resp.Code)
	}

	out := rpc(t, api, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"nope"}`)
	if errObj, ok := out["error"].(map[string]any); !ok || errObj["code"].(float64) != -32602 {
		t.Errorf("expected invalid params, got %v", out)
	}
}

func TestAttachPanicsOnDuplicateName(t *testing.T) {
	_, base := humatest.New(t)
	handler := func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}
	huma.Get(base, "/a", handler, mcp.Tool(mcp.WithName("same")))
	huma.Get(base, "/b", handler, mcp.Tool(mcp.WithName("same")))

	defer func() {
		if recover() == nil {
			t.Error("expected Attach to panic on a duplicate tool name")
		}
	}()
	mcp.Attach(base, mcp.Options{})
}
