package responses

import (
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/istreamlabs/huma"
	"github.com/stretchr/testify/assert"
)

var funcs = struct {
	Responses []func() huma.Response
}{
	Responses: []func() huma.Response{
		OK,
		Created,
		Accepted,
		NoContent,
		MovedPermanently,
		Found,
		NotModified,
		TemporaryRedirect,
		PermanentRedirect,
		BadRequest,
		Unauthorized,
		Forbidden,
		NotFound,
		RequestTimeout,
		Conflict,
		PreconditionFailed,
		RequestEntityTooLarge,
		PreconditionRequired,
		InternalServerError,
		BadGateway,
		ServiceUnavailable,
		GatewayTimeout,
	},
}

func TestResponses(t *testing.T) {
	var status int
	response = func(s int) huma.Response {
		status = s
		return newResponse(s)
	}

	table := map[string]int{}
	for _, s := range []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNoContent,
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusNotModified,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusPreconditionRequired,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		table[strings.Replace(http.StatusText(s), " ", "", -1)] = s
	}

	for _, f := range funcs.Responses {
		parts := strings.Split(runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name(), ".")
		name := parts[len(parts)-1]
		t.Run(name, func(t *testing.T) {
			f()

			// The response we created has the right status code given the creation
			// func name.
			assert.Equal(t, table[name], status)
		})
	}

	String(http.StatusOK)
	assert.Equal(t, 200, status)
}
