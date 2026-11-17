package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/mcp"
	"github.com/go-chi/chi/v5"
)

func TestAuthHeaderForwarding(t *testing.T) {
	_, base := humatest.New(t)

	// Auth middleware modeled on the MANRS pattern: operations declaring an
	// apiKey security requirement need a valid bearer token.
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

	api, _ := mcp.Attach(base, mcp.Options{Name: "auth", Version: "1.0.0"})

	huma.Post(api, "/admin/thing", func(_ context.Context, in *createInput) (*createOutput, error) {
		out := &createOutput{}
		out.Body.Name = in.Body.Name
		return out, nil
	}, mcp.Tool(mcp.WithName("admin_thing")), func(o *huma.Operation) {
		o.Hidden = true
		o.Security = []map[string][]string{{"apiKey": {"admin"}}}
	})

	callBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"admin_thing","arguments":{"name":"n","count":1}}}`

	// Without a token the internal tool call must be rejected by the operation's
	// own auth middleware.
	noAuth := rpc(t, base, callBody)
	if noAuth["result"].(map[string]any)["isError"] != true {
		t.Fatalf("expected auth failure without token, got %v", noAuth)
	}

	// With the caller's Authorization header, it is forwarded onto the internal
	// request and the call succeeds.
	resp := base.Post("/mcp", strings.NewReader(callBody), "Authorization: Bearer secret")
	if resp.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var withAuth map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &withAuth); err != nil {
		t.Fatal(err)
	}
	result := withAuth["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("expected success with forwarded token, got %v", result)
	}
	if result["structuredContent"].(map[string]any)["name"] != "n" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestOriginForwarding(t *testing.T) {
	_, base := humatest.New(t)

	// Record the connection details the tool's own middleware sees, as a rate
	// limiter or a URL builder would.
	var seen string
	var seenHost string
	var seenTLS bool
	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Operation().OperationID == "remote-addr" {
			seen = ctx.RemoteAddr()
			seenHost = ctx.Host()
			seenTLS = ctx.TLS() != nil
		}
		next(ctx)
	})

	api, _ := mcp.Attach(base, mcp.Options{Name: "addr", Version: "1.0.0"})

	huma.Register(api, mcp.WithTool(huma.Operation{
		OperationID: "remote-addr",
		Method:      http.MethodGet,
		Path:        "/remote-addr",
	}), func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "https://api.example.com/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"remote-addr","arguments":{}}}`))
	req.RemoteAddr = "203.0.113.7:4242"
	base.Adapter().ServeHTTP(httptest.NewRecorder(), req)

	if seen != "203.0.113.7:4242" {
		t.Errorf("tool request remote address = %q, want the caller's", seen)
	}
	if seenHost != "api.example.com" || !seenTLS {
		t.Errorf("tool request host = %q, TLS = %v, want the caller's", seenHost, seenTLS)
	}
}

func TestToolsCallGet(t *testing.T) {
	api, _ := setup(t)

	// Find the greeting tool name via tools/list.
	list := rpc(t, api, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	greetingName := ""
	for _, tRaw := range list["result"].(map[string]any)["tools"].([]any) {
		tm := tRaw.(map[string]any)
		props := tm["inputSchema"].(map[string]any)["properties"].(map[string]any)
		if _, ok := props["greeting"]; ok {
			greetingName = tm["name"].(string)
		}
	}
	if greetingName == "" {
		t.Fatal("greeting tool not found")
	}

	body := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + greetingName +
		`","arguments":{"name":"World","greeting":"Hi"}}}`
	out := rpc(t, api, body)

	result := out["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("unexpected error result: %v", result)
	}
	content := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "Hi, World") {
		t.Errorf("unexpected content: %s", text)
	}

	structured := result["structuredContent"].(map[string]any)
	if structured["message"] != "Hi, World" {
		t.Errorf("unexpected structuredContent: %v", structured)
	}
}

func TestToolsCallPostBody(t *testing.T) {
	api, _ := setup(t)

	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_thing",` +
		`"arguments":{"name":"widget","count":5}}}`
	out := rpc(t, api, body)

	result := out["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("unexpected error: %v", result)
	}
	structured := result["structuredContent"].(map[string]any)
	if structured["name"] != "widget" {
		t.Errorf("expected name=widget, got %v", structured["name"])
	}
	if structured["count"].(float64) != 5 {
		t.Errorf("expected count=5, got %v", structured["count"])
	}
}

func TestToolsCallValidationError(t *testing.T) {
	api, _ := setup(t)

	// count minimum is 1; sending 0 should surface Huma's validation error.
	body := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"create_thing",` +
		`"arguments":{"name":"widget","count":0}}}`
	out := rpc(t, api, body)

	result := out["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("expected isError=true, got %v", result)
	}
}

// Cookie must not be forwarded unless asked for, since it is ambient and would
// let any page reaching this endpoint act as the visitor.
func TestCookieNotForwardedByDefault(t *testing.T) {
	_, base := humatest.New(t)

	var gotCookie, gotAuth string

	base.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if strings.HasPrefix(ctx.URL().Path, "/echo") {
			gotCookie = ctx.Header("Cookie")
			gotAuth = ctx.Header("Authorization")
		}
		next(ctx)
	})

	api, _ := mcp.Attach(base, mcp.Options{Name: "c", Version: "1.0.0"})

	huma.Post(api, "/echo", func(_ context.Context, in *createInput) (*createOutput, error) {
		return &createOutput{}, nil
	}, mcp.Tool(mcp.WithName("echo")))

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"name":"n","count":1}}}`
	base.Post("/mcp", strings.NewReader(body), "Authorization: Bearer secret", "Cookie: session=abc")

	if gotCookie != "" {
		t.Errorf("Cookie was forwarded by default: %q", gotCookie)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want it forwarded", gotAuth)
	}
}

// chi keeps its routing state in the request context and reuses any it finds, so
// a tool request inheriting the MCP endpoint's context was misrouted back to that
// endpoint and answered 405. humatest holds routing state elsewhere, so the rest
// of this suite cannot catch it.
func TestToolCallThroughChiAdapter(t *testing.T) {
	router := chi.NewMux()
	api := humachi.New(router, huma.DefaultConfig("Chi API", "1.0.0"))

	api, _ = mcp.Attach(api, mcp.Options{Name: "chi", Version: "1.0.0"})

	huma.Get(api, "/greeting/{name}", func(_ context.Context, in *greetingInput) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "Hello, " + in.Name

		return out, nil
	}, mcp.Tool(mcp.WithName("greet")))

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"greet","arguments":{"name":"world"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, rec.Body.String())
	}

	if resp.Result.IsError {
		t.Fatalf("tool call failed: %s", rec.Body.String())
	}
	if len(resp.Result.Content) == 0 || !strings.Contains(resp.Result.Content[0].Text, "Hello, world") {
		t.Errorf("expected the operation's response, got %s", rec.Body.String())
	}
}

func TestUnresolvableBodyRef(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "ref", Version: "1.0.0"})

	// A body schema whose ref the registry cannot resolve must not take the
	// server down; the tool falls back to exposing the whole body.
	huma.Register(api, mcp.WithTool(huma.Operation{
		OperationID: "dangling",
		Method:      http.MethodPost,
		Path:        "/dangling",
		RequestBody: &huma.RequestBody{Content: map[string]*huma.MediaType{
			"application/json": {Schema: &huma.Schema{Ref: "#/components/schemas/Missing"}},
		}},
	}), func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return nil, nil
	})

	tools := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", tools)
	}
	props := tools[0].(map[string]any)["inputSchema"].(map[string]any)["properties"].(map[string]any)
	if _, ok := props["body"]; !ok {
		t.Errorf("expected the body exposed whole, got %v", props)
	}
}

func TestQueryCollectionArguments(t *testing.T) {
	type input struct {
		Tags   []string          `query:"tags"`
		IDs    []int             `query:"ids,explode"`
		Filter map[string]string `query:"filter,deepObject"`
	}
	type output struct {
		Body struct {
			Tags   []string          `json:"tags"`
			IDs    []int             `json:"ids"`
			Filter map[string]string `json:"filter"`
		}
	}

	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "query", Version: "1.0.0"})

	huma.Get(api, "/search", func(_ context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.Tags, out.Body.IDs, out.Body.Filter = in.Tags, in.IDs, in.Filter
		return out, nil
	}, mcp.Tool(mcp.WithName("search")))

	out := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"tags":["a","b"],"ids":[1,2],"filter":{"kind":"x"}}}}`)
	result := out["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("tool call failed: %v", result)
	}

	got, _ := json.Marshal(result["structuredContent"])
	if want := `{"filter":{"kind":"x"},"ids":[1,2],"tags":["a","b"]}`; string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestMissingPathArgument(t *testing.T) {
	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "path", Version: "1.0.0"})

	called := false
	huma.Get(api, "/things/{id}", func(_ context.Context, _ *struct {
		ID string `path:"id"`
	},
	) (*greetingOutput, error) {
		called = true
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("get_thing")))

	result := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_thing","arguments":{}}}`)["result"].(map[string]any)
	if result["isError"] != true || !strings.Contains(result["content"].([]any)[0].(map[string]any)["text"].(string), `missing required path argument "id"`) {
		t.Errorf("expected a missing-argument error, got %v", result)
	}
	if called {
		t.Error("handler ran with the {id} placeholder as its value")
	}
}

func TestToolArgumentBinding(t *testing.T) {
	type optionalBodyInput struct {
		Body *struct {
			Name string `json:"name"`
		}
	}
	type bodyParamInput struct {
		Body   []string
		Filter string `query:"body"`
	}
	type requiredBodyInput struct {
		Body struct {
			Name string `json:"name,omitempty" required:"false"`
		}
	}
	type sharedNameInput struct {
		ID      string `path:"id"`
		QueryID string `query:"id" required:"true"`
	}
	type nullableInput struct {
		Body struct {
			Nick *string `json:"nick" nullable:"true"`
		}
	}
	type readOnlyInput struct {
		Body struct {
			ID   string `json:"id" readOnly:"true"`
			Name string `json:"name"`
		}
	}

	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "args", Version: "1.0.0"})

	huma.Get(api, "/greeting/{name}", func(_ context.Context, in *greetingInput) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = "greeting=" + in.Greeting
		return out, nil
	}, mcp.Tool(mcp.WithName("greet")))
	huma.Post(api, "/optional", func(_ context.Context, _ *optionalBodyInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("optional")))
	huma.Post(api, "/list", func(_ context.Context, in *bodyParamInput) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = in.Filter + ":" + strings.Join(in.Body, ",")
		return out, nil
	}, mcp.Tool(mcp.WithName("list")))

	huma.Post(api, "/required", func(_ context.Context, _ *requiredBodyInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("required")))
	huma.Post(api, "/nullable", func(_ context.Context, _ *nullableInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("nullable")))
	huma.Post(api, "/read-only", func(_ context.Context, _ *readOnlyInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("read_only")))
	huma.Get(api, "/shared/{id}", func(_ context.Context, in *sharedNameInput) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = in.ID + ":" + in.QueryID
		return out, nil
	}, mcp.Tool(mcp.WithName("shared")))

	schemas := map[string]map[string]any{}
	list := rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	for _, tRaw := range list["result"].(map[string]any)["tools"].([]any) {
		tm := tRaw.(map[string]any)
		schemas[tm["name"].(string)] = tm["inputSchema"].(map[string]any)
	}

	tests := []struct {
		name string
		call string
		want string
	}{
		{name: "null param is omitted", call: `{"name":"greet","arguments":{"name":"n","greeting":null}}`, want: `"message":"greeting="`},
		{name: "null kept for a nullable whole body", call: `{"name":"list","arguments":{"body":"f","_body":null}}`, want: `"message":"f:"`},
		{name: "body renamed past a param named body", call: `{"name":"list","arguments":{"body":"f","_body":["a","b"]}}`, want: `"message":"f:a,b"`},
		{name: "optional body may be omitted", call: `{"name":"optional","arguments":{}}`, want: `"structuredContent"`},
		{name: "null body field is omitted", call: `{"name":"optional","arguments":{"name":null}}`, want: `"structuredContent"`},
		{name: "null kept for a nullable body field", call: `{"name":"nullable","arguments":{"nick":null}}`, want: `"structuredContent"`},
		{name: "required body with no fields sends {}", call: `{"name":"required","arguments":{}}`, want: `"structuredContent"`},
		{name: "same name in path and query", call: `{"name":"shared","arguments":{"id":"p","_id":"q"}}`, want: `"message":"p:q"`},
		{name: "readOnly field may be omitted", call: `{"name":"read_only","arguments":{"name":"n"}}`, want: `"structuredContent"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":`+tt.call+`}`))
			if !strings.Contains(resp.Body.String(), tt.want) {
				t.Errorf("expected %s in %s", tt.want, resp.Body.String())
			}
		})
	}

	if _, ok := schemas["optional"]["required"]; ok {
		t.Errorf("optional body fields must not be required: %v", schemas["optional"])
	}
	if got := fmt.Sprint(schemas["read_only"]["required"]); got != "[name]" {
		t.Errorf("readOnly fields must not be required, got %s", got)
	}
}

func TestCustomRegistryPrefix(t *testing.T) {
	type thing struct {
		Name string `json:"name"`
	}
	type thingsInput struct {
		Body []thing
	}

	config := huma.DefaultConfig("API", "1.0.0")
	config.Components.Schemas = huma.NewMapRegistry("#/definitions/", huma.DefaultSchemaNamer)
	_, base := humatest.New(t, config)
	api, _ := mcp.Attach(base, mcp.Options{Name: "reg", Version: "1.0.0"})

	huma.Post(api, "/things", func(_ context.Context, _ *thingsInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("things")))

	resp := base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if !strings.Contains(resp.Body.String(), `"$ref":"#/$defs/Thing"`) || !strings.Contains(resp.Body.String(), `"$defs":{"Thing"`) {
		t.Errorf("expected refs rewritten to $defs, got %s", resp.Body.String())
	}
}

func TestHeaderAndCookieArguments(t *testing.T) {
	type input struct {
		Auth    string `header:"Authorization"`
		Session string `cookie:"session"`
		Trace   string `header:"X-Trace" doc:"Trace ID"`
	}

	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "p", Version: "1.0.0", ForwardHeaders: []string{"authorization", "Cookie"}})
	huma.Get(api, "/echo", func(_ context.Context, in *input) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = in.Trace + ":" + in.Session + ":" + in.Auth
		return out, nil
	}, mcp.Tool(mcp.WithName("echo")))

	list := fmt.Sprint(rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if !strings.Contains(list, "Trace ID") {
		t.Errorf("expected the parameter description in the schema, got %v", list)
	}
	if strings.Contains(list, "Authorization") {
		t.Errorf("expected a forwarded header to be left out of the arguments, got %v", list)
	}

	// The cookie argument merges with the forwarded Cookie header, and the
	// forwarded Authorization fills its header parameter.
	resp := base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"X-Trace":"t1","session":"abc"}}}`),
		"Cookie: theme=dark", "Authorization: Bearer fwd")
	if !strings.Contains(resp.Body.String(), `"message":"t1:abc:Bearer fwd"`) {
		t.Errorf("expected header and cookie arguments to reach the handler, got %s", resp.Body.String())
	}

	// A separator in a cookie argument must not split it into another cookie.
	resp = base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"X-Trace":"t1","session":"abc; theme=evil"}}}`))
	if !strings.Contains(resp.Body.String(), `cookie argument \"session\"`) {
		t.Errorf("expected the cookie argument rejected, got %s", resp.Body.String())
	}
}

func TestBodyNestedOnCollision(t *testing.T) {
	type input struct {
		Name string `path:"name"`
		Body struct {
			Name string `json:"name"`
		}
	}

	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "c", Version: "1.0.0"})
	huma.Post(api, "/things/{name}", func(_ context.Context, in *input) (*greetingOutput, error) {
		out := &greetingOutput{}
		out.Body.Message = in.Name + ":" + in.Body.Name
		return out, nil
	}, mcp.Tool(mcp.WithName("rename")))

	resp := base.Post("/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"rename","arguments":{"name":"p","body":{"name":"b"}}}}`))
	if !strings.Contains(resp.Body.String(), `"message":"p:b"`) {
		t.Errorf("expected the body nested under body, got %s", resp.Body.String())
	}
}

func TestSchemaEdgeCases(t *testing.T) {
	type sub struct {
		Name string `json:"name"`
	}
	type pairInput struct {
		Body struct {
			A sub `json:"a"`
			B sub `json:"b"`
		}
	}

	_, base := humatest.New(t)
	api, _ := mcp.Attach(base, mcp.Options{Name: "s", Version: "1.0.0", ForwardHeaders: []string{}})

	// A hand-written parameter keeps its description only on the parameter.
	huma.Register(api, mcp.WithTool(huma.Operation{
		OperationID: "manual",
		Method:      http.MethodGet,
		Path:        "/manual",
		Parameters:  []*huma.Param{{Name: "q", In: "query", Description: "Manual description", Schema: &huma.Schema{Type: "string"}}},
	}), func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	})
	huma.Post(api, "/pair", func(_ context.Context, _ *pairInput) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	}, mcp.Tool(mcp.WithName("pair")))
	huma.Register(api, mcp.WithTool(huma.Operation{Method: http.MethodGet, Path: "/unnamed"}), func(_ context.Context, _ *struct{}) (*greetingOutput, error) {
		return &greetingOutput{}, nil
	})

	list := fmt.Sprint(rpc(t, base, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	for _, want := range []string{"Manual description", "name:tool", "$defs:map[Sub:"} {
		if !strings.Contains(list, want) {
			t.Errorf("expected %q in %s", want, list)
		}
	}
}
