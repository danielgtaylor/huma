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
	d := openAPIDependency{}

	typ := reflect.TypeOf(123)

	assert.Panics(t, func() {
		d.validate(typ)
	})
}

func TestGlobalDepWrongType(t *testing.T) {
	d := openAPIDependency{
		handler: "test",
	}

	typ := reflect.TypeOf(123)

	assert.Panics(t, func() {
		d.validate(typ)
	})
}

func TestDepContext(t *testing.T) {
	d := openAPIDependency{
		dependencies: []*openAPIDependency{
			&contextDependency,
		},
		handler: func(ctx context.Context) (context.Context, error) { return ctx, nil },
	}

	mock, _ := gin.CreateTestContext(nil)
	mock.Request = httptest.NewRequest("GET", "/", nil)

	typ := reflect.TypeOf(mock)
	d.validate(typ)

	_, v, err := d.resolve(mock, &openAPIOperation{})
	assert.NoError(t, err)
	assert.Equal(t, v, mock.Request.Context())
}

func TestDepGinContext(t *testing.T) {
	d := openAPIDependency{
		dependencies: []*openAPIDependency{
			&ginContextDependency,
		},
		handler: func(c *gin.Context) (*gin.Context, error) { return c, nil },
	}

	mock, _ := gin.CreateTestContext(nil)

	typ := reflect.TypeOf(mock)
	d.validate(typ)

	_, v, err := d.resolve(mock, &openAPIOperation{})
	assert.NoError(t, err)
	assert.Equal(t, v, mock)
}

func TestDepOperationID(t *testing.T) {
	d := openAPIDependency{
		dependencies: []*openAPIDependency{
			&operationIDDependency,
		},
		handler: func(id string) (string, error) { return id, nil },
	}

	mock := &openAPIOperation{
		id: "test-id",
	}

	typ := reflect.TypeOf(mock)
	d.validate(typ)

	_, v, err := d.resolve(&gin.Context{}, mock)
	assert.NoError(t, err)
	assert.Equal(t, v, "test-id")
}
func TestDepFuncWrongArgs(t *testing.T) {
	d := &openAPIDependency{}

	Dependency(HeaderParam("foo", "desc", ""), func() (string, error) {
		return "", nil
	}).applyDependency(d)

	assert.Panics(t, func() {
		d.validate(reflect.TypeOf(""))
	})
}

func TestDepFunc(t *testing.T) {
	d := openAPIDependency{
		handler: func(xin string) (string, string, error) {
			return "xout", "value", nil
		},
	}

	DependencyOptions(
		HeaderParam("x-in", "desc", ""),
		ResponseHeader("x-out", "desc"),
	).applyDependency(&d)

	c := &gin.Context{
		Request: &http.Request{
			Header: http.Header{
				"x-in": []string{"xin"},
			},
		},
	}

	d.validate(reflect.TypeOf(""))
	h, v, err := d.resolve(c, &openAPIOperation{})
	assert.NoError(t, err)
	assert.Equal(t, "xout", h["x-out"])
	assert.Equal(t, "value", v)
}
