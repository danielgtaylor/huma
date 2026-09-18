package sse_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type DefaultMessage struct {
	Message string `json:"message"`
}

type UserEvent struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
}

type UserCreatedEvent UserEvent
type UserDeletedEvent UserEvent

type AMessage UserEvent
type ZMessage UserEvent

type DummyWriter struct {
	writeErr    error
	deadlineErr error
}

func (w *DummyWriter) Header() http.Header {
	return http.Header{}
}

func (w *DummyWriter) Write(p []byte) (n int, err error) {
	return len(p), w.writeErr
}

func (w *DummyWriter) WriteHeader(statusCode int) {}

func (w *DummyWriter) Unwrap() http.ResponseWriter {
	return &WrappedDeadliner{deadlineErr: w.deadlineErr}
}

type WrappedDeadliner struct {
	http.ResponseWriter
	deadlineErr error
}

func (w *WrappedDeadliner) SetWriteDeadline(t time.Time) error {
	return w.deadlineErr
}

// orderedWriter records the order in which WriteHeader, Write, and Flush
// are called so tests can assert headers are committed before body writes.
type orderedWriter struct {
	events []string
	status int
}

func (w *orderedWriter) Header() http.Header {
	return http.Header{}
}

func (w *orderedWriter) Write(p []byte) (int, error) {
	w.events = append(w.events, "write")
	return len(p), nil
}

func (w *orderedWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.events = append(w.events, "writeHeader")
}

func (w *orderedWriter) Flush() {
	w.events = append(w.events, "flush")
}

func (w *orderedWriter) SetWriteDeadline(t time.Time) error {
	return nil
}

type sseTest struct {
	Title    string
	TestFunc func(t *testing.T)
}

var sseTests = []sseTest{
	{
		Title: "sse",
		TestFunc: func(t *testing.T) {
			_, api := humatest.New(t)
			sse.Register(api, huma.Operation{
				OperationID: "sse",
				Method:      http.MethodGet,
				Path:        "/sse",
			}, map[string]any{
				"message":    &DefaultMessage{},
				"userCreate": UserCreatedEvent{},
				"userDelete": UserDeletedEvent{},
			}, func(ctx context.Context, input *struct{}, send sse.Sender) {
				send.Data(DefaultMessage{Message: "Hello, world!"})

				send(sse.Message{
					ID:    5,
					Retry: 1000,
					Data:  UserCreatedEvent{UserID: 1, Username: "foo"},
				})

				send.Data(UserDeletedEvent{UserID: 2, Username: "bar"})

				// Unknown event type gets sent as the default. Still uses JSON encoding!
				send.Data("unknown event")

				// Encode failure should return an error.
				require.Error(t, send(sse.Message{
					Data: make(chan int),
				}))
			})

			resp := api.Get("/sse")

			assert.Equal(t, http.StatusOK, resp.Code)
			assert.Equal(t, "text/event-stream", resp.Header().Get("Content-Type"))
			assert.Equal(t, `data: {"message":"Hello, world!"}

id: 5
retry: 1000
event: userCreate
data: {"user_id":1,"username":"foo"}

event: userDelete
data: {"user_id":2,"username":"bar"}

data: "unknown event"

data: {"error": "encode error: json: unsupported type: chan int"}

`, resp.Body.String())

			// Test write error doesn't panic
			w := &DummyWriter{writeErr: errors.New("whoops")}
			req, _ := http.NewRequest(http.MethodGet, "/sse", nil)
			api.Adapter().ServeHTTP(w, req)

			// Test inability to flush doesn't panic
			w = &DummyWriter{}
			req, _ = http.NewRequest(http.MethodGet, "/sse", nil)
			api.Adapter().ServeHTTP(w, req)

			// Test inability to set write deadline due to error doesn't panic
			w = &DummyWriter{deadlineErr: errors.New("whoops")}
			req, _ = http.NewRequest(http.MethodGet, "/sse", nil)
			api.Adapter().ServeHTTP(w, req)
		},
	},
	{
		Title: "sse comment heartbeat",
		TestFunc: func(t *testing.T) {
			_, api := humatest.New(t)
			sse.Register(api, huma.Operation{
				OperationID: "sse",
				Method:      http.MethodGet,
				Path:        "/sse",
			}, map[string]any{
				"message": &DefaultMessage{},
			}, func(ctx context.Context, input *struct{}, send sse.Sender) {
				send.Comment("heartbeat")
				send(sse.Message{ID: 5, Comment: "keep\nalive"})
				send.Data(DefaultMessage{Message: "Hello, world!"})
			})

			resp := api.Get("/sse")

			assert.Equal(t, http.StatusOK, resp.Code)
			assert.Equal(t, `: heartbeat

id: 5
: keep
: alive

data: {"message":"Hello, world!"}

`, resp.Body.String())
		},
	},
	{
		Title: "sse comment normalizes line breaks",
		TestFunc: func(t *testing.T) {
			_, api := humatest.New(t)
			sse.Register(api, huma.Operation{
				OperationID: "sse",
				Method:      http.MethodGet,
				Path:        "/sse",
			}, map[string]any{
				"message": &DefaultMessage{},
			}, func(ctx context.Context, input *struct{}, send sse.Sender) {
				// CR, CRLF, and LF are all SSE line terminators. Each line must
				// be re-prefixed with a colon so an embedded line break can't
				// inject another SSE field (here a stray "data:" line).
				send.Comment("a\rb\r\nc\ndata: not-injected")
			})

			resp := api.Get("/sse")

			assert.Equal(t, http.StatusOK, resp.Code)
			assert.Equal(t, `: a
: b
: c
: data: not-injected

`, resp.Body.String())
		},
	},
	{
		Title: "sse flushes headers before first event",
		TestFunc: func(t *testing.T) {
			_, api := humatest.New(t)
			started := make(chan struct{})
			release := make(chan struct{})
			sse.Register(api, huma.Operation{
				OperationID: "sse",
				Method:      http.MethodGet,
				Path:        "/sse",
			}, map[string]any{
				"message": &DefaultMessage{},
			}, func(ctx context.Context, input *struct{}, send sse.Sender) {
				close(started)
				<-release
			})

			w := &orderedWriter{}
			req, _ := http.NewRequest(http.MethodGet, "/sse", nil)
			done := make(chan struct{})
			go func() {
				api.Adapter().ServeHTTP(w, req)
				close(done)
			}()
			<-started
			// At this point the user handler is running but has not sent any
			// events yet. Headers must already be committed and flushed so
			// that EventSource.onopen fires on the client.
			eventsBeforeRelease := append([]string(nil), w.events...)
			close(release)
			<-done

			assert.Equal(t, http.StatusOK, w.status)
			require.Contains(t, eventsBeforeRelease, "writeHeader",
				"WriteHeader must be called before the user handler blocks")
			require.Contains(t, eventsBeforeRelease, "flush",
				"Flush must be called before the user handler blocks")
			whIdx := slices.Index(eventsBeforeRelease, "writeHeader")
			flushIdx := slices.Index(eventsBeforeRelease, "flush")
			assert.Less(t, whIdx, flushIdx, "WriteHeader must precede Flush")
			assert.NotContains(t, eventsBeforeRelease, "write",
				"no body write should occur before the user handler sends an event")
		},
	},
	{
		Title: "sse stable event order in openapi",
		TestFunc: func(t *testing.T) {
			_, api := humatest.New(t)
			sse.Register(api, huma.Operation{
				OperationID: "sse",
				Method:      http.MethodGet,
				Path:        "/sse",
			}, map[string]any{
				"message":    &DefaultMessage{},
				"userCreate": UserCreatedEvent{},
				"zMessage":   ZMessage{},
				"userDelete": UserDeletedEvent{},
				"aMessage":   AMessage{},
			}, func(ctx context.Context, input *struct{}, send sse.Sender) {
				send.Data(DefaultMessage{Message: "Hello, world!"})
			})

			o := api.OpenAPI()
			events := o.Paths["/sse"].Get.Responses["200"].Content["text/event-stream"].Schema.Items.Extensions["oneOf"].([]*huma.Schema)
			titles := make([]string, len(events))
			for i, event := range events {
				titles[i] = event.Title
			}
			assert.Equal(t, []string{
				"Event aMessage",
				"Event message",
				"Event userCreate",
				"Event userDelete",
				"Event zMessage",
			}, titles)
		},
	},
}

func TestSSE(t *testing.T) {
	for _, test := range sseTests {
		t.Run(test.Title, func(t *testing.T) {
			test.TestFunc(t)
		})
	}
}
