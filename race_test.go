package huma_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// When a chi middleware reads the request body before Huma's handler wrapper,
// net/http's body reader hits EOF early and starts a background read on the
// connection. Huma's BodyReadTimeout must not be left on that background read
// after the request body has been successfully read. If it is, a slow handler
// can cause the background read to time out and net/http cancels the
// connection-level context.
//
// Huma should clear the read deadline after the body has been read so the
// timeout only applies to body reading, not the rest of the handler lifetime.

// bodyReadingMiddleware simulates the old signature middleware: reads the entire
// body, then restores it for downstream handlers.
func bodyReadingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		// Restore body for Huma.
		r.Body = io.NopCloser(bytes.NewReader(body))
		next.ServeHTTP(w, r)
	})
}

type slowInput struct {
	RawBody []byte
	Body    struct {
		Value string `json:"value"`
	}
}

type slowOutput struct {
	Body struct {
		OK        bool   `json:"ok"`
		CtxErr    string `json:"ctx_err,omitempty"`
		Broken    bool   `json:"broken"`
		Cancelled bool   `json:"cancelled"`
	}
}

// newSlowHandler returns a huma handler that sleeps for the given duration,
// then reports whether the context was canceled during the sleep.
func newSlowHandler(sleep time.Duration) func(context.Context, *slowInput) (*slowOutput, error) {
	return func(ctx context.Context, input *slowInput) (*slowOutput, error) {
		timer := time.NewTimer(sleep)
		defer timer.Stop()

		out := &slowOutput{}

		select {
		case <-ctx.Done():
			out.Body.Broken = true
			out.Body.Cancelled = true
			out.Body.CtxErr = ctx.Err().Error()
			return out, nil
		default:
		}

		select {
		case <-ctx.Done():
			out.Body.Cancelled = true
			out.Body.CtxErr = ctx.Err().Error()
			return out, nil
		case <-timer.C:
			out.Body.OK = true
			return out, nil
		}
	}
}

// startServer creates a chi router with huma, registers the handler on the
// given path, optionally wraps it with middleware, and starts a real TCP server.
// Returns the base URL and a cleanup function.
func startServer(t *testing.T, withMiddleware bool, bodyReadTimeout time.Duration, handlerSleep time.Duration) (string, func()) {
	t.Helper()

	router := chi.NewRouter()

	if withMiddleware {
		router.Use(bodyReadingMiddleware)
	}

	config := huma.DefaultConfig("Test", "1.0.0")
	api := humachi.New(router, config)

	op := huma.Operation{
		Path:            "/webhook",
		Method:          http.MethodPost,
		DefaultStatus:   http.StatusOK,
		BodyReadTimeout: bodyReadTimeout,
	}
	huma.Register(api, op, newSlowHandler(handlerSleep))

	// Use a real TCP listener so SetReadDeadline works on the connection.
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{Handler: router}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = server.Serve(listener)
	}()

	baseURL := "http://" + listener.Addr().String()
	cleanup := func() {
		_ = server.Close()
		wg.Wait()
	}

	return baseURL, cleanup
}

// newKeepAliveClient returns an HTTP client that reuses connections (keep-alive).
func newKeepAliveClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     30 * time.Second,
			DisableKeepAlives:   false,
		},
		Timeout: 30 * time.Second,
	}
}

func postJSON(client *http.Client, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}

// TestMiddlewareBodyRead_DoesNotCancelContext verifies that a body-reading
// middleware + BodyReadTimeout + slow handler does not cancel the context after
// Huma has finished reading the restored request body.
func TestMiddlewareBodyRead_DoesNotCancelContext(t *testing.T) {
	// BodyReadTimeout = 500ms, handler sleeps 1s. The read deadline should be
	// cleared after body read, so the handler context stays alive.
	bodyReadTimeout := 500 * time.Millisecond
	handlerSleep := 1 * time.Second
	payload := []byte(`{"value":"test"}`)

	baseURL, cleanup := startServer(t, true, bodyReadTimeout, handlerSleep)
	defer cleanup()

	client := newKeepAliveClient()
	defer client.CloseIdleConnections()

	url := baseURL + "/webhook"

	resp1, err := postJSON(client, url, payload)
	require.NoError(t, err)

	body1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	t.Logf("Request 1 (with middleware): status=%d body=%s", resp1.StatusCode, string(body1))

	assert.Contains(t, string(body1), `"cancelled":false`,
		"handler should not observe context cancellation after middleware body read")
	assert.Contains(t, string(body1), `"broken":false`,
		"handler should not start with a canceled context after middleware body read")
	assert.Contains(t, string(body1), `"ok":true`)
}

// TestNoMiddleware_NoContextCancel demonstrates that without the body-reading
// middleware, the same handler + BodyReadTimeout does NOT cause context cancellation.
func TestNoMiddleware_NoContextCancel(t *testing.T) {
	// Same timeout/sleep as above, but no middleware.
	bodyReadTimeout := 500 * time.Millisecond
	handlerSleep := 1 * time.Second
	payload := []byte(`{"value":"test"}`)

	baseURL, cleanup := startServer(t, false, bodyReadTimeout, handlerSleep)
	defer cleanup()

	client := newKeepAliveClient()
	defer client.CloseIdleConnections()

	url := baseURL + "/webhook"

	// Without middleware, Huma reads the body itself:
	// 1. Huma sets SetReadDeadline(now + 500ms)
	// 2. Huma reads body (consuming the deadline for its intended purpose)
	// 3. startBackgroundRead fires after body is read
	// 4. Handler sleeps 1s, but backgroundRead's deadline was already satisfied
	//    during the body read phase
	resp1, err := postJSON(client, url, payload)
	require.NoError(t, err)

	body1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	t.Logf("Request 1 (no middleware): status=%d body=%s", resp1.StatusCode, string(body1))

	assert.Contains(t, string(body1), `"cancelled":false`,
		"handler should NOT observe context cancellation without middleware")
	assert.Contains(t, string(body1), `"ok":true`)
}

// TestKeepAlive_SecondRequestUsesFreshContext verifies that a slow request with
// a body-reading middleware does not poison the keep-alive connection for the
// next request.
func TestKeepAlive_SecondRequestUsesFreshContext(t *testing.T) {
	bodyReadTimeout := 500 * time.Millisecond
	handlerSleep := 1 * time.Second
	payload := []byte(`{"value":"test"}`)

	baseURL, cleanup := startServer(t, true, bodyReadTimeout, handlerSleep)
	defer cleanup()

	client := newKeepAliveClient()
	defer client.CloseIdleConnections()

	url := baseURL + "/webhook"

	resp1, err := postJSON(client, url, payload)
	require.NoError(t, err)
	body1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	t.Logf("Request 1 (with middleware, keep-alive): status=%d body=%s", resp1.StatusCode, string(body1))
	assert.Contains(t, string(body1), `"cancelled":false`)

	// Brief pause to let the keep-alive connection settle.
	time.Sleep(100 * time.Millisecond)

	resp2, err := postJSON(client, url, payload)
	require.NoError(t, err)

	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	t.Logf("Request 2 (with middleware, keep-alive): status=%d body=%s", resp2.StatusCode, string(body2))

	assert.Contains(t, string(body2), `"cancelled":false`,
		"second keep-alive request should use a live request context")
	assert.Contains(t, string(body2), `"broken":false`,
		"second keep-alive request should not inherit a canceled connection context")
	assert.Contains(t, string(body2), `"ok":true`)
}
