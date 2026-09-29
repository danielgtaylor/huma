package humagofr

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	gofr "gofr.dev/pkg/gofr"
	gofrHTTP "gofr.dev/pkg/gofr/http"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRouter struct {
	mux         *http.ServeMux
	middlewares []gofrHTTP.Middleware
}

func newFakeRouter() *fakeRouter {
	return &fakeRouter{mux: http.NewServeMux()}
}

func (f *fakeRouter) add(method, path string, handler gofr.Handler) {
	f.mux.HandleFunc(method+" "+path, func(w http.ResponseWriter, r *http.Request) {
		request := &serveHTTPRequest{Request: gofrHTTP.NewRequest(r), r: r}
		ctx := &gofr.Context{Context: r.Context(), Request: request}
		_, err := handler(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Simulate GoFr's responder. The adapter must suppress this second
		// response after Huma has already written its response.
		w.Header().Set("X-GoFr-Responder", "called")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"data":null}`))
	})
}

func (f *fakeRouter) GET(path string, handler gofr.Handler) {
	f.add(http.MethodGet, path, handler)
}

func (f *fakeRouter) PUT(path string, handler gofr.Handler) {
	f.add(http.MethodPut, path, handler)
}

func (f *fakeRouter) POST(path string, handler gofr.Handler) {
	f.add(http.MethodPost, path, handler)
}

func (f *fakeRouter) DELETE(path string, handler gofr.Handler) {
	f.add(http.MethodDelete, path, handler)
}

func (f *fakeRouter) PATCH(path string, handler gofr.Handler) {
	f.add(http.MethodPatch, path, handler)
}

func (f *fakeRouter) UseMiddleware(middlewares ...gofrHTTP.Middleware) {
	f.middlewares = append(f.middlewares, middlewares...)
}

func (f *fakeRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var handler http.Handler = f.mux
	for i := len(f.middlewares) - 1; i >= 0; i-- {
		handler = f.middlewares[i](handler)
	}
	handler.ServeHTTP(w, r)
}

type contextKey struct{}

func TestGoFrRouting(t *testing.T) {
	router := newFakeRouter()
	api := New(router, huma.DefaultConfig("Test API", "1.0.0"))

	var nativeContext context.Context
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx = huma.WithValue(ctx, contextKey{}, "value")
		nativeContext = Unwrap(ctx).Context
		next(ctx)
	})

	type Input struct {
		ID    string `path:"id"`
		Limit int    `query:"limit" minimum:"1"`
	}
	type Output struct {
		Value string `header:"X-Value"`
		Body  struct {
			ID string `json:"id"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{id}",
	}, func(_ context.Context, input *Input) (*Output, error) {
		output := &Output{Value: "huma"}
		output.Body.ID = input.ID
		return output, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/things/123?limit=1", nil)
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "huma", recorder.Header().Get("X-Value"))
	assert.Empty(t, recorder.Header().Get("X-GoFr-Responder"))
	assert.Equal(t, "value", nativeContext.Value(contextKey{}))

	var body map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "123", body["id"])
	assert.NotContains(t, recorder.Body.String(), `"data"`)

	invalidRecorder := httptest.NewRecorder()
	invalidRequest := httptest.NewRequest(http.MethodGet, "/things/123?limit=0", nil)
	router.ServeHTTP(invalidRecorder, invalidRequest)
	assert.Equal(t, http.StatusUnprocessableEntity, invalidRecorder.Code)
	assert.Contains(t, invalidRecorder.Body.String(), "expected number >= 1")
	assert.NotContains(t, invalidRecorder.Body.String(), `"data"`)
}

func TestServeHTTP(t *testing.T) {
	router := newFakeRouter()
	api := New(router, huma.DefaultConfig("Test API", "1.0.0"))

	var native *gofr.Context
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		native = Unwrap(ctx)
		next(ctx)
	})

	huma.Get(api, "/ping/{value}", func(_ context.Context, input *struct {
		Value string `path:"value"`
	}) (*struct {
		Body string
	}, error) {
		return &struct{ Body string }{Body: input.Value}, nil
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ping/pong", nil)
	api.Adapter().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `"pong"`, recorder.Body.String())
	assert.NotNil(t, native)
}

func TestUnsupportedMethod(t *testing.T) {
	router := newFakeRouter()
	api := New(router, huma.DefaultConfig("Test API", "1.0.0"))

	assert.PanicsWithValue(t, "humagofr: GoFr does not support OPTIONS route registration", func() {
		api.Adapter().Handle(&huma.Operation{Method: http.MethodOptions, Path: "/things"}, func(huma.Context) {})
	})
}

func TestSupportedMethods(t *testing.T) {
	for _, method := range []string{
		http.MethodGet,
		http.MethodPut,
		http.MethodPost,
		http.MethodDelete,
		http.MethodPatch,
	} {
		t.Run(method, func(t *testing.T) {
			router := newFakeRouter()
			api := New(router, huma.DefaultConfig("Test API", "1.0.0"))
			called := false
			api.Adapter().Handle(&huma.Operation{Method: method, Path: "/method"}, func(ctx huma.Context) {
				called = true
				ctx.SetStatus(http.StatusNoContent)
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, "/method", nil)
			router.ServeHTTP(recorder, request)

			assert.True(t, called)
			assert.Equal(t, http.StatusNoContent, recorder.Code)
			assert.Empty(t, recorder.Body.String())
		})
	}
}

func TestContext(t *testing.T) {
	op := &huma.Operation{Method: http.MethodPost, Path: "/things/{id}"}
	request := httptest.NewRequest(http.MethodPost, "https://example.com/things/123?search=foo", strings.NewReader("body"))
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Add("X-Test", "one")
	request.Header.Add("X-Test", "two")
	request.SetPathValue("id", "123")
	nativeRequest := &serveHTTPRequest{Request: gofrHTTP.NewRequest(request), r: request}
	native := &gofr.Context{Context: request.Context(), Request: nativeRequest}
	recorder := httptest.NewRecorder()
	ctx := newContext(op, native, request, recorder)

	assert.Same(t, op, ctx.Operation())
	assert.Equal(t, request.Context(), ctx.Context())
	assert.Equal(t, request.Context(), nativeRequest.Context())
	assert.Equal(t, http.MethodPost, ctx.Method())
	assert.Equal(t, "example.com", ctx.Host())
	assert.Equal(t, "127.0.0.1:1234", ctx.RemoteAddr())
	assert.Equal(t, url.URL{Scheme: "https", Host: "example.com", Path: "/things/123", RawQuery: "search=foo"}, ctx.URL())
	assert.Equal(t, "123", ctx.Param("id"))
	assert.Equal(t, "foo", ctx.Query("search"))
	assert.Equal(t, "one", ctx.Header("X-Test"))

	var headers []string
	ctx.EachHeader(func(name, value string) {
		if name == "X-Test" {
			headers = append(headers, value)
		}
	})
	assert.ElementsMatch(t, []string{"one", "two"}, headers)

	body, err := io.ReadAll(ctx.BodyReader())
	require.NoError(t, err)
	assert.Equal(t, "body", string(body))
	require.Error(t, ctx.SetReadDeadline(time.Now()))

	ctx.AppendHeader("X-Output", "one")
	ctx.AppendHeader("X-Output", "two")
	ctx.SetHeader("X-Set", "value")
	ctx.SetStatus(http.StatusCreated)
	_, err = ctx.BodyWriter().Write([]byte("response"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, ctx.Status())
	assert.Equal(t, []string{"one", "two"}, recorder.Header().Values("X-Output"))
	assert.Equal(t, "value", recorder.Header().Get("X-Set"))
	assert.Equal(t, "response", recorder.Body.String())
	assert.NotNil(t, ctx.TLS())
	assert.Equal(t, huma.ProtoVersion{Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1}, ctx.Version())

	wrapped := huma.WithValue(ctx, contextKey{}, "updated")
	assert.Equal(t, "updated", wrapped.Context().Value(contextKey{}))
	assert.Equal(t, "updated", Unwrap(wrapped).Request.Context().Value(contextKey{}))
}

func TestMultipartForm(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("name", "huma"))
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	nativeRequest := &serveHTTPRequest{Request: gofrHTTP.NewRequest(request), r: request}
	native := &gofr.Context{Context: request.Context(), Request: nativeRequest}
	ctx := newContext(&huma.Operation{}, native, request, httptest.NewRecorder())

	form, err := ctx.GetMultipartForm()
	require.NoError(t, err)
	assert.Equal(t, []string{"huma"}, form.Value["name"])
}

func TestResponseWriter(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &responseWriter{ResponseWriter: recorder}

	assert.Same(t, recorder, writer.Unwrap())
	writer.Flush()
	assert.True(t, recorder.Flushed)

	writer.Header().Set("X-Before", "value")
	writer.seal()
	writer.Header().Set("X-After", "ignored")
	writer.WriteHeader(http.StatusCreated)
	written, err := writer.Write([]byte("ignored"))
	require.NoError(t, err)
	writer.Flush()

	assert.Equal(t, len("ignored"), written)
	assert.Equal(t, "value", recorder.Header().Get("X-Before"))
	assert.Empty(t, recorder.Header().Get("X-After"))
	assert.Empty(t, recorder.Body.String())
}
