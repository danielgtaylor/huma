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
	d := Dependency{}

	typ := reflect.TypeOf(123)

	assert.Error(t, d.validate(typ))
}

func TestGlobalDepWrongType(t *testing.T) {
	d := Dependency{
		Value: "test",
	}

	typ := reflect.TypeOf(123)

	assert.Error(t, d.validate(typ))
}

func TestGlobalDepParams(t *testing.T) {
	d := Dependency{
		Params: []*Param{
			HeaderParam("foo", "description", "hello"),
		},
		Value: "test",
	}

	typ := reflect.TypeOf("test")

	assert.Error(t, d.validate(typ))
}

func TestGlobalDepHeaders(t *testing.T) {
	d := Dependency{
		ResponseHeaders: []*ResponseHeader{Header("foo", "description")},
		Value:           "test",
	}

	typ := reflect.TypeOf("test")

	assert.Error(t, d.validate(typ))
}

func TestDepContext(t *testing.T) {
	d := Dependency{
		Dependencies: []*Dependency{
			ContextDependency(),
		},
		Value: func(ctx context.Context) (context.Context, error) { return ctx, nil },
	}

	mock, _ := gin.CreateTestContext(nil)
	mock.Request = httptest.NewRequest("GET", "/", nil)

	typ := reflect.TypeOf(mock)
	assert.NoError(t, d.validate(typ))

	_, v, err := d.Resolve(mock, &Operation{})
	assert.NoError(t, err)
	assert.Equal(t, v, mock.Request.Context())
}

func TestDepGinContext(t *testing.T) {
	d := Dependency{
		Dependencies: []*Dependency{
			GinContextDependency(),
		},
		Value: func(c *gin.Context) (*gin.Context, error) { return c, nil },
	}

	mock, _ := gin.CreateTestContext(nil)

	typ := reflect.TypeOf(mock)
	assert.NoError(t, d.validate(typ))

	_, v, err := d.Resolve(mock, &Operation{})
	assert.NoError(t, err)
	assert.Equal(t, v, mock)
}

func TestDepOperation(t *testing.T) {
	d := Dependency{
		Dependencies: []*Dependency{
			OperationDependency(),
		},
		Value: func(o *Operation) (*Operation, error) { return o, nil },
	}

	mock := &Operation{}

	typ := reflect.TypeOf(mock)
	assert.NoError(t, d.validate(typ))

	_, v, err := d.Resolve(&gin.Context{}, mock)
	assert.NoError(t, err)
	assert.Equal(t, v, mock)
}
func TestDepFuncWrongArgs(t *testing.T) {
	d := Dependency{
		Params: []*Param{
			HeaderParam("foo", "desc", ""),
		},
		Value: func() (string, error) {
			return "", nil
		},
	}

	assert.Error(t, d.validate(reflect.TypeOf("")))
}

func TestDepFunc(t *testing.T) {
	d := Dependency{
		Params: []*Param{
			HeaderParam("x-in", "desc", ""),
		},
		ResponseHeaders: []*ResponseHeader{
			Header("x-out", "desc"),
		},
		Value: func(xin string) (string, string, error) {
			return "xout", "value", nil
		},
	}

	c := &gin.Context{
		Request: &http.Request{
			Header: http.Header{
				"x-in": []string{"xin"},
			},
		},
	}

	assert.NoError(t, d.validate(reflect.TypeOf("")))
	h, v, err := d.Resolve(c, &Operation{})
	assert.NoError(t, err)
	assert.Equal(t, "xout", h["x-out"])
	assert.Equal(t, "value", v)
}
