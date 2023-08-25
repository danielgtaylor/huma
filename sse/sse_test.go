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

type UserEvent struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
}

type UserCreatedEvent UserEvent
type UserDeletedEvent UserEvent

func TestSSE(t *testing.T) {
	_, api := humatest.New(t)

	Register(api, huma.Operation{
		OperationID: "sse",
		Method:      http.MethodGet,
		Path:        "/sse",
	}, map[string]any{
		"message":    &DefaultMessage{},
		"userCreate": UserCreatedEvent{},
		"userDelete": UserDeletedEvent{},
	}, func(ctx context.Context, input *struct{}, send Sender) {
		send.Data(DefaultMessage{Message: "Hello, world!"})

		send(Message{
			ID:    5,
			Retry: 1000,
			Data:  UserCreatedEvent{UserID: 1, Username: "foo"},
		})

		send.Data(UserDeletedEvent{UserID: 2, Username: "bar"})

		// Unknown event type gets sent as the default. Still uses JSON encoding!
		send.Data("unknown event")

		// Encode failure should return an error.
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

event: userDelete
data: {"user_id":2,"username":"bar"}

data: "unknown event"

data: {"error": "encode error: json: unsupported type: chan int"}

`, resp.Body.String())
}
