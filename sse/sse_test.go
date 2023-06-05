package sse

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
)

type DefaultMessage struct {
	Message string `json:"message"`
}

type UserCreatedEvent struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
}

func TestSSE(t *testing.T) {
	_, api := humatest.New(t)

	Register(api, huma.Operation{
		OperationID: "sse",
		Method:      http.MethodGet,
		Path:        "/sse",
	}, map[string]any{
		"message":    &DefaultMessage{},
		"userCreate": UserCreatedEvent{},
	}, func(ctx context.Context, input *struct{}, send func(Message) error) {
		send(Message{
			Data: DefaultMessage{Message: "Hello, world!"},
		})

		send(Message{
			ID:    5,
			Retry: 1000,
			Data:  UserCreatedEvent{UserID: 1, Username: "foo"},
		})

		// Unknown event type gets sent as the default. Still uses JSON encoding!
		send(Message{
			Data: "unknown event",
		})

		// Encode failure should return an error and not write anything.
		assert.Error(t, send(Message{
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

data: "unknown event"

data: {"error": "encode error: json: unsupported type: chan int"}

`, resp.Body.String())
}
