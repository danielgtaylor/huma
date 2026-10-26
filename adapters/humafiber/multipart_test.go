package humafiber

import (
	"bytes"
	"compress/gzip"
	"context"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	fiberV2 "github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMultipartBody = "--B\r\nContent-Disposition: form-data; name=\"f\"\r\n\r\nhello\r\n--B--\r\n"

// TestMultipartLimit covers the Fiber-specific paths of the MaxBodyBytes
// check: streamed bodies, gzip-encoded bodies and the app-wide BodyLimit.
func TestMultipartLimit(t *testing.T) {
	var gzipped bytes.Buffer
	gz := gzip.NewWriter(&gzipped)
	_, err := gz.Write([]byte(strings.Replace(testMultipartBody, "hello", strings.Repeat("a", 1<<20), 1)))
	require.NoError(t, err)
	require.NoError(t, gz.Close())

	// Streamed: fasthttp hands the body over as a stream instead of buffering
	// or pre-parsing it.
	streamV3 := fiber.Config{StreamRequestBody: true, BodyLimit: 1, DisablePreParseMultipartForm: true}
	streamV2 := fiberV2.Config{StreamRequestBody: true, BodyLimit: 1, DisablePreParseMultipartForm: true}
	n := int64(len(testMultipartBody))

	for _, tc := range []struct {
		name        string
		v3          fiber.Config
		v2          fiberV2.Config
		maxBytes    int64
		contentType string
		encoding    string
		body        []byte
		status      int
	}{
		{name: "streamed-at-limit", v3: streamV3, v2: streamV2, maxBytes: n, status: http.StatusNoContent},
		{name: "streamed-over-limit", v3: streamV3, v2: streamV2, maxBytes: n - 1, status: http.StatusRequestEntityTooLarge},
		{name: "streamed-mixed-case-content-type", v3: streamV3, v2: streamV2, maxBytes: n, contentType: "Multipart/Form-Data; Boundary=B", status: http.StatusNoContent},
		{name: "huge-limit", v3: streamV3, v2: streamV2, maxBytes: 1<<63 - 1, status: http.StatusNoContent},
		// The decoded size counts, not the compressed one.
		{name: "gzip-over-limit", maxBytes: 1 << 19, encoding: "gzip", body: gzipped.Bytes(), status: http.StatusRequestEntityTooLarge},
		// MaxBodyBytes must not raise the app-wide BodyLimit, which fasthttp only
		// checks against the compressed body itself.
		{name: "body-limit-wins", v3: fiber.Config{BodyLimit: 1 << 15}, v2: fiberV2.Config{BodyLimit: 1 << 15}, maxBytes: 1 << 30, encoding: "gzip", body: gzipped.Bytes(), status: http.StatusRequestEntityTooLarge},
	} {
		for _, version := range []struct {
			name string
			new  func() (huma.API, func(*http.Request) (*http.Response, error))
		}{
			{"v3", func() (huma.API, func(*http.Request) (*http.Response, error)) {
				app := fiber.New(tc.v3)
				return New(app, huma.DefaultConfig("Test", "1.0.0")), func(r *http.Request) (*http.Response, error) { return app.Test(r) }
			}},
			{"v2", func() (huma.API, func(*http.Request) (*http.Response, error)) {
				app := fiberV2.New(tc.v2)
				return NewV2(app, huma.DefaultConfig("Test", "1.0.0")), func(r *http.Request) (*http.Response, error) { return app.Test(r) }
			}},
		} {
			t.Run(tc.name+"/"+version.name, func(t *testing.T) {
				a, do := version.new()
				huma.Register(a, huma.Operation{
					Method:       http.MethodPost,
					Path:         "/upload",
					MaxBodyBytes: tc.maxBytes,
				}, func(ctx context.Context, input *struct {
					RawBody multipart.Form
				}) (*struct{}, error) {
					assert.Equal(t, []string{"hello"}, input.RawBody.Value["f"])
					return nil, nil
				})

				body := tc.body
				if body == nil {
					body = []byte(testMultipartBody)
				}
				req, _ := http.NewRequest(http.MethodPost, "http://example.com/upload", bytes.NewReader(body))
				req.Header.Set("Content-Type", "multipart/form-data; boundary=B")
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
				}
				if tc.encoding != "" {
					req.Header.Set("Content-Encoding", tc.encoding)
				}
				resp, err := do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				assert.Equal(t, tc.status, resp.StatusCode)
			})
		}
	}
}
