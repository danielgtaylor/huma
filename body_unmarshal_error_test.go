package huma_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

// failingTextUnmarshaler is a custom type whose `encoding.TextUnmarshaler`
// implementation fails when the input is not a valid integer.
type failingTextUnmarshaler int

func (c *failingTextUnmarshaler) UnmarshalText(text []byte) error {
	value, err := strconv.Atoi(string(text))
	if err != nil {
		return err
	}
	*c = failingTextUnmarshaler(value)
	return nil
}

// When a custom type in the request body fails to unmarshal, the error should
// point at the field that failed to decode and include that field's value,
// not report `body` as the location with the entire request body as the
// value. See https://github.com/danielgtaylor/huma/issues/1109.
func TestBodyCustomTypeUnmarshalErrorLocation(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPost,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		Body struct {
			Amount       int                    `json:"amount"`
			CustomAmount failingTextUnmarshaler `json:"customAmount"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"amount": 1, "customAmount": "a"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	var body huma.ErrorModel
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	found := false
	for _, e := range body.Errors {
		if e.Location == "body.customAmount" && e.Value == "a" {
			found = true
			break
		}
	}
	require.True(t, found, "expected error at body.customAmount with value \"a\"; got: %v", body.Errors)
}

// dualUnmarshaler implements both json.Unmarshaler and
// encoding.TextUnmarshaler. encoding/json prefers UnmarshalJSON and never
// calls UnmarshalText, so validation must not probe UnmarshalText either.
type dualUnmarshaler int

func (d *dualUnmarshaler) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*d = dualUnmarshaler(len(s))
	return nil
}

func (d *dualUnmarshaler) UnmarshalText(text []byte) error {
	return strconv.ErrSyntax
}

func TestBodyDualUnmarshalerNotProbed(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
		OperationID: "test-dual",
		Method:      http.MethodPost,
		Path:        "/test-dual",
	}, func(ctx context.Context, input *struct {
		Body struct {
			V dualUnmarshaler `json:"v"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})

	// UnmarshalJSON accepts any string; UnmarshalText always fails. The
	// request must succeed because encoding/json only calls UnmarshalJSON.
	req, _ := http.NewRequest(http.MethodPost, "/test-dual", strings.NewReader(`{"v": "hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

// textIfaceField is an interface type that includes UnmarshalText. A field
// declared with an interface type must not panic the validation probe
// (reflect.New of an interface type does not implement the interface).
type textIfaceField interface {
	UnmarshalText(text []byte) error
}

func TestBodyInterfaceTextUnmarshalerFieldNoPanic(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
		OperationID: "test-iface",
		Method:      http.MethodPost,
		Path:        "/test-iface",
	}, func(ctx context.Context, input *struct {
		Body struct {
			V textIfaceField `json:"v"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})

	req, _ := http.NewRequest(http.MethodPost, "/test-iface", strings.NewReader(`{"v": "x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// Upstream behavior: decoding into an interface fails with a 422; the
	// important property is that validation does not panic.
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}
