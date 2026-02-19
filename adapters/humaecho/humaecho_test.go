package humaecho

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var lastModified = time.Now()

func BenchmarkHumaEcho(b *testing.B) {
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

	r := echo.New()
	app := New(r, huma.DefaultConfig("Test", "1.0.0"))

	huma.Register(app, huma.Operation{
		OperationID: "greet",
		Method:      http.MethodPost,
		Path:        "/foo/:id",
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

func BenchmarkRawEcho(b *testing.B) {
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

	r := echo.New()

	r.POST("/foo/:id", func(c echo.Context) error {
		r := c.Request()
		w := c.Response()

		pb := huma.NewPathBuffer([]byte{}, 0)
		res := &huma.ValidateResult{}

		// Read and validate params
		id := c.Param("id")
		huma.Validate(registry, strSchema, pb, huma.ModeReadFromServer, id, res)

		ct := r.Header.Get("Content-Type")
		huma.Validate(registry, strSchema, pb, huma.ModeReadFromServer, ct, res)

		num, err := strconv.Atoi(c.QueryParam("num"))
		if err != nil {
			panic(err)
		}
		huma.Validate(registry, numSchema, pb, huma.ModeReadFromServer, num, res)

		// Read and validate body
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		var tmp any
		if err := json.Unmarshal(data, &tmp); err != nil {
			panic(err)
		}

		huma.Validate(registry, schema, pb, huma.ModeWriteToServer, tmp, res)
		if len(res.Errors) > 0 {
			panic(res.Errors)
		}

		var input GreetingInput
		if err := json.Unmarshal(data, &input); err != nil {
			panic(err)
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
			panic(err)
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

func BenchmarkRawEchoFast(b *testing.B) {
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

	r := echo.New()

	r.POST("/foo/:id", func(c echo.Context) error {
		r := c.Request()
		w := c.Response()

		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}

		var input GreetingInput
		if err := json.Unmarshal(data, &input); err != nil {
			panic(err)
		}

		if len(input.Suffix) > 5 {
			panic("suffix too long")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", "abc123")
		w.Header().Set("Last-Modified", lastModified.Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
		resp := &GreetingOutput{}
		resp.Greeting = "Hello, " + c.Param("id") + input.Suffix
		resp.Suffix = input.Suffix
		resp.Length = len(resp.Greeting)
		resp.ContentType = c.Request().Header.Get("Content-Type")
		resp.Num, _ = strconv.Atoi(c.QueryParam("num"))
		data, err = json.Marshal(resp)
		if err != nil {
			panic(err)
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
	r := echo.New()
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
			middleware(func(next echo.HandlerFunc) echo.HandlerFunc {
				return func(c echo.Context) error {
					val, _ := c.Request().Context().Value(ctxKey{}).(string)
					_, err := io.WriteString(c.Response().Writer, val)
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
