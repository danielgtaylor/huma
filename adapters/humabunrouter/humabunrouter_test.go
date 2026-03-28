package humabunrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bunrouter"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

var lastModified = time.Now()

func BenchmarkHumaV2BunRouterNormal(b *testing.B) {
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

	r := bunrouter.New()
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

type GreetingInputWithResolverBody struct {
	Suffix string `json:"suffix" maxLength:"5"`
}

func (b *GreetingInputWithResolverBody) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	if len(b.Suffix) > 0 && b.Suffix[0] == 'a' {
		return []error{&huma.ErrorDetail{
			Location: prefix.With("suffix"),
			Message:  "foo bar baz",
			Value:    b.Suffix,
		}}
	}
	return nil
}

type GreetingInputWithResolver struct {
	ID          string `path:"id"`
	ContentType string `header:"Content-Type"`
	Num         int    `query:"num"`
	Body        GreetingInputWithResolverBody
}

func (i *GreetingInputWithResolver) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	if i.Num == 3 {
		return []error{&huma.ErrorDetail{
			Location: prefix.With("num"),
			Message:  "foo bar baz",
			Value:    i.Num,
		}}
	}
	return nil
}

func BenchmarkHumaV2BunRouterResolver(b *testing.B) {
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

	r := bunrouter.New()
	app := New(r, huma.DefaultConfig("Test", "1.0.0"))

	huma.Register(app, huma.Operation{
		OperationID: "greet",
		Method:      http.MethodPost,
		Path:        "/foo/{id}",
	}, func(ctx context.Context, input *GreetingInputWithResolver) (*GreetingOutput, error) {
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

func BenchmarkRawBunRouter(b *testing.B) {
	type GreetingInput struct {
		Suffix string `json:"suffix" maxLength:"5"`
	}

	type GreetingOutput struct {
		Schema      string `json:"$schema"`
		Greeting    string `json:"greeting"`
		Suffix      string `json:"suffix"`
		Length      int    `json:"length"`
		ContentType string `json:"content_type"`
		Num         int    `json:"num"`
	}

	registry := huma.NewMapRegistry("#/components/schemas/",
		func(t reflect.Type, hint string) string {
			return t.Name()
		})
	schema := registry.Schema(reflect.TypeFor[GreetingInput](), false, "")

	strSchema := registry.Schema(reflect.TypeFor[string](), false, "")
	numSchema := registry.Schema(reflect.TypeFor[int](), false, "")

	r := bunrouter.New()

	r.POST("/foo/:id", func(w http.ResponseWriter, r bunrouter.Request) error {
		pb := huma.NewPathBuffer([]byte{}, 0)
		res := &huma.ValidateResult{}

		// Read and validate params
		id := r.Param("id")
		huma.Validate(registry, strSchema, pb, huma.ModeReadFromServer, id, res)

		ct := r.Header.Get("Content-Type")
		huma.Validate(registry, strSchema, pb, huma.ModeReadFromServer, ct, res)

		num, err := strconv.Atoi(r.URL.Query().Get("num"))
		if err != nil {
			return err
		}
		huma.Validate(registry, numSchema, pb, huma.ModeReadFromServer, num, res)

		// Read and validate body
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}

		var tmp any
		if err := json.Unmarshal(data, &tmp); err != nil {
			return err
		}

		huma.Validate(registry, schema, pb, huma.ModeWriteToServer, tmp, res)
		if len(res.Errors) > 0 {
			return fmt.Errorf("%v", res.Errors)
		}

		var input GreetingInput
		if err := json.Unmarshal(data, &input); err != nil {
			return err
		}

		// Set up and write the response
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", "abc123")
		w.Header().Set("Last-Modified", lastModified.Format(http.TimeFormat))
		w.Header().Set("Link", "</schemas/GreetingOutput.json>; rel=\"describedBy\"")
		w.WriteHeader(http.StatusOK)
		resp := &GreetingOutput{}
		resp.Schema = "/schemas/GreetingOutput.json"
		resp.Greeting = "Hello, " + id + input.Suffix
		resp.Suffix = input.Suffix
		resp.Length = len(resp.Greeting)
		resp.ContentType = ct
		resp.Num = num
		data, err = json.Marshal(resp)
		if err != nil {
			return err
		}
		w.Write(data)

		return nil
	})

	reqBody := strings.NewReader(`{"suffix": "!"}`)
	req, _ := http.NewRequest(http.MethodPost, "/foo/123?num=5", reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqBody.Seek(0, 0)
		w.Body.Reset()
		r.ServeHTTP(w, req)
	}
}

func BenchmarkRawBunRouterFast(b *testing.B) {
	type GreetingInput struct {
		Suffix string `json:"suffix" maxLength:"5"`
	}

	type GreetingOutput struct {
		Greeting    string `json:"greeting"`
		Suffix      string `json:"suffix"`
		Length      int    `json:"length"`
		ContentType string `json:"content_type"`
		Num         int    `json:"num"`
	}

	r := bunrouter.New()

	r.POST("/foo/:id", func(w http.ResponseWriter, r bunrouter.Request) error {
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return err
		}

		var input GreetingInput
		if err := json.Unmarshal(data, &input); err != nil {
			return err
		}

		if len(input.Suffix) > 5 {
			return errors.New("suffix too long")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", "abc123")
		w.Header().Set("Last-Modified", lastModified.Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
		resp := &GreetingOutput{}
		resp.Greeting = "Hello, " + r.Param("id") + input.Suffix
		resp.Suffix = input.Suffix
		resp.Length = len(resp.Greeting)
		resp.ContentType = r.Header.Get("Content-Type")
		resp.Num, _ = strconv.Atoi(r.URL.Query().Get("num"))
		data, err = json.Marshal(resp)
		if err != nil {
			return err
		}
		w.Write(data)

		return nil
	})

	reqBody := strings.NewReader(`{"suffix": "!"}`)
	req, _ := http.NewRequest(http.MethodPost, "/foo/123?num=5", reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reqBody.Seek(0, 0)
		w.Body.Reset()
		r.ServeHTTP(w, req)
	}
}

// See https://github.com/danielgtaylor/huma/issues/859
func TestWithValueShouldPropagateContext(t *testing.T) {
	r := bunrouter.New()
	app := New(r, huma.DefaultConfig("Test", "1.0.0"))

	type (
		testInput  struct{}
		testOutput struct{}
		ctxKey     struct{}
	)

	ctxValue := "sentinelValue"

	huma.Register(app, huma.Operation{
		OperationID: "test",
		Path:        "/test",
		Method:      http.MethodGet,
		Middlewares: huma.Middlewares{
			func(ctx huma.Context, next func(huma.Context)) {
				ctx = huma.WithValue(ctx, ctxKey{}, ctxValue)
				next(ctx)
			},
			middleware(func(next bunrouter.HandlerFunc) bunrouter.HandlerFunc {
				return func(w http.ResponseWriter, r bunrouter.Request) error {
					val, _ := r.Context().Value(ctxKey{}).(string)
					_, err := io.WriteString(w, val)
					return err
				}
			}),
		},
	}, func(ctx context.Context, input *testInput) (*testOutput, error) {
		out := &testOutput{}
		return out, nil
	})

	tapi := humatest.Wrap(t, app)

	resp := tapi.Get("/test")
	assert.Equal(t, http.StatusOK, resp.Code)
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, ctxValue, string(out))
}
