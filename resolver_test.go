package huma

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExhaustiveErrors(t *testing.T) {
	type Input struct {
		BoolParam    bool      `query:"bool"`
		IntParam     int       `query:"int"`
		Float32Param float32   `query:"float32"`
		Float64Param float64   `query:"float64"`
		Tags         []int     `query:"tags"`
		Time         time.Time `query:"time"`
		Body         struct {
			Value int `json:"value" minimum:"5"`
		}
	}

	app := newTestRouter()

	app.Resource("/").Get("test", "Test").Run(func(ctx Context, input Input) {
		// Do nothing
	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/?bool=bad&int=bad&float32=bad&float64=bad&tags=1,2,bad&time=bad", strings.NewReader(`{"value": 1}`))
	app.ServeHTTP(w, r)

	assert.JSONEq(t, `{"title":"Bad Request","status":400,"detail":"Error while parsing input parameters","errors":[{"message":"cannot parse boolean","location":"query.bool","value":"bad"},{"message":"cannot parse integer","location":"query.int","value":"bad"},{"message":"cannot parse float","location":"query.float32","value":"bad"},{"message":"cannot parse float","location":"query.float64","value":"bad"},{"message":"cannot parse integer","location":"query[2].tags","value":"bad"},{"message":"unable to validate against schema: invalid character 'b' looking for beginning of value","location":"query.tags","value":"[1,2,bad]"},{"message":"cannot parse time","location":"query.time","value":"bad"},{"message":"Must be greater than or equal to 5","location":"body.value","value":1}]}`, w.Body.String())
}
