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
	res.
		With(
			PathParam("q", "desc"),
			ResponseText(http.StatusOK, "desc")).
		Get("desc",
			func(q string) string {
				return q
			},
		)

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
			res := NewResource(r, "/test").Text(http.StatusOK, "desc")

			var f func(string, interface{})

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
			f("desc", func() bool {
				return true
			})
		})
	}
}

var resourceShorthandFuncs = []struct {
	n           string
	statusCode  int
	contentType string
	desc        string
}{
	{"Text", http.StatusOK, "text/plain", "desc"},
	{"JSON", http.StatusOK, "application/json", "desc"},
	{"NoContent", http.StatusNoContent, "", "desc"},
	{"Empty", http.StatusNotModified, "", "desc"},
}

func TestResourceShorthandFuncs(outer *testing.T) {
	for _, tt := range resourceShorthandFuncs {
		local := tt
		outer.Run(fmt.Sprintf("%v", local.n), func(t *testing.T) {
			r := NewRouter(&OpenAPI{Title: "Test API", Version: "1.0.0"})
			res := NewResource(r, "/test")

			switch local.n {
			case "Text":
				res = res.Text(local.statusCode, local.desc, "header")
			case "JSON":
				res = res.JSON(local.statusCode, local.desc, "header")
			case "NoContent":
				res = res.NoContent(local.desc, "header")
			case "Empty":
				res = res.Empty(local.statusCode, local.desc, "header")
			default:
				panic("invalid case " + local.n)
			}

			resp := res.responses[0]
			assert.Equal(t, local.statusCode, resp.StatusCode)
			assert.Equal(t, local.contentType, resp.ContentType)
			assert.Equal(t, local.desc, resp.Description)
		})
	}
}
