package huma

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGlobalDepEmpty(t *testing.T) {
	d := OpenAPIDependency{}

	typ := reflect.TypeOf(123)

	assert.Panics(t, func() {
		d.validate(typ)
	})
}

func TestGlobalDepWrongType(t *testing.T) {
	d := OpenAPIDependency{
		handler: "test",
	}

	typ := reflect.TypeOf(123)

	assert.Panics(t, func() {
		d.validate(typ)
	})
}

func TestGlobalDepParams(t *testing.T) {
	d := OpenAPIDependency{
		handler: "test",
	}

	HeaderParam("foo", "description", "hello").ApplyDependency(&d)

	typ := reflect.TypeOf("test")

	assert.Panics(t, func() {
		d.validate(typ)
	})
}

func TestGlobalDepHeaders(t *testing.T) {
	d := OpenAPIDependency{
		handler: "test",
	}

	ResponseHeader("foo", "description").ApplyDependency(&d)

	typ := reflect.TypeOf("test")

	assert.Panics(t, func() {
		d.validate(typ)
	})
}

func TestDepContext(t *testing.T) {
	d := OpenAPIDependency{
		dependencies: []*OpenAPIDependency{
			&contextDependency,
		},
		handler: func(ctx context.Context) (context.Context, error) { return ctx, nil },
	}

	mock, _ := gin.CreateTestContext(nil)
	mock.Request = httptest.NewRequest("GET", "/", nil)

	typ := reflect.TypeOf(mock)
	d.validate(typ)

	_, v, err := d.resolve(mock, &OpenAPIOperation{})
	assert.NoError(t, err)
	assert.Equal(t, v, mock.Request.Context())
}

func TestDepGinContext(t *testing.T) {
	d := OpenAPIDependency{
		dependencies: []*OpenAPIDependency{
			&ginContextDependency,
		},
		handler: func(c *gin.Context) (*gin.Context, error) { return c, nil },
	}

	mock, _ := gin.CreateTestContext(nil)

	typ := reflect.TypeOf(mock)
	d.validate(typ)

	_, v, err := d.resolve(mock, &OpenAPIOperation{})
	assert.NoError(t, err)
	assert.Equal(t, v, mock)
}

func TestDepOperation(t *testing.T) {
	d := OpenAPIDependency{
		dependencies: []*OpenAPIDependency{
			&operationDependency,
		},
		handler: func(o *OpenAPIOperation) (*OpenAPIOperation, error) { return o, nil },
	}

	mock := &OpenAPIOperation{}

	typ := reflect.TypeOf(mock)
	d.validate(typ)

	_, v, err := d.resolve(&gin.Context{}, mock)
	assert.NoError(t, err)
	assert.Equal(t, v, mock)
}
func TestDepFuncWrongArgs(t *testing.T) {
	d := OpenAPIDependency{
		handler: func() (string, error) {
			return "", nil
		},
	}

	HeaderParam("foo", "desc", "").ApplyDependency(&d)

	assert.Panics(t, func() {
		d.validate(reflect.TypeOf(""))
	})
}

func TestDepFunc(t *testing.T) {
	d := OpenAPIDependency{
		handler: func(xin string) (string, string, error) {
			return "xout", "value", nil
		},
	}

	DependencyOptions(
		HeaderParam("x-in", "desc", ""),
		ResponseHeader("x-out", "desc"),
	).ApplyDependency(&d)

	c := &gin.Context{
		Request: &http.Request{
			Header: http.Header{
				"x-in": []string{"xin"},
			},
		},
	}

	d.validate(reflect.TypeOf(""))
	h, v, err := d.resolve(c, &OpenAPIOperation{})
	assert.NoError(t, err)
	assert.Equal(t, "xout", h["x-out"])
	assert.Equal(t, "value", v)
}
