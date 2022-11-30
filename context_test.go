package huma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
)

func TestGetContextFromRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), contextKey("foo"), "bar"))
		ctx := ContextFromRequest(nil, w, r)
		assert.Equal(t, "bar", ctx.Value(contextKey("foo")))
	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	handler(w, r)
}

func TestContentNegotiation(t *testing.T) {
	type Response struct {
		Value string `json:"value"`
	}

	app := newTestRouter()

	app.Resource("/negotiated").Get("test", "Test",
		NewResponse(200, "desc").Model(Response{}),
	).Run(func(ctx Context) {
		ctx.WriteModel(http.StatusOK, Response{
			Value: "Hello, world!",
		})
	})

	var parsed Response
	expected := Response{
		Value: "Hello, world!",
	}

	// No preference
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/negotiated", nil)
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, expected, parsed)

	// Prefer JSON
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/negotiated", nil)
	req.Header.Set("Accept", "application/json")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	err = json.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, expected, parsed)

	// Prefer YAML
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/negotiated", nil)
	req.Header.Set("Accept", "application/yaml")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/yaml", w.Header().Get("Content-Type"))
	err = yaml.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, expected, parsed)

	// Prefer CBOR
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/negotiated", nil)
	req.Header.Set("Accept", "application/cbor")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/cbor", w.Header().Get("Content-Type"))
	err = cbor.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.EqualValues(t, expected, parsed)
}

func TestErrorNegotiation(t *testing.T) {
	app := newTestRouter()

	app.Resource("/error").Get("test", "Test",
		NewResponse(400, "desc").Model(&ErrorModel{}),
	).Run(func(ctx Context) {
		ctx.AddError(fmt.Errorf("some error"))
		ctx.AddError(&ErrorDetail{
			Message:  "Invalid value",
			Location: "body.field",
			Value:    "test",
		})
		ctx.WriteError(http.StatusBadRequest, "test error")
	})

	var parsed ErrorModel
	expected := ErrorModel{
		Status: http.StatusBadRequest,
		Title:  http.StatusText(http.StatusBadRequest),
		Detail: "test error",
		Errors: []*ErrorDetail{
			{Message: "some error"},
			{
				Message:  "Invalid value",
				Location: "body.field",
				Value:    "test",
			},
		},
	}

	// No preference
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/error", nil)
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, expected, parsed)

	// Prefer JSON
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/error", nil)
	req.Header.Set("Accept", "application/json")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	err = json.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, expected, parsed)

	// Prefer YAML
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/error", nil)
	req.Header.Set("Accept", "application/yaml")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+yaml", w.Header().Get("Content-Type"))
	err = yaml.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.EqualValues(t, expected, parsed)

	// Prefer CBOR
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/error", nil)
	req.Header.Set("Accept", "application/cbor")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/problem+cbor", w.Header().Get("Content-Type"))
	err = cbor.Unmarshal(w.Body.Bytes(), &parsed)
	assert.NoError(t, err)
	assert.Equal(t, expected, parsed)
}

func TestInvalidModel(t *testing.T) {
	type R1 struct {
		Foo string `json:"foo"`
	}

	type R2 struct {
		Bar string `json:"bar"`
	}

	app := newTestRouter()

	app.Resource("/bad-status").Get("test", "Test",
		NewResponse(http.StatusOK, "desc").Model(R1{}),
	).Run(func(ctx Context) {
		ctx.WriteModel(http.StatusNoContent, R2{Bar: "blah"})
	})

	app.Resource("/bad-model").Get("test", "Test",
		NewResponse(http.StatusOK, "desc").Model(R1{}),
	).Run(func(ctx Context) {
		ctx.WriteModel(http.StatusOK, R2{Bar: "blah"})
	})

	assert.Panics(t, func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/bad-status", nil)
		app.ServeHTTP(w, req)
	})

	assert.Panics(t, func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/bad-model", nil)
		app.ServeHTTP(w, req)
	})
}

func TestInvalidHeader(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusNoContent, "desc").Headers("Extra"),
	).Run(func(ctx Context) {
		// Typo in the header should not be allowed
		ctx.Header().Set("Extra2", "some-value")
		ctx.WriteHeader(http.StatusNoContent)
	})

	assert.Panics(t, func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		app.ServeHTTP(w, req)
	})
}

func TestWriteAfterClose(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("test", "Test",
		NewResponse(http.StatusBadRequest, "desc").Model(&ErrorModel{}),
	).Run(func(ctx Context) {
		ctx.WriteError(http.StatusBadRequest, "some error")
		// Second write should fail
		ctx.WriteError(http.StatusBadRequest, "some error")
	})

	assert.Panics(t, func() {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		app.ServeHTTP(w, req)
	})
}

func TestValue(t *testing.T) {
	key := contextKey("foo")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ContextFromRequest(w, r)

		// Create a copy with a new value attached.
		ctx2 := ctx.WithValue(key, "bar")

		assert.Equal(t, nil, ctx.Value(key))
		assert.Equal(t, "bar", ctx2.Value(key))

		// Set a value in-place in the original
		ctx.SetValue(key, "baz")

		// Make sure it was set on ctx, and that ctx2 wasn't modified since it's
		// a copy created before the call to `SetValue(...)`.
		assert.Equal(t, "baz", ctx.Value(key))
		assert.Equal(t, "bar", ctx2.Value(key))
	})

	w := httptest.NewRecorder()
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	handler(w, r)
}

func TestWriteContent(t *testing.T) {
	app := newTestRouter()

	b := []byte("Test Byte Data")

	app.Resource("/content").Get("test", "Test",
		NewResponse(200, "desc").Model(Response{}),
	).Run(func(ctx Context) {
		ctx.Header().Set("Content-Type", "application/octet-stream")
		content := bytes.NewReader(b)
		ctx.WriteContent("", content, time.Time{})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/content", nil)
	app.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Result().StatusCode)
	assert.Equal(t, "application/octet-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, b, w.Body.Bytes())
}

func TestWriteContentRespectsRange(t *testing.T) {
	app := newTestRouter()

	b := []byte("Test Byte Data")

	app.Resource("/content").Get("test", "Test",
		NewResponse(206, "desc").Model(Response{}),
	).Run(func(ctx Context) {
		ctx.Header().Set("Content-Type", "application/octet-stream")
		content := bytes.NewReader(b)
		ctx.WriteContent("", content, time.Time{})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/content", nil)
	req.Header.Set("Range", "bytes=0-5")
	app.ServeHTTP(w, req)

	// confirms that Range is properly being forwarded to ServeContent.
	// we'll assume more advanced range use cases are properly tested
	// in the http library.
	assert.Equal(t, w.Result().StatusCode, 206)
	assert.Equal(t, []byte("Test B"), w.Body.Bytes())
}

func TestWriteContentLastModified(t *testing.T) {
	app := newTestRouter()

	b := []byte("Test Byte Data")
	modTime := time.Now()

	app.Resource("/content").Get("test", "Test",
		NewResponse(206, "desc").Model(Response{}),
	).Run(func(ctx Context) {
		ctx.Header().Set("Content-Type", "application/octet-stream")
		content := bytes.NewReader(b)
		ctx.WriteContent("", content, modTime)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/content", nil)
	app.ServeHTTP(w, req)

	// confirms that modTime is properly being forwarded to ServeContent.
	// We'll assume the more advanced modTime use cases are properly tested
	// in http library.
	strTime := modTime.UTC().Format(http.TimeFormat)

	assert.Equal(t, strTime, w.Header().Get("Last-Modified"))

}

func TestWriteContentName(t *testing.T) {
	app := newTestRouter()

	b := []byte("Test Byte Data")

	app.Resource("/content").Get("test", "Test",
		NewResponse(206, "desc").Model(Response{}),
	).Run(func(ctx Context) {

		content := bytes.NewReader(b)
		ctx.WriteContent("/path/with/content.mp4", content, time.Time{})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/content", nil)
	app.ServeHTTP(w, req)

	// confirms that name is properly being forwarded to ServeContent.
	// We'll assume the more advanced modTime use cases are properly tested
	// in http library.
	assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
}
