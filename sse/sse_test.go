package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	router, api := humatest.New()

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
	})

	r, _ := http.NewRequest(http.MethodGet, "/sse", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, `data: {"message":"Hello, world!"}

id: 5
retry: 1000
event: userCreate
data: {"user_id":1,"username":"foo"}

`, w.Body.String())
}
