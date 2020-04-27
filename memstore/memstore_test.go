package memstore

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/humatest"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

func TestAutoBadType(t *testing.T) {
	r := humatest.NewRouter(t).Resource("/tests")
	store := New()

	assert.Panics(t, func() {
		store.AutoResource(r, "whoops")
	})
}

func TestAutoMissingID(t *testing.T) {
	type Data struct {
		Content string `json:"content"`
	}

	r := humatest.NewRouter(t).Resource("/tests")
	store := New()

	assert.Panics(t, func() {
		store.AutoResource(r, Data{})
	})
}

func TestAutoIDType(t *testing.T) {
	type Data struct {
		ID int32 `json:"id"`
	}

	r := humatest.NewRouter(t).Resource("/tests")
	store := New()

	assert.Panics(t, func() {
		store.AutoResource(r, Data{})
	})
}

func TestAutoIDMissingJSON(t *testing.T) {
	type Data struct {
		ID string
	}

	r := humatest.NewRouter(t).Resource("/tests")
	store := New()

	assert.Panics(t, func() {
		store.AutoResource(r, Data{})
	})
}

func TestAutoIDJSONTag(t *testing.T) {
	type Data struct {
		ID string `json:"other"`
	}

	r := humatest.NewRouter(t).Resource("/tests")
	store := New()

	assert.Panics(t, func() {
		store.AutoResource(r, Data{})
	})
}

func TestPointer(t *testing.T) {
	type Data struct {
		ID string `json:"id"`
	}

	r := humatest.NewRouter(t)
	store := New()

	// Should not panic
	store.AutoResource(r.Resource("/tests"), &Data{})
}

func TestRename(t *testing.T) {
	type Data struct {
		ID string `json:"id"`
	}

	r := humatest.NewRouter(t)
	store := New()

	// Should not panic
	store.AutoResource(r.Resource("/tests"), Data{},
		Name("item", "items"),
	)

	// TODO: check OpenAPI naming
}

type Data struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created" readOnly:"true"`
	Content string    `json:"content"`
}

func (d *Data) OnCreate() {
	d.Created = time.Now().UTC()
}

func TestAuto(t *testing.T) {
	r := humatest.NewRouter(t)
	store := New()

	store.AutoResource(r.Resource("/tests"), &Data{})

	// Create some items
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/tests/test1", strings.NewReader(`{"content": "test1"}`))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/tests/test2", strings.NewReader(`{"content": "test2"}`))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// List all items
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/tests", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	result := []interface{}{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)
	assert.Len(t, result, 2)

	// Update an item
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/tests/test1", strings.NewReader(`{"content": "updated"}`))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Get with bad ID
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/tests/invalid", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Get an item
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/tests/test1", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	item := map[string]interface{}{}
	err = json.Unmarshal(w.Body.Bytes(), &item)
	assert.NoError(t, err)
	assert.Equal(t, "updated", item["content"])
	etag := w.Header().Get("etag")

	// TODO: conditional update failure?

	// Delete with invalid ID
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, "/tests/invalid", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Delete with incorrect ETag
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, "/tests/test1", nil)
	req.Header.Add("if-match", `"bad-value"`)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)

	// Delete with correct ETag
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, "/tests/test1", nil)
	req.Header.Add("if-match", etag)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)

	// Unconditional delete
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, "/tests/test2", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}
