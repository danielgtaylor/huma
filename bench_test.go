package huma_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

type BenchComplexSub struct {
	ID   int    `json:"id" minimum:"1"`
	Name string `json:"name" minLength:"3" maxLength:"20"`
}

type BenchComplex struct {
	String    string                     `json:"string" minLength:"1" maxLength:"100" pattern:"^[a-zA-Z ]+$"`
	Int       int                        `json:"int" minimum:"1" maximum:"1000"`
	Float     float64                    `json:"float" exclusiveMinimum:"0" maximum:"1"`
	Bool      bool                       `json:"bool"`
	Time      time.Time                  `json:"time"`
	Slice     []string                   `json:"slice" minItems:"1" maxItems:"10" uniqueItems:"true"`
	Sub       BenchComplexSub            `json:"sub"`
	SubSlice  []BenchComplexSub          `json:"sub_slice" minItems:"1" maxItems:"5"`
	Map       map[string]BenchComplexSub `json:"map"`
	Recursive *BenchComplex              `json:"recursive,omitempty"`
}

func BenchmarkDefaultSchemaNamer(b *testing.B) {
	t := reflect.TypeFor[[]map[string]BenchComplex]()
	hint := "github.com/danielgtaylor/huma/v2.SomeGenericType[int, string]"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		huma.DefaultSchemaNamer(t, hint)
	}
}

func BenchmarkPathBuffer(b *testing.B) {
	pb := huma.NewPathBuffer(make([]byte, 0, 128), 0)

	b.Run("Push", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pb.Reset()
			pb.Push("users")
			pb.Push("profile")
			pb.Push("settings")
			pb.Push("theme")
		}
	})

	b.Run("PushIndex", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pb.Reset()
			pb.PushIndex(0)
			pb.PushIndex(10)
			pb.PushIndex(100)
			pb.PushIndex(1000)
		}
	})

	b.Run("Mixed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			pb.Reset()
			pb.Push("users")
			pb.PushIndex(5)
			pb.Push("comments")
			pb.PushIndex(42)
			pb.Push("text")
		}
	})
}

func BenchmarkSchemaGeneration(b *testing.B) {
	t := reflect.TypeFor[BenchComplex]()

	b.Run("NewRegistry", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Create a new registry each time to measure generation cost.
			reg := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
			huma.SchemaFromType(reg, t)
		}
	})

	b.Run("ReusedRegistry", func(b *testing.B) {
		reg := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Reuse the registry to see the cost of lookup/caching.
			huma.SchemaFromType(reg, t)
		}
	})

	b.Run("RegistryLookup", func(b *testing.B) {
		reg := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
		reg.Schema(t, true, "") // initial registration
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Measure the cost of a simple lookup for a registered struct.
			reg.Schema(t, true, "")
		}
	})
}

func BenchmarkValidate_Complex(b *testing.B) {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := huma.SchemaFromType(registry, reflect.TypeFor[BenchComplex]())

	input := map[string]any{
		"string": "Hello World",
		"int":    500,
		"float":  0.5,
		"bool":   true,
		"time":   time.Now().Format(time.RFC3339),
		"slice":  []any{"a", "b", "c"},
		"sub": map[string]any{
			"id":   5,
			"name": "Testing",
		},
		"sub_slice": []any{
			map[string]any{"id": 1, "name": "One"},
			map[string]any{"id": 2, "name": "Two"},
		},
		"map": map[string]any{
			"key1": map[string]any{"id": 10, "name": "Ten"},
		},
	}

	pb := huma.NewPathBuffer(make([]byte, 0, 256), 0)
	res := &huma.ValidateResult{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pb.Reset()
		res.Reset()
		huma.Validate(registry, s, pb, huma.ModeWriteToServer, input, res)
	}
}

func BenchmarkRegister(b *testing.B) {
	_, api := humatest.New(b, huma.DefaultConfig("Test API", "1.0.0"))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		huma.Register(api, huma.Operation{
			Method: http.MethodPost,
			Path:   fmt.Sprintf("/test-%d", i),
		}, func(ctx context.Context, input *ComplexInput) (*ComplexOutput, error) {
			return nil, nil
		})
	}
}

type ComplexInput struct {
	ID     string `path:"id"`
	Search string `query:"search" minLength:"3"`
	Sort   string `query:"sort" enum:"asc,desc" default:"asc"`
	Page   int    `query:"page" minimum:"1" default:"1"`
	Body   BenchComplex
}

type ComplexOutput struct {
	Status int `status:"201"`
	Body   struct {
		ID      string       `json:"id"`
		Result  BenchComplex `json:"result"`
		Message string       `json:"message"`
	}
}

func BenchmarkFullRequest_Complex(b *testing.B) {
	_, api := humatest.New(b, huma.DefaultConfig("Test API", "1.0.0"))

	huma.Register(api, huma.Operation{
		Method: http.MethodPost,
		Path:   "/complex/{id}",
	}, func(ctx context.Context, input *ComplexInput) (*ComplexOutput, error) {
		resp := &ComplexOutput{}
		resp.Status = 201
		resp.Body.ID = input.ID
		resp.Body.Result = input.Body
		resp.Body.Message = "Created"
		return resp, nil
	})

	reqBody := `{"string": "Benchmark", "int": 100, "float": 0.1, "bool": true, "time": "2023-01-01T00:00:00Z", "slice": ["a"], "sub": {"id": 1, "name": "SubItem"}, "sub_slice": [{"id": 1, "name": "Item"}], "map": {"k": {"id": 1, "name": "val"}}}`

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest(http.MethodPost, "/complex/123?search=test&page=2", strings.NewReader(reqBody))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		api.Adapter().ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			b.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	}
}
