package sse_test

import (
	"context"
	"errors"
	"net/http"
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
