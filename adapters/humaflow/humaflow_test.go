package humaflow

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaflow/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// See https://github.com/danielgtaylor/huma/issues/859
func TestWithValueShouldPropagateContext(t *testing.T) {
	r := flow.New()
	app := New(r, huma.DefaultConfig("Test", "1.0.0"))

	type (
		testInput  struct{}
		testOutput struct{}
		ctxKey     struct{}
	)

	ctxValue := "sentinelValue"

	huma.Register(app, huma.Operation{
		OperationID: "test",
		Path:        "/test",
		Method:      http.MethodGet,
		Middlewares: huma.Middlewares{
			func(ctx huma.Context, next func(huma.Context)) {
				ctx = huma.WithValue(ctx, ctxKey{}, ctxValue)
				next(ctx)
			},
			middleware(func(h http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					val, _ := r.Context().Value(ctxKey{}).(string)
					io.WriteString(w, val)
				})
			}),
		},
	}, func(ctx context.Context, input *testInput) (*testOutput, error) {
		out := &testOutput{}
		return out, nil
	})

	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	out, err := io.ReadAll(w.Body)
	require.NoError(t, err)
	assert.Equal(t, ctxValue, string(out))
}

func middleware(mw func(http.Handler) http.Handler) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, w := Unwrap(ctx)
		mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx = NewContext(ctx.Operation(), r, w)
			next(ctx)
		})).ServeHTTP(w, r)
	}
}
