package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceCopy(t *testing.T) {
	r1 := NewResource(nil, "/test")
	r2 := r1.Copy()

	assert.NotSame(t, r1.deps, r2.deps)
	assert.NotSame(t, r1.params, r2.params)
	assert.NotSame(t, r1.responseHeaders, r2.responseHeaders)
	assert.NotSame(t, r1.responses, r2.responses)
}

func TestResourceWithBadInput(t *testing.T) {
	assert.Panics(t, func() {
		NewResource(nil, "/test").With("bad-value")
	})
}

func TestResourceWithDep(t *testing.T) {
	dep1 := &Dependency{Value: "dep1"}
	dep2 := &Dependency{Value: "dep2"}

	r1 := NewResource(nil, "/test")
	r2 := r1.With(dep1)
	r3 := r1.With(dep2)

	assert.Contains(t, r2.deps, dep1)
	assert.NotContains(t, r2.deps, dep2)
	assert.Contains(t, r3.deps, dep2)
	assert.NotContains(t, r3.deps, dep1)
}

func TestResourceWithParam(t *testing.T) {
	param1 := PathParam("p1", "desc")
	param2 := PathParam("p2", "desc")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(param1)
	r3 := r1.With(param2)

	assert.Contains(t, r2.params, param1)
	assert.NotContains(t, r2.params, param2)
	assert.Contains(t, r3.params, param2)
	assert.NotContains(t, r3.params, param1)

	assert.Equal(t, "/test/{p1}", r2.Path())
	assert.Equal(t, "/test/{p2}", r3.Path())
}

func TestResourceWithHeader(t *testing.T) {
	header1 := Header("h1", "desc")
	header2 := Header("h2", "desc")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(header1)
	r3 := r1.With(header2)

	assert.Contains(t, r2.responseHeaders, header1)
	assert.NotContains(t, r2.responseHeaders, header2)
	assert.Contains(t, r3.responseHeaders, header2)
	assert.NotContains(t, r3.responseHeaders, header1)
}

func TestResourceWithResponse(t *testing.T) {
	resp1 := ResponseText(200, "desc")
	resp2 := ResponseText(201, "desc2")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(resp1)
	r3 := r1.With(resp2)

	assert.Contains(t, r2.responses, resp1)
	assert.NotContains(t, r2.responses, resp2)
	assert.Contains(t, r3.responses, resp2)
	assert.NotContains(t, r3.responses, resp1)
}

func TestSubResource(t *testing.T) {
	r1 := NewResource(nil, "/tests").With(PathParam("testId", "desc"))
	r2 := r1.SubResource("/results", PathParam("resultId", "desc"))
	r3 := r2.With(PathParam("format", "desc"))

	assert.Equal(t, "/tests/{testId}", r1.Path())
	assert.Equal(t, "/tests/{testId}/results/{resultId}", r2.Path())
	assert.Equal(t, "/tests/{testId}/results/{resultId}/{format}", r3.Path())
}

func TestResourceWithAddedParam(t *testing.T) {
	r := NewTestRouter(t)
	res := NewResource(r, "/test")
	res.Get(&Operation{
		Description: "desc",
		Params: []*Param{
			PathParam("q", "desc"),
		},
		Responses: []*Response{
			ResponseText(http.StatusOK, "desc"),
		},
		Handler: func(q string) string {
			return q
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test/hello", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello", w.Body.String())
}

var resourceFuncsTest = []string{
	"Head", "List", "Get", "Post", "Put", "Patch", "Delete",
}

func TestResourceFuncs(outer *testing.T) {
	for _, tt := range resourceFuncsTest {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt), func(t *testing.T) {
			r := NewRouter(&OpenAPI{Title: "Test API", Version: "1.0.0"})
			res := NewResource(r, "/test")

			var f func(*Operation)

			switch local {
			case "Head":
				f = res.Head
			case "List":
				f = res.List
			case "Get":
				f = res.Get
			case "Post":
				f = res.Post
			case "Put":
				f = res.Put
			case "Patch":
				f = res.Patch
			case "Delete":
				f = res.Delete
			default:
				panic("invalid case " + local)
			}

			// Registering it should not panic.
			f(&Operation{
				Description: local + " test",
				Responses: []*Response{
					ResponseEmpty(http.StatusOK, "desc"),
				},
				Handler: func() bool {
					return true
				},
			})
		})
	}
}

var resourceShortcutsTest = []string{
	"ListJSON", "GetJSON", "PostJSON", "PostNoContent", "PutJSON", "PutNoContent", "PatchJSON", "PatchNoContent", "DeleteNoContent",
}

func TestResourceShortcuts(outer *testing.T) {
	for _, tt := range resourceShortcutsTest {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt), func(t *testing.T) {
			r := NewRouter(&OpenAPI{Title: "Test API", Version: "1.0.0"})
			res := NewResource(r, "/test")

			var f func(int, string, interface{})

			switch local {
			case "ListJSON":
				f = res.ListJSON
			case "GetJSON":
				f = res.GetJSON
			case "PostJSON":
				f = res.PostJSON
			case "PostNoContent":
				f = res.PostNoContent
			case "PutJSON":
				f = res.PutJSON
			case "PutNoContent":
				f = res.PutNoContent
			case "PatchJSON":
				f = res.PatchJSON
			case "PatchNoContent":
				f = res.PatchNoContent
			case "DeleteNoContent":
				f = res.DeleteNoContent
			default:
				panic("invalid case " + local)
			}

			// Registering it should not panic.
			f(200, "desc", func() bool {
				return true
			})
		})
	}
}
