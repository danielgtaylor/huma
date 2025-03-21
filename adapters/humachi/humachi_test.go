package humachi

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
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
)

var lastModified = time.Now()

func BenchmarkHumaV2ChiNormal(b *testing.B) {
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

	r := chi.NewMux()
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

func BenchmarkHumaV2ChiResolver(b *testing.B) {
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

	r := chi.NewMux()
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

func BenchmarkRawChi(b *testing.B) {
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
	schema := registry.Schema(reflect.TypeOf(GreetingInput{}), false, "")

	strSchema := registry.Schema(reflect.TypeOf(""), false, "")
	numSchema := registry.Schema(reflect.TypeOf(0), false, "")

	r := chi.NewMux()

	r.Post("/foo/{id}", func(w http.ResponseWriter, r *http.Request) {
		pb := huma.NewPathBuffer([]byte{}, 0)
		res := &huma.ValidateResult{}

		// Read and validate params
		id := chi.URLParam(r, "id")
		huma.Validate(registry, strSchema, pb, huma.ModeReadFromServer, id, res)

		ct := r.Header.Get("Content-Type")
		huma.Validate(registry, strSchema, pb, huma.ModeReadFromServer, ct, res)

		num, err := strconv.Atoi(r.URL.Query().Get("num"))
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

func BenchmarkRawChiFast(b *testing.B) {
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

	r := chi.NewMux()

	r.Post("/foo/{id}", func(w http.ResponseWriter, r *http.Request) {
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
		resp.Greeting = "Hello, " + chi.URLParam(r, "id") + input.Suffix
		resp.Suffix = input.Suffix
		resp.Length = len(resp.Greeting)
		resp.ContentType = r.Header.Get("Content-Type")
		resp.Num, _ = strconv.Atoi(r.URL.Query().Get("num"))
		data, err = json.Marshal(resp)
		if err != nil {
			panic(err)
		}
		w.Write(data)
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

func TestChiRouterPrefix(t *testing.T) {
	mux := chi.NewMux()
	var api huma.API
	mux.Route("/api", func(r chi.Router) {
		config := huma.DefaultConfig("My API", "1.0.0")
		config.Servers = []*huma.Server{{URL: "http://localhost:8888/api"}}
		api = New(r, config)
	})

	type TestOutput struct {
		Body struct {
			Field string `json:"field"`
		}
	}

	// Register a simple hello world operation in the API.
	huma.Get(api, "/test", func(ctx context.Context, input *struct{}) (*TestOutput, error) {
		return &TestOutput{}, nil
	})

	// Create a test API around the underlying router to make easier requests.
	tapi := humatest.Wrap(t, New(mux, huma.DefaultConfig("Test", "1.0.0")))

	// The top-level router should respond to the full path even though the
	// operation was registered with just `/test`.
	resp := tapi.Get("/api/test")
	assert.Equal(t, http.StatusOK, resp.Code)

	// The transformer should generate links with the full URL path.
	assert.Contains(t, resp.Header().Get("Link"), "/api/schemas/TestOutputBody.json")

	// The docs HTML should point to the full URL including base path.
	resp = tapi.Get("/api/docs")
	assert.Equal(t, http.StatusOK, resp.Code)
	// The openapi.yaml should be a relative path.
	assert.Contains(t, resp.Body.String(), `apiDescriptionUrl="openapi.yaml"`)
}

// func BenchmarkHumaV1Chi(t *testing.B) {
// 	type GreetingInput struct {
// 		ID          string `path:"id"`
// 		ContentType string `header:"Content-Type"`
// 		Num         int    `query:"num"`
// 		Body        struct {
// 			Suffix string `json:"suffix" maxLength:"5"`
// 		}
// 	}

// 	type GreetingOutput struct {
// 		Greeting    string `json:"greeting"`
// 		Suffix      string `json:"suffix"`
// 		Length      int    `json:"length"`
// 		ContentType string `json:"content_type"`
// 		Num         int    `json:"num"`
// 	}

// 	app := humav1.New("My API", "1.0.0")

// 	app.Resource("/foo/{id}").Post("greet", "Get a greeting",
// 		responses.OK().Model(&GreetingOutput{}).Headers("ETag", "Last-Modified"),
// 	).Run(func(ctx humav1.Context, input GreetingInput) {
// 		ctx.Header().Set("ETag", "abc123")
// 		ctx.Header().Set("Last-Modified", lastModified.Format(http.TimeFormat))
// 		resp := &GreetingOutput{}
// 		resp.Greeting = "Hello, " + input.ID + input.Body.Suffix
// 		resp.Suffix = input.Body.Suffix
// 		resp.Length = len(resp.Greeting)
// 		resp.ContentType = input.ContentType
// 		resp.Num = input.Num
// 		ctx.WriteModel(http.StatusOK, resp)
// 	})

// 	reqBody := strings.NewReader(`{"suffix": "!"}`)
// 	req, _ := http.NewRequest(http.MethodPost, "/foo/123?num=5", reqBody)
// 	req.Header.Set("Content-Type", "application/json")
// 	w := httptest.NewRecorder()
// 	t.ResetTimer()
// 	t.ReportAllocs()
// 	for i := 0; i < t.N; i++ {
// 		reqBody.Seek(0, 0)
// 		w.Body.Reset()
// 		app.ServeHTTP(w, req)
// 	}
// }
