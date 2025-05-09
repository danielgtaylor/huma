package humamux

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
)

var lastModified = time.Now()

type TestInput struct {
	Group      string `path:"group"`
	Verbose    bool   `query:"verbose"`
	Auth       string `header:"Authorization"`
	TestHeader string `header:"TestHeader"`
	Body       struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
}

// Test outputs (headers, body).
type TestOutput struct {
	MyHeader   string `header:"MyHeader"`
	TestHeader string `header:"TestHeader"`
	Body       struct {
		Message string `json:"message"`
	}
}

func testHandler(ctx context.Context, input *TestInput) (*TestOutput, error) {
	resp := &TestOutput{}
	resp.MyHeader = "my-value"
	resp.TestHeader = input.TestHeader
	resp.Body.Message = fmt.Sprintf("Hello, %s <%s>! (%s, %v, %s)", input.Body.Name, input.Body.Email, input.Group, input.Verbose, input.Auth)
	return resp, nil
}

func TestCustomMiddleware(t *testing.T) {
	mw1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("TestHeader", "test-value")
			next.ServeHTTP(w, r)
		})
	}

	r := mux.NewRouter()
	api := New(r, huma.DefaultConfig("Test", "1.0.0"),
		WithRouteCustomizer(func(op *huma.Operation, r *mux.Route) {
			r.Handler(mw1(r.GetHandler()))
		}))

	huma.Register(api, huma.Operation{
		OperationID: "test",
		Method:      http.MethodGet,
		Path:        "/{group}",
	}, testHandler)

	testAPI := humatest.Wrap(t, api)
	resp := testAPI.Do(http.MethodGet, "/foo",
		"Host: localhost",
		"Authorization: Bearer abc123",
		strings.NewReader(`{"name": "Daniel", "email": "daniel@example.com"}`),
	)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "my-value", resp.Header().Get("MyHeader"))
	assert.Equal(t, "test-value", resp.Header().Get("TestHeader"))
}

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
