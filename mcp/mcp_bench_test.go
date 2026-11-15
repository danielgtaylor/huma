package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/mcp"
)

func BenchmarkToolsCall(b *testing.B) {
	type input struct {
		ID   string `path:"id"`
		Num  int    `query:"num"`
		Body struct {
			Suffix string `json:"suffix" maxLength:"5"`
		}
	}
	type output struct {
		Body struct {
			Greeting string `json:"greeting"`
			Num      int    `json:"num"`
		}
	}

	mux := http.NewServeMux()
	api, _ := mcp.Attach(humago.New(mux, huma.DefaultConfig("Bench", "1.0.0")), mcp.Options{})
	huma.Post(api, "/greet/{id}", func(_ context.Context, in *input) (*output, error) {
		out := &output{}
		out.Body.Greeting = "Hello, " + in.ID + in.Body.Suffix
		out.Body.Num = in.Num
		return out, nil
	}, mcp.Tool(mcp.WithName("greet")))

	// The direct request is the baseline: the same operation without MCP.
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "direct", path: "/greet/123?num=5", body: `{"suffix":"!"}`},
		{name: "mcp", path: "/mcp", body: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"greet","arguments":{"id":"123","num":5,"suffix":"!"}}}`},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			body := strings.NewReader(tt.body)
			req, _ := http.NewRequest(http.MethodPost, tt.path, body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				body.Seek(0, 0)
				w.Body.Reset()
				mux.ServeHTTP(w, req)
				if w.Code != http.StatusOK || strings.Contains(w.Body.String(), `"isError":true`) {
					b.Fatal(w.Body.String())
				}
			}
		})
	}
}

func BenchmarkToolsList(b *testing.B) {
	mux := http.NewServeMux()
	api, _ := mcp.Attach(humago.New(mux, huma.DefaultConfig("Bench", "1.0.0")), mcp.Options{ExposeVisible: true})
	for i := range 100 {
		huma.Get(api, "/greeting/"+strconv.Itoa(i)+"/{name}", func(_ context.Context, _ *greetingInput) (*greetingOutput, error) {
			return &greetingOutput{}, nil
		})
	}

	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	req, _ := http.NewRequest(http.MethodPost, "/mcp", body)
	w := httptest.NewRecorder()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body.Seek(0, 0)
		w.Body.Reset()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatal(w.Body.String())
		}
	}
}
