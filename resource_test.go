package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewResourceOption(t *testing.T) {
	r := NewResource(nil, "/test", PathParam("id", "desc"))

	assert.NotEmpty(t, r.params)
}

func TestResourceCopy(t *testing.T) {
	r1 := NewResource(nil, "/test")
	r2 := r1.Copy()

	assert.NotSame(t, r1.dependencies, r2.dependencies)
	assert.NotSame(t, r1.params, r2.params)
	assert.NotSame(t, r1.responseHeaders, r2.responseHeaders)
	assert.NotSame(t, r1.responses, r2.responses)
}

func TestResourceWithDep(t *testing.T) {
	dep1 := SimpleDependency("dep1")
	dep2 := SimpleDependency("dep2")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(dep1)
	r3 := r1.With(dep2)

	assert.NotEmpty(t, r2.dependencies)
	assert.NotEmpty(t, r3.dependencies)

	assert.NotSame(t, r2.dependencies[0], r3.dependencies[0])
}

func TestResourceWithSecurity(t *testing.T) {
	sec1 := SecurityRef("sec1")
	sec2 := SecurityRef("sec2")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(sec1)
	r3 := r1.With(sec2)

	assert.NotEmpty(t, r2.security)
	assert.NotEmpty(t, r3.security)

	assert.NotSame(t, r2.security[0], r3.security[0])
}

func TestResourceWithParam(t *testing.T) {
	param1 := PathParam("p1", "desc")
	param2 := PathParam("p2", "desc")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(param1)
	r3 := r1.With(param2)

	assert.NotEmpty(t, r2.params)
	assert.NotEmpty(t, r3.params)

	assert.NotSame(t, r2.params[0], r3.params[0])

	assert.Equal(t, "/test/{p1}", r2.Path())
	assert.Equal(t, "/test/{p2}", r3.Path())
}

func TestResourceWithHeader(t *testing.T) {
	header1 := ResponseHeader("h1", "desc")
	header2 := ResponseHeader("h2", "desc")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(header1)
	r3 := r1.With(header2)

	assert.NotEmpty(t, r2.responseHeaders)
	assert.NotEmpty(t, r3.responseHeaders)

	assert.NotSame(t, r2.responseHeaders[0], r3.responseHeaders[0])
}

func TestResourceWithResponse(t *testing.T) {
	resp1 := ResponseText(200, "desc")
	resp2 := ResponseText(201, "desc2")

	r1 := NewResource(nil, "/test")
	r2 := r1.With(resp1)
	r3 := r1.With(resp2)

	assert.NotEmpty(t, r2.responses)
	assert.NotEmpty(t, r3.responses)

	assert.NotSame(t, r2.responses[0], r3.responses[0])
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
	"Head", "List", "Get", "Post", "Put", "Patch", "Delete", "Options",
}

func TestResourceFuncs(outer *testing.T) {
	for _, tt := range resourceFuncsTest {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt), func(t *testing.T) {
			r := NewTestRouter(t)
			res := NewResource(r, "/test")

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
			case "Options":
				f = res.Options
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

func TestResourceAutoJSON(t *testing.T) {
	r := NewTestRouter(t)

	type MyResponse struct{}

	// Registering the handler should not panic
	r.Resource("/test").Get("desc", func() *MyResponse {
		return &MyResponse{}
	})

	assert.Equal(t, http.StatusOK, r.api.Paths["/test"][http.MethodGet].responses[0].StatusCode)
	assert.Equal(t, "application/json", r.api.Paths["/test"][http.MethodGet].responses[0].ContentType)
}

func TestResourceAutoText(t *testing.T) {
	r := NewTestRouter(t)

	// Registering the handler should not panic
	r.Resource("/test").Get("desc", func() string {
		return "Hello, world"
	})

	assert.Equal(t, http.StatusOK, r.api.Paths["/test"][http.MethodGet].responses[0].StatusCode)
	assert.Equal(t, "text/plain", r.api.Paths["/test"][http.MethodGet].responses[0].ContentType)
}

func TestResourceAutoNoContent(t *testing.T) {
	r := NewTestRouter(t)

	// Registering the handler should not panic
	r.Resource("/test").Get("desc", func() bool {
		return true
	})

	assert.Equal(t, http.StatusNoContent, r.api.Paths["/test"][http.MethodGet].responses[0].StatusCode)
	assert.Equal(t, "", r.api.Paths["/test"][http.MethodGet].responses[0].ContentType)
}

func TestResourceGetPathParams(t *testing.T) {
	r := NewTestRouter(t)

	res := r.Resource("/test", PathParam("foo", "desc"), PathParam("bar", "desc"))

	assert.Equal(t, []string{"foo", "bar"}, res.PathParams())
}

func TestResourceUnsafeHandler(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/unsafe").Get("doc", UnsafeHandler(func(inputs ...interface{}) []interface{} {
			return []interface{}{true}
		}))
	})

	assert.NotPanics(t, func() {
		r.Resource("/unsafe",
			Response(http.StatusNoContent, "doc"),
		).Get("doc", UnsafeHandler(func(inputs ...interface{}) []interface{} {
			return []interface{}{true}
		}))
	})
}
