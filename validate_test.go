package huma

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestOperationDescriptionRequired(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &OpenAPIOperation{})
	})
}

func TestOperationResponseRequired(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &OpenAPIOperation{
			description: "Test",
		})
	})
}

func TestOperationHandlerInput(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/",
			SimpleDependency("test"),
			ResponseText(200, "Test"),
		).Get("Test", func() string {
			// Wrong number of inputs!
			return "fails"
		})
	})
}

func TestOperationHandlerOutput(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/",
			ResponseHeader("x-test", "Test"),
			ResponseText(200, "Test", Headers("x-test")),
		).Get("Test", func() string {
			// Wrong number of outputs!
			return "fails"
		})
	})
}

func TestOperationListAutoID(t *testing.T) {
	r := NewTestRouter(t)

	r.Resource("/items").Get("Test", func() []string {
		return []string{"test"}
	})

	o := r.OpenAPI().Paths["/items"][http.MethodGet]

	assert.Equal(t, "list-items", o.id)
}

func TestOperationContextPointer(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/",
			GinContextDependency(),
		).Get("Test", func(c gin.Context) string {
			return "test"
		})
	})
}

func TestOperationOperationPointer(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/",
			OperationDependency(),
		).Get("Test", func(o OpenAPIOperation) string {
			return "test"
		})
	})
}

func TestOperationInvalidDep(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/",
			SimpleDependency(nil),
		).Get("Test", func(o OpenAPIOperation) string {
			return "test"
		})
	})
}

func TestOperationParamDep(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/",
			QueryParam("foo", "Test", ""),
		).Get("Test", func(c *gin.Context) string {
			return "test"
		})
	})

	assert.Panics(t, func() {
		r.Resource("/",
			QueryParam("foo", "Test", ""),
		).Get("Test", func(c *OpenAPIOperation) string {
			return "test"
		})
	})
}

func TestOperationParamRedeclare(t *testing.T) {
	r := NewTestRouter(t)

	param := QueryParam("foo", "Test", 0)

	r.Resource("/a", param).Get("Test", func(p int) string { return "a" })

	//  Redeclare param `p` as a string while it was an int above.
	assert.Panics(t, func() {
		r.Resource("/b", param).Get("Test", func(p string) string { return "b" })
	})
}

func TestOperationParamExampleType(t *testing.T) {
	r := NewTestRouter(t)

	assert.Panics(t, func() {
		r.Resource("/",
			QueryParam("foo", "Test", "", Example(123)),
		).Get("Test", func(p string) string {
			return "test"
		})
	})
}

func TestOperationParamExampleSchema(t *testing.T) {
	r := NewTestRouter(t)

	p := QueryParam("foo", "Test", 0, Example(123))

	r.Resource("/", p).Get("Test", func(p int) string {
		return "test"
	})

	param := r.OpenAPI().Paths["/"][http.MethodGet].params[0]

	assert.Equal(t, 123, param.Schema.Example)
}
