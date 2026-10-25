package autopatch

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
)

// TestPatchEchoGzip is a regression test for #753. Router middleware also wraps
// the internal GET and PUT, so they must not inherit the PATCH's content codings.
func TestPatchEchoGzip(t *testing.T) {
	type body struct {
		Name string `json:"name"`
	}
	type output struct {
		Body body
	}

	var patch bytes.Buffer
	zw := gzip.NewWriter(&patch)
	_, err := zw.Write([]byte(`{"name":"after"}`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	cases := []struct {
		name     string
		putError bool
		status   int
	}{
		{name: "success", status: http.StatusOK},
		{name: "put-error", putError: true, status: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := echo.New()
			r.Use(middleware.Recover(), middleware.Gzip(), middleware.Decompress())
			api := humaecho.New(r, huma.DefaultConfig("Test", "1.0.0"))
			huma.Get(api, "/thing", func(ctx context.Context, input *struct{}) (*output, error) {
				return &output{Body: body{Name: "before"}}, nil
			})
			huma.Put(api, "/thing", func(ctx context.Context, input *struct{ Body body }) (*output, error) {
				if tc.putError {
					return nil, huma.Error400BadRequest("Update rejected")
				}
				return &output{Body: input.Body}, nil
			})
			AutoPatch(api)

			req := httptest.NewRequest(http.MethodPatch, "/thing", bytes.NewReader(patch.Bytes()))
			req.Header.Set("Content-Type", "application/merge-patch+json")
			req.Header.Set("Content-Encoding", "gzip")
			req.Header.Set("Accept-Encoding", "gzip")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tc.status, w.Code)
			require.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
			// Clients decode Content-Encoding once, so what's left must be JSON.
			zr, err := gzip.NewReader(w.Body)
			require.NoError(t, err)
			var resp struct {
				Name   string `json:"name"`
				Status int    `json:"status"`
			}
			require.NoError(t, json.NewDecoder(zr).Decode(&resp))
			if tc.putError {
				assert.Equal(t, tc.status, resp.Status)
			} else {
				assert.Equal(t, "after", resp.Name)
			}
		})
	}
}
