package autopatch

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	echoV4 "github.com/labstack/echo/v4"
	middlewareV4 "github.com/labstack/echo/v4/middleware"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
)

// Internal requests pass through the router's middleware too. Their responses
// must not be compressed again when forwarded through the PATCH middleware.
func TestPatchEchoGzip(t *testing.T) {
	adapters := []struct {
		name   string
		newAPI func() (http.Handler, huma.API)
	}{
		{"v4", func() (http.Handler, huma.API) {
			r := echoV4.New()
			r.Use(middlewareV4.Gzip())
			return r, humaecho.NewV4(r, huma.DefaultConfig("Test", "1.0.0"))
		}},
		{"v5", func() (http.Handler, huma.API) {
			r := echo.New()
			r.Use(middleware.Gzip())
			return r, humaecho.New(r, huma.DefaultConfig("Test", "1.0.0"))
		}},
	}
	cases := []struct {
		name     string
		getError bool
		putError bool
		ifMatch  string
		status   int
	}{
		{name: "success", status: http.StatusOK},
		{name: "get-error", getError: true, status: http.StatusNotFound},
		{name: "put-error", putError: true, status: http.StatusBadRequest},
		{name: "precondition-failed", ifMatch: `"stale"`, status: http.StatusPreconditionFailed},
	}
	for _, adapter := range adapters {
		for _, encoding := range []string{"", "identity", "gzip"} {
			for _, tc := range cases {
				t.Run(adapter.name+"/"+encoding+"/"+tc.name, func(t *testing.T) {
					router, api := adapter.newAPI()
					type body struct {
						Name string `json:"name"`
					}
					type output struct {
						ETag string `header:"ETag"`
						Body body
					}
					value := body{Name: "before"}
					putCalled := false
					huma.Get(api, "/thing", func(ctx context.Context, input *struct{}) (*output, error) {
						if tc.getError {
							return nil, huma.Error404NotFound("Not found")
						}
						return &output{ETag: `"before"`, Body: value}, nil
					})
					huma.Put(api, "/thing", func(ctx context.Context, input *struct {
						IfMatch string `header:"If-Match"`
						Body    body
					}) (*output, error) {
						putCalled = true
						if tc.ifMatch != "" {
							assert.Equal(t, tc.ifMatch, input.IfMatch)
						} else {
							assert.Equal(t, `"before"`, input.IfMatch)
						}
						if input.IfMatch != `"before"` {
							return nil, huma.Error412PreconditionFailed("ETag mismatch")
						}
						if tc.putError {
							return nil, huma.Error400BadRequest("Update rejected")
						}
						value = input.Body
						return &output{ETag: `"after"`, Body: value}, nil
					})
					AutoPatch(api)

					req := httptest.NewRequest(http.MethodPatch, "/thing", strings.NewReader(`{"name":"after"}`))
					req.Header.Set("Content-Type", "application/merge-patch+json")
					if encoding != "" {
						req.Header.Set("Accept-Encoding", encoding)
					}
					if tc.ifMatch != "" {
						req.Header.Set("If-Match", tc.ifMatch)
					}
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)
					assert.Equal(t, tc.status, w.Code)
					assert.Equal(t, !tc.getError, putCalled)
					if tc.status == http.StatusOK {
						assert.Equal(t, "after", value.Name)
						assert.Equal(t, `"after"`, w.Header().Get("ETag"))
					} else {
						assert.Equal(t, "before", value.Name)
					}

					var reader io.Reader = w.Body
					if encoding == "gzip" {
						require.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
						gz, err := gzip.NewReader(reader)
						require.NoError(t, err)
						defer gz.Close()
						reader = gz
					} else {
						assert.Empty(t, w.Header().Get("Content-Encoding"))
					}
					data, err := io.ReadAll(reader)
					require.NoError(t, err)
					var response struct {
						Name   string `json:"name"`
						Status int    `json:"status"`
					}
					// Decode Content-Encoding exactly once, as an HTTP client does.
					require.NoError(t, json.Unmarshal(data, &response))
					if tc.status == http.StatusOK {
						assert.Equal(t, "after", response.Name)
					} else {
						assert.Equal(t, tc.status, response.Status)
					}
				})
			}
		}
	}
}
