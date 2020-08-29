package middleware

import (
	"compress/gzip"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/responses"
	"github.com/stretchr/testify/assert"
)

func TestContentEncodingTooSmall(t *testing.T) {
	app, _ := NewTestRouter(t)
	app.Resource("/").Get("root", "test",
		responses.OK().ContentType("text/plain"),
	).Run(func(ctx huma.Context) {
		ctx.Write([]byte("Short string"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip, br")
	app.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "", w.Result().Header.Get("Content-Encoding"))
	assert.Equal(t, "Short string", w.Body.String())
}

func TestContentEncodingIgnoredPath(t *testing.T) {
	app, _ := NewTestRouter(t)
	app.Resource("/foo.png").Get("root", "test",
		responses.OK().ContentType("image/png"),
	).Run(func(ctx huma.Context) {
		ctx.Header().Set("Content-Type", "image/png")
		ctx.Write([]byte("fake png"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/foo.png", nil)
	req.Header.Add("Accept-Encoding", "gzip, br")
	app.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "", w.Result().Header.Get("Content-Encoding"))
	assert.Equal(t, "fake png", w.Body.String())
}

func TestContentEncodingCompressed(t *testing.T) {
	app, _ := NewTestRouter(t)
	app.Resource("/").Get("root", "test",
		responses.OK(),
	).Run(func(ctx huma.Context) {
		ctx.Write(make([]byte, 1500))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip, br")
	app.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "br", w.Result().Header.Get("Content-Encoding"))
	assert.Less(t, len(w.Body.String()), 1500)

	br := brotli.NewReader(w.Body)
	decoded, _ := ioutil.ReadAll(br)
	assert.Equal(t, 1500, len(decoded))
}

func TestContentEncodingCompressedPick(t *testing.T) {
	app, _ := NewTestRouter(t)
	app.Resource("/").Get("root", "test",
		responses.OK(),
	).Run(func(ctx huma.Context) {
		ctx.Write(make([]byte, 1500))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip, br; q=0.9, deflate")
	app.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "gzip", w.Result().Header.Get("Content-Encoding"))
	assert.Less(t, len(w.Body.String()), 1500)
}

func TestContentEncodingCompressedMultiWrite(t *testing.T) {
	app, _ := NewTestRouter(t)
	app.Resource("/").Get("root", "test",
		responses.OK(),
	).Run(func(ctx huma.Context) {
		buf := make([]byte, 750)
		ctx.Write(buf)
		ctx.Write(buf)
		ctx.Write(buf)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip")
	app.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "gzip", w.Result().Header.Get("Content-Encoding"))
	assert.Less(t, len(w.Body.String()), 2250)

	gr, _ := gzip.NewReader(w.Body)
	decoded, _ := ioutil.ReadAll(gr)
	assert.Equal(t, 2250, len(decoded))
}

func TestContentEncodingError(t *testing.T) {
	var status int

	app, _ := NewTestRouter(t)
	app.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wrapped := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(wrapped, r)
			status = wrapped.status
		})
	})

	app.Resource("/").Get("root", "test",
		responses.OK(),
	).Run(func(ctx huma.Context) {
		ctx.WriteHeader(http.StatusNotFound)
		ctx.Write([]byte("some text"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}
