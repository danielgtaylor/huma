package conditional

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma"
	"github.com/stretchr/testify/assert"
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
	ctx := huma.ContextFromRequest(w, r)

	p.IfMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx, r)
	assert.False(t, p.PreconditionFailed(ctx, "abc123", time.Time{}))
	assert.False(t, p.PreconditionFailed(ctx, "def456", time.Time{}))
	assert.True(t, p.PreconditionFailed(ctx, "bad", time.Time{}))
	assert.True(t, p.PreconditionFailed(ctx, "", time.Time{}))

	assert.False(t, ctx.HasError())
	assert.Equal(t, http.StatusNotModified, w.Result().StatusCode)

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = huma.ContextFromRequest(w, r)

	p.IfMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx, r)
	assert.False(t, p.PreconditionFailed(ctx, "abc123", time.Time{}))
	assert.False(t, ctx.HasError())

	assert.True(t, p.PreconditionFailed(ctx, "bad", time.Time{}))
	assert.True(t, ctx.HasError())
	assert.Equal(t, http.StatusPreconditionFailed, w.Result().StatusCode)
}

func TestIfNoneMatch(t *testing.T) {
	p := Params{}

	// Read request
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/resource", nil)
	w := httptest.NewRecorder()
	ctx := huma.ContextFromRequest(w, r)

	p.IfNoneMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx, r)
	assert.False(t, p.PreconditionFailed(ctx, "bad", time.Time{}))
	assert.False(t, p.PreconditionFailed(ctx, "", time.Time{}))
	assert.True(t, p.PreconditionFailed(ctx, "abc123", time.Time{}))
	assert.True(t, p.PreconditionFailed(ctx, "def456", time.Time{}))

	assert.False(t, ctx.HasError())
	assert.Equal(t, http.StatusNotModified, w.Result().StatusCode)

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = huma.ContextFromRequest(w, r)

	p.IfNoneMatch = []string{`"abc123"`, `W/"def456"`}
	p.Resolve(ctx, r)
	assert.True(t, p.PreconditionFailed(ctx, "abc123", time.Time{}))
	assert.True(t, ctx.HasError())

	ctx = huma.ContextFromRequest(w, r)
	assert.False(t, p.PreconditionFailed(ctx, "bad", time.Time{}))
	assert.False(t, ctx.HasError())

	// Write with special `*` syntax to match any.
	p.IfNoneMatch = []string{"*"}
	ctx = huma.ContextFromRequest(w, r)
	assert.False(t, p.PreconditionFailed(ctx, "", time.Time{}))
	assert.False(t, ctx.HasError())

	assert.True(t, p.PreconditionFailed(ctx, "abc123", time.Time{}))
	assert.True(t, ctx.HasError())
	assert.Equal(t, http.StatusPreconditionFailed, w.Result().StatusCode)
}

func TestIfModifiedSince(t *testing.T) {
	p := Params{}

	now, err := time.Parse(time.RFC3339, "2021-01-01T12:00:00Z")
	assert.NoError(t, err)

	before, err := time.Parse(time.RFC3339, "2020-01-01T12:00:00Z")
	assert.NoError(t, err)

	after, err := time.Parse(time.RFC3339, "2022-01-01T12:00:00Z")
	assert.NoError(t, err)

	// Read request
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/resource", nil)
	w := httptest.NewRecorder()
	ctx := huma.ContextFromRequest(w, r)

	p.IfModifiedSince = now

	p.Resolve(ctx, r)
	assert.True(t, p.PreconditionFailed(ctx, "", before))
	assert.True(t, p.PreconditionFailed(ctx, "", now))
	assert.False(t, p.PreconditionFailed(ctx, "", after))

	assert.False(t, ctx.HasError())

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = huma.ContextFromRequest(w, r)

	p.IfModifiedSince = now

	p.Resolve(ctx, r)
	assert.True(t, p.PreconditionFailed(ctx, "", before))
	assert.True(t, ctx.HasError())
	assert.Equal(t, http.StatusPreconditionFailed, w.Result().StatusCode)
}

func TestIfUnmodifiedSince(t *testing.T) {
	p := Params{}

	now, err := time.Parse(time.RFC3339, "2021-01-01T12:00:00Z")
	assert.NoError(t, err)

	before, err := time.Parse(time.RFC3339, "2020-01-01T12:00:00Z")
	assert.NoError(t, err)

	after, err := time.Parse(time.RFC3339, "2022-01-01T12:00:00Z")
	assert.NoError(t, err)

	// Read request
	r, _ := http.NewRequest(http.MethodGet, "https://example.com/resource", nil)
	w := httptest.NewRecorder()
	ctx := huma.ContextFromRequest(w, r)

	p.IfUnmodifiedSince = now

	p.Resolve(ctx, r)
	assert.False(t, p.PreconditionFailed(ctx, "", before))
	assert.False(t, p.PreconditionFailed(ctx, "", now))
	assert.True(t, p.PreconditionFailed(ctx, "", after))

	assert.False(t, ctx.HasError())

	// Write request
	r, _ = http.NewRequest(http.MethodPut, "https://example.com/resource", nil)
	w = httptest.NewRecorder()
	ctx = huma.ContextFromRequest(w, r)

	p.IfUnmodifiedSince = now

	p.Resolve(ctx, r)
	assert.True(t, p.PreconditionFailed(ctx, "", after))
	assert.True(t, ctx.HasError())
	assert.Equal(t, http.StatusPreconditionFailed, w.Result().StatusCode)
}
