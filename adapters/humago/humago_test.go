package humago

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
)

var lastModified = time.Now()

func BenchmarkHumaV2Go(b *testing.B) {
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

	r := http.NewServeMux()
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

func BenchmarkRawGo(b *testing.B) {
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

	r := http.NewServeMux()

	r.HandleFunc("POST /foo/{id}", func(w http.ResponseWriter, r *http.Request) {
		pb := huma.NewPathBuffer([]byte{}, 0)
		res := &huma.ValidateResult{}

		// Read and validate params
		id := ""
		var v any = r
		if pv, ok := v.(interface{ PathValue(string) string }); ok {
			id = pv.PathValue("id")
		}
		huma.ValidateAndSetDefaults(registry, strSchema, pb, huma.ModeReadFromServer, id, res)

		ct := r.Header.Get("Content-Type")
		huma.ValidateAndSetDefaults(registry, strSchema, pb, huma.ModeReadFromServer, ct, res)

		num, err := strconv.Atoi(r.URL.Query().Get("num"))
		if err != nil {
			panic(err)
		}
		huma.ValidateAndSetDefaults(registry, numSchema, pb, huma.ModeReadFromServer, num, res)

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

		huma.ValidateAndSetDefaults(registry, schema, pb, huma.ModeWriteToServer, tmp, res)
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
		if w.Code != http.StatusOK {
			b.Fatal(w.Body.String())
		}
	}
}
