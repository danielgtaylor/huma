package huma

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type GreetingInput struct {
	ID string `path:"id"`
}

type TestInputSub struct {
	Num int `json:"num" minimum:"1"`
}

type TestInput struct {
	Name string       `json:"name" minLength:"1"`
	Sub  TestInputSub `json:"sub"`
}

type RecursiveInput struct {
	Value *RecursiveInput
}

func TestSchema(t *testing.T) {
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

	s := r.Schema(reflect.TypeOf(GreetingInput{}), false, "")
	// fmt.Printf("%+v\n", s)
	assert.Equal(t, "object", s.Type)
	assert.Equal(t, 1, len(s.Properties))
	assert.Equal(t, "string", s.Properties["ID"].Type)

	r.Schema(reflect.TypeOf(RecursiveInput{}), false, "")

	s2 := r.Schema(reflect.TypeOf(TestInput{}), false, "")
	pb := NewPathBuffer(make([]byte, 0, 128), 0)
	res := ValidateResult{}
	Validate(r, s2, pb, ModeReadFromServer, map[string]any{
		"name": "foo",
		"sub": map[string]any{
			"num": 1.0,
		},
	}, &res)
	assert.Empty(t, res.Errors)

	// b, _ := json.MarshalIndent(r.Map(), "", "  ")
	// fmt.Println(string(b))
}

/*
BenchmarkSchemaSB-10         	 7117623	       154.7 ns/op	     128 B/op	       1 allocs/op
BenchmarkSchemaBuffer-10    	 9186157	       130.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkPathBuf-10         	 9646777	       109.7 ns/op	       0 B/op	       0 allocs/op
*/

type BenchSub struct {
	Visible bool      `json:"visible" default:"true"`
	Metrics []float64 `json:"metrics" maxItems:"31"`
}

type BenchStruct struct {
	Name   string    `json:"name" minLength:"1"`
	Code   string    `json:"code" pattern:"^[a-z]{3}-[0-9]+$"`
	Count  uint      `json:"count" maximum:"10"`
	Rating float32   `json:"rating" minimum:"0" maximum:"5"`
	Region string    `json:"region,omitempty" enum:"east,west"`
	Labels []string  `json:"labels,omitempty" maxItems:"5" uniqueItems:"true"`
	Sub    *BenchSub `json:"sub,omitempty"`
}

func BenchmarkSchema(b *testing.B) {
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

	s2 := r.Schema(reflect.TypeOf(BenchStruct{}), false, "")

	// data, _ := json.MarshalIndent(r.Map(), "", "  ")
	// fmt.Println(string(data))

	input := map[string]interface{}{
		"name":   "foo",
		"code":   "bar-123",
		"count":  8,
		"rating": 3.5,
		"region": "west",
		"labels": []any{"a", "b"},
		"sub": map[string]any{
			"visible": true,
			"metrics": []any{1.0, 2.0, 3.0},
		},
	}
	pb := NewPathBuffer(make([]byte, 0, 128), 0)
	res := ValidateResult{}
	Validate(r, s2, pb, ModeReadFromServer, input, &res)
	assert.Empty(b, res.Errors)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pb.Reset()
		res.Reset()
		Validate(r, s2, pb, ModeReadFromServer, input, &res)
		if len(res.Errors) > 0 {
			b.Fatal(res.Errors)
		}
	}
}

func BenchmarkSchemaErrors(b *testing.B) {
	r := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)

	s2 := r.Schema(reflect.TypeOf(BenchStruct{}), false, "")

	input := map[string]any{
		"name":   true,
		"code":   "wrong",
		"count":  20,
		"rating": 5.5,
		"region": "error",
		"labels": []any{"dupe", "dupe"},
		"sub": map[string]any{
			"visible":    1,
			"unexpected": 2,
		},
	}
	pb := NewPathBuffer(make([]byte, 0, 128), 0)
	res := ValidateResult{}
	Validate(r, s2, pb, ModeReadFromServer, input, &res)
	assert.NotEmpty(b, res.Errors)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pb.Reset()
		res.Reset()
		Validate(r, s2, pb, ModeReadFromServer, input, &res)
	}
}
