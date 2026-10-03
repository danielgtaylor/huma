package conditional

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasConditional(t *testing.T) {
	p := Params{}
	assert.False(t, p.HasConditionalParams())

	p = Params{IfMatch: []string{"test"}}
	assert.True(t, p.HasConditionalParams())

	p = Params{IfNoneMatch: []string{"test"}}
	assert.True(t, p.HasConditionalParams())

	p = Params{IfModifiedSince: time.Now()}
	assert.True(t, p.HasConditionalParams())

	p = Params{IfUnmodifiedSince: time.Now()}
	assert.True(t, p.HasConditionalParams())
}

func TestIfMatch(t *testing.T) {
	p := Params{}

	// Read request
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/resource", nil)
	w := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, r, w)

	p.IfMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx)
	require.NoError(t, p.PreconditionFailed("abc123", time.Time{}))
	require.NoError(t, p.PreconditionFailed("def456", time.Time{}))

	err := p.PreconditionFailed("bad", time.Time{})
	require.Error(t, err)
	assert.Equal(t, http.StatusNotModified, err.GetStatus())

	err = p.PreconditionFailed("", time.Time{})
	require.Error(t, err)
	assert.Equal(t, http.StatusNotModified, err.GetStatus())

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = humatest.NewContext(nil, r, w)

	p.IfMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx)
	require.NoError(t, p.PreconditionFailed("abc123", time.Time{}))

	err = p.PreconditionFailed("bad", time.Time{})
	require.Error(t, err)
	assert.Equal(t, http.StatusPreconditionFailed, err.GetStatus())
}

func TestIfNoneMatch(t *testing.T) {
	p := Params{}

	// Read request
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/resource", nil)
	w := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, r, w)

	p.IfNoneMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx)
	require.NoError(t, p.PreconditionFailed("bad", time.Time{}))
	require.NoError(t, p.PreconditionFailed("", time.Time{}))

	err := p.PreconditionFailed("abc123", time.Time{})
	require.Error(t, err)
	assert.Equal(t, http.StatusNotModified, err.GetStatus())

	err = p.PreconditionFailed("def456", time.Time{})
	require.Error(t, err)
	assert.Equal(t, http.StatusNotModified, err.GetStatus())

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = humatest.NewContext(nil, r, w)

	p.IfNoneMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx)
	require.Error(t, p.PreconditionFailed("abc123", time.Time{}))
	require.NoError(t, p.PreconditionFailed("bad", time.Time{}))

	// Write with special `*` syntax to match any.
	p.IfNoneMatch = []string{"*"}
	require.NoError(t, p.PreconditionFailed("", time.Time{}))

	err = p.PreconditionFailed("abc123", time.Time{})
	require.Error(t, err)
	assert.Equal(t, http.StatusPreconditionFailed, err.GetStatus())
}

func TestIfModifiedSince(t *testing.T) {
	p := Params{}

	now, err := time.Parse(time.RFC3339, "2021-01-01T12:00:00Z")
	require.NoError(t, err)

	before, err := time.Parse(time.RFC3339, "2020-01-01T12:00:00Z")
	require.NoError(t, err)

	after, err := time.Parse(time.RFC3339, "2022-01-01T12:00:00Z")
	require.NoError(t, err)

	// Read request
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/resource", nil)
	w := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, r, w)

	p.IfModifiedSince = now

	p.Resolve(ctx)
	require.Error(t, p.PreconditionFailed("", before))
	require.Error(t, p.PreconditionFailed("", now))
	require.NoError(t, p.PreconditionFailed("", after))

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = humatest.NewContext(nil, r, w)

	p.IfModifiedSince = now

	p.Resolve(ctx)
	perr := p.PreconditionFailed("", before)
	require.Error(t, perr)
	assert.Equal(t, http.StatusPreconditionFailed, perr.GetStatus())
}

func TestIfNoneMatchOWSAfterComma(t *testing.T) {
	// RFC 9110 §5.6.1 list grammar permits optional whitespace around each
	// comma, e.g. `If-None-Match: "a", "b"`. Every element after the first
	// must still match (issue #1084).
	_, api := humatest.New(t)
	huma.Register(api, huma.Operation{
		OperationID: "conditional",
		Method:      http.MethodGet,
		Path:        "/resource",
	}, func(ctx context.Context, input *struct {
		Params
	}) (*struct {
		Body string `json:"body"`
	}, error) {
		if err := input.Params.PreconditionFailed("target-tag", time.Time{}); err != nil {
			return nil, err
		}
		return &struct {
			Body string `json:"body"`
		}{Body: "ok"}, nil
	})

	// The OWS lands on the second element; without the fix it never matches
	// and the request returns 200 instead of 304.
	res := api.Get("/resource", `If-None-Match: "other-tag", "target-tag"`)
	assert.Equal(t, http.StatusNotModified, res.Code)

	// First-position elements keep working, with and without whitespace.
	res = api.Get("/resource", `If-None-Match: "target-tag", "other-tag"`)
	assert.Equal(t, http.StatusNotModified, res.Code)

	res = api.Get("/resource", `If-None-Match: "other-tag","target-tag"`)
	assert.Equal(t, http.StatusNotModified, res.Code)

	// A non-matching list still serves the resource.
	res = api.Get("/resource", `If-None-Match: "other-tag", "yet-another-tag"`)
	assert.Equal(t, http.StatusOK, res.Code)
}

func TestIfUnmodifiedSince(t *testing.T) {
	p := Params{}

	now, err := time.Parse(time.RFC3339, "2021-01-01T12:00:00Z")
	require.NoError(t, err)

	before, err := time.Parse(time.RFC3339, "2020-01-01T12:00:00Z")
	require.NoError(t, err)

	after, err := time.Parse(time.RFC3339, "2022-01-01T12:00:00Z")
	require.NoError(t, err)

	// Read request
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/resource", nil)
	w := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, r, w)

	p.IfUnmodifiedSince = now

	p.Resolve(ctx)
	require.NoError(t, p.PreconditionFailed("", before))
	require.NoError(t, p.PreconditionFailed("", now))
	require.Error(t, p.PreconditionFailed("", after))

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = humatest.NewContext(nil, r, w)

	p.IfUnmodifiedSince = now

	p.Resolve(ctx)
	perr := p.PreconditionFailed("", after)
	require.Error(t, perr)
	assert.Equal(t, http.StatusPreconditionFailed, perr.GetStatus())
}
