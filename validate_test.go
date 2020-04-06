package huma

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTitleRequired(t *testing.T) {
	assert.Panics(t, func() {
		_ = NewRouter(&OpenAPI{})
	})
}

func TestVersionRequired(t *testing.T) {
	assert.Panics(t, func() {
		_ = NewRouter(&OpenAPI{
			Title: "Version Required",
		})
	})
}

func TestOperationDescriptionRequired(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{})
	})
}

func TestOperationResponseRequired(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
		})
	})
}

func TestOperationHandlerMissing(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
		})
	})
}

func TestOperationHandlerInput(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	d := &Dependency{
		Value: func() (string, error) {
			return "test", nil
		},
	}

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description:  "Test",
			Dependencies: []*Dependency{d},
			Params: []*Param{
				QueryParam("foo", "Test", ""),
			},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func() string {
				// Wrong number of inputs!
				return "fails"
			},
		})
	})
}

func TestOperationHandlerOutput(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			ResponseHeaders: []*ResponseHeader{
				Header("x-test", "Test"),
			},
			Responses: []*Response{
				ResponseText(200, "Test", "x-test"),
			},
			Handler: func() string {
				// Wrong number of outputs!
				return "fails"
			},
		})
	})
}

func TestOperationListAutoID(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	o := &Operation{
		Description: "Test",
		Responses: []*Response{
			ResponseJSON(200, "Test"),
		},
		Handler: func() []string {
			return []string{"test"}
		},
	}

	r.Register(http.MethodGet, "/items", o)

	assert.Equal(t, "list-items", o.ID)
}

func TestOperationContextPointer(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Dependencies: []*Dependency{
				ContextDependency(),
			},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func(c gin.Context) string {
				return "test"
			},
		})
	})
}

func TestOperationOperationPointer(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Dependencies: []*Dependency{
				OperationDependency(),
			},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func(o Operation) string {
				return "test"
			},
		})
	})
}

func TestOperationInvalidDep(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Dependencies: []*Dependency{
				&Dependency{},
			},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func(string) string {
				return "test"
			},
		})
	})
}

func TestOperationParamDep(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Params: []*Param{
				QueryParam("foo", "Test", ""),
			},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func(c *gin.Context) string {
				return "test"
			},
		})
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Params: []*Param{
				QueryParam("foo", "Test", ""),
			},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func(o *Operation) string {
				return "test"
			},
		})
	})
}

func TestOperationParamRedeclare(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	p := QueryParam("foo", "Test", 0)

	r.Register(http.MethodGet, "/", &Operation{
		Description: "Test",
		Params:      []*Param{p},
		Responses: []*Response{
			ResponseText(200, "Test"),
		},
		Handler: func(p int) string {
			return "test"
		},
	})

	// Param p was declared as `int` above but is `string` here.
	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Params:      []*Param{p},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func(p string) string {
				return "test"
			},
		})
	})
}

func TestOperationParamExampleType(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	assert.Panics(t, func() {
		r.Register(http.MethodGet, "/", &Operation{
			Description: "Test",
			Params: []*Param{
				QueryParamExample("foo", "Test", "", 123),
			},
			Responses: []*Response{
				ResponseText(200, "Test"),
			},
			Handler: func(p string) string {
				return "test"
			},
		})
	})
}

func TestOperationParamExampleSchema(t *testing.T) {
	r := NewRouter(&OpenAPI{
		Title:   "Test API",
		Version: "1.0.0",
	})

	p := QueryParamExample("foo", "Test", 0, 123)

	r.Register(http.MethodGet, "/", &Operation{
		Description: "Test",
		Params: []*Param{
			p,
		},
		Responses: []*Response{
			ResponseText(200, "Test"),
		},
		Handler: func(p int) string {
			return "test"
		},
	})

	assert.Equal(t, 123, p.Schema.Example)
}
