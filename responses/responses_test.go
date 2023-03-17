package responses

import (
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/danielgtaylor/huma"
)

var funcs = struct {
	Responses []func() huma.Response
}{
	Responses: []func() huma.Response{
		OK,
		Created,
		Accepted,
		NoContent,
		PartialContent,
		MovedPermanently,
		Found,
		NotModified,
		TemporaryRedirect,
		PermanentRedirect,
		BadRequest,
		Unauthorized,
		Forbidden,
		NotFound,
		NotAcceptable,
		RequestTimeout,
		Conflict,
		PreconditionFailed,
		UnsupportedMediaType,
		RequestEntityTooLarge,
		UnprocessableEntity,
		PreconditionRequired,
		ClientClosedRequest,
		InternalServerError,
		NotImplemented,
		BadGateway,
		ServiceUnavailable,
		GatewayTimeout,
	},
}

func TestNesResponses(t *testing.T) {
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
		http.StatusPartialContent,
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusNotModified,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusNotAcceptable,
		http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity,
		http.StatusPreconditionRequired,
		// 499 not yet supported by net/http lib
		499,
		http.StatusInternalServerError,
		http.StatusNotImplemented,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		if s == 499 {
			table["ClientClosedRequest"] = s
		} else {
			table[strings.ReplaceAll(http.StatusText(s), " ", "")] = s
		}
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

func contains(r []huma.Response, e huma.Response) bool {
	for _, i := range r {
		if i.GetStatus() == e.GetStatus() {
			return true
		}
	}
	return false
}

func TestWriteContentResponses(t *testing.T) {
	r := WriteContent()

	assert.Equal(t, 5, len(r))
	assert.True(t, contains(r, OK()))
	assert.True(t, contains(r, PartialContent()))
	assert.True(t, contains(r, NotModified()))
	assert.True(t, contains(r, PreconditionFailed()))
	assert.True(t, contains(r, InternalServerError()))
}
