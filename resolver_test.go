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

type Dep1 struct {
	Value    string `json:"value"`
	Computed string `json:"-"`
}

func (d *Dep1) Resolve(ctx Context, r *http.Request) {
	if d.Value == "error" {
		ctx.AddError(&ErrorDetail{
			Message:  "msg",
			Location: "value",
			Value:    d.Value,
		})
	}

	d.Computed = strings.ToUpper(d.Value)
}

type Dep2 struct {
	Foo map[string][]Dep1 `json:"foo"`
}

func TestNestedResolver(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Post("test", "Test",
		NewResponse(http.StatusOK, "desc").ContentType("text/plain"),
	).Run(func(ctx Context, input struct {
		Body Dep2
	}) {
		computed := []string{}

		for _, v := range input.Body.Foo {
			for _, dep1 := range v {
				computed = append(computed, dep1.Computed)
			}
		}

		ctx.WriteHeader(http.StatusOK)
		ctx.Write([]byte(strings.Join(computed, ", ")))
	})

	// Test happy case
	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"foo": {"one": [{"value": "v1"}], "two": [{"value": "v2"}]}}`))
	app.ServeHTTP(w, r)

	assert.Equal(t, "V1, V2", w.Body.String())

	// Test error case
	w = httptest.NewRecorder()
	r, _ = http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"foo": {"one": [{"value": "error"}]}}`))
	app.ServeHTTP(w, r)

	assert.JSONEq(t, `{
		"status": 400,
		"title": "Bad Request",
		"detail": "Error while parsing input parameters",
		"errors": [
			{
				"message": "msg",
				"location": "body.foo.one[0].value",
				"value": "error"
			}
		]
	}`, w.Body.String())
}
