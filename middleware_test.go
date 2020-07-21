package huma

import (
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRecoveryMiddleware(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(Recovery())

	r.Resource("/panic").Get("Panic recovery test", func() string {
		panic(fmt.Errorf("Some error"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Result().Header.Get("content-type"))
}

func TestRecoveryMiddlewareString(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(Recovery())

	r.Resource("/panic").Get("Panic recovery test", func() string {
		panic("Some error")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Result().Header.Get("content-type"))
}

func TestRecoveryMiddlewareLogBody(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(Recovery())

	r.Resource("/panic").Put("Panic recovery test", func(in map[string]string) string {
		panic(fmt.Errorf("Some error"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/panic", strings.NewReader(`{"foo": "bar"}`))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Result().Header.Get("content-type"))
}

func TestPreferMinimalMiddleware(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(PreferMinimalMiddleware())

	r.Resource("/test").Get("desc", func() string {
		return "Hello, test"
	})

	r.Resource("/non200", ResponseText(http.StatusBadRequest, "desc")).Get("desc", func() string {
		return "Error details"
	})

	// Normal request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())

	// Prefer minimal should return 204 No Content
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Add("prefer", "return=minimal")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())

	// Prefer minimal which can still return non-200 response bodies
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/non200", nil)
	req.Header.Add("prefer", "return=minimal")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.NotEmpty(t, w.Body.String())
}

func TestHandler404(t *testing.T) {
	g := gin.New()
	g.NoRoute(Handler404())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/notfound", nil)
	g.ServeHTTP(w, req)
	assert.Equal(t, w.Result().StatusCode, http.StatusNotFound)
	assert.Equal(t, "application/problem+json", w.Result().Header.Get("content-type"))
}

func TestServiceLinks(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(ServiceLinkMiddleware())
	r.GinEngine().NoRoute(Handler404())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, w.Result().StatusCode, http.StatusNoContent)
	assert.Contains(t, w.Result().Header.Get("link"), "service-desc")
	assert.Contains(t, w.Result().Header.Get("link"), "service-doc")
}

func TestServiceLinksExists(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(ServiceLinkMiddleware())
	r.GinEngine().NoRoute(Handler404())
	r.GinEngine().GET("/", func(c *gin.Context) {
		c.Header("link", `<>; rel=self`)
		AddServiceLinks(c)
		c.Data(http.StatusOK, "text/plain", []byte("Hello"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Contains(t, w.Result().Header.Get("link"), "service-desc")
	assert.Contains(t, w.Result().Header.Get("link"), "service-doc")
}

func TestContentEncodingTooSmall(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(ContentEncodingMiddleware())
	r.Resource("/").Get("test", func() string {
		return "Short string"
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip, br")
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "", w.Result().Header.Get("Content-Encoding"))
	assert.Equal(t, "Short string", w.Body.String())
}

func TestContentEncodingIgnoredPath(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(ContentEncodingMiddleware())
	r.Resource("/foo.png").Get("test", func() string {
		return "fake png"
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/foo.png", nil)
	req.Header.Add("Accept-Encoding", "gzip, br")
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "", w.Result().Header.Get("Content-Encoding"))
	assert.Equal(t, "fake png", w.Body.String())
}

func TestContentEncodingCompressed(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(ContentEncodingMiddleware())
	r.Resource("/").Get("test", func() string {
		// Highly compressable 1500 zero bytes.
		buf := make([]byte, 1500)
		return string(buf)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip, br")
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "br", w.Result().Header.Get("Content-Encoding"))
	assert.Less(t, len(w.Body.String()), 1500)

	br := brotli.NewReader(w.Body)
	decoded, _ := ioutil.ReadAll(br)
	assert.Equal(t, 1500, len(decoded))
}

func TestContentEncodingCompressedPick(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(ContentEncodingMiddleware())
	r.Resource("/").Get("test", func() string {
		// Highly compressable 1500 zero bytes.
		buf := make([]byte, 1500)
		return string(buf)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip, br; q=0.9, deflate")
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "gzip", w.Result().Header.Get("Content-Encoding"))
	assert.Less(t, len(w.Body.String()), 1500)
}

func TestContentEncodingCompressedMultiWrite(t *testing.T) {
	r := NewTestRouter(t)
	r.GinEngine().Use(ContentEncodingMiddleware())
	r.GinEngine().GET("/", func(c *gin.Context) {
		buf := make([]byte, 750)
		// Making writes past the MTU boundary should still work.
		c.Writer.Write(buf)
		c.Writer.Write(buf)
		c.Writer.Write(buf)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip")
	r.ServeHTTP(w, req)

	assert.Equal(t, w.Result().StatusCode, http.StatusOK)
	assert.Equal(t, "gzip", w.Result().Header.Get("Content-Encoding"))
	assert.Less(t, len(w.Body.String()), 2250)

	gr, _ := gzip.NewReader(w.Body)
	decoded, _ := ioutil.ReadAll(gr)
	assert.Equal(t, 2250, len(decoded))
}

func TestContentEncodingError(t *testing.T) {
	var status int

	r := NewTestRouter(t)
	r.GinEngine().Use(ContentEncodingMiddleware())
	r.GinEngine().Use(func(c *gin.Context) {
		c.Next()

		// Other middleware should be able to read the response status
		status = c.Writer.Status()
	})
	r.GinEngine().GET("/", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusNotFound)
		c.Writer.Write([]byte("some text"))
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Accept-Encoding", "gzip")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, status)
	assert.Equal(t, http.StatusNotFound, w.Result().StatusCode)
}
