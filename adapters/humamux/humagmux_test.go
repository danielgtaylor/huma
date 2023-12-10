package humamux

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gorilla/mux"
)

var lastModified = time.Now()

func BenchmarkHumaGorillaMux(b *testing.B) {
	type GreetingInput struct {
		ID          string `path:"id"`
		ContentType string `header:"Content-Type"`
		Num         int    `query:"num"`
		Body        struct {
			Suffix string `json:"suffix" maxLength:"5"`
		}
	}

	type GreetingOutput struct {
		ETag         string    `header:"ETag"`
		LastModified time.Time `header:"Last-Modified"`
		Body         struct {
			Greeting    string `json:"greeting"`
			Suffix      string `json:"suffix"`
			Length      int    `json:"length"`
			ContentType string `json:"content_type"`
			Num         int    `json:"num"`
		}
	}

	r := mux.NewRouter()
	app := New(r, huma.DefaultConfig("Test", "1.0.0"))

	huma.Register(app, huma.Operation{
		OperationID: "greet",
		Method:      http.MethodPost,
		Path:        "/foo/{id}",
	}, func(ctx context.Context, input *GreetingInput) (*GreetingOutput, error) {
		resp := &GreetingOutput{}
		resp.ETag = "abc123"
		resp.LastModified = lastModified
		resp.Body.Greeting = "Hello, " + input.ID + input.Body.Suffix
		resp.Body.Suffix = input.Body.Suffix
		resp.Body.Length = len(resp.Body.Greeting)
		resp.Body.ContentType = input.ContentType
		resp.Body.Num = input.Num
		return resp, nil
	})

	reqBody := strings.NewReader(`{"suffix": "!"}`)
	req, _ := http.NewRequest(http.MethodPost, "/foo/123?num=5", reqBody)
	req.Header.Set("Content-Type", "application/json")
	b.ResetTimer()
	b.ReportAllocs()
	w := httptest.NewRecorder()
	for i := 0; i < b.N; i++ {
		reqBody.Seek(0, 0)
		w.Body.Reset()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatal(w.Body.String())
		}
	}
}
