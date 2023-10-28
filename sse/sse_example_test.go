package sse_test

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/go-chi/chi/v5"
)

func ExampleRegister_sse() {
	// 1. Define some message types.
	type DefaultMessage struct {
		Message string `json:"message"`
	}

	type UserEvent struct {
		UserID   int    `json:"user_id"`
		Username string `json:"username"`
	}

	type UserCreatedEvent UserEvent
	type UserDeletedEvent UserEvent

	// 2. Set up the API.
	router := chi.NewMux()
	api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

	// 3. Register an SSE operation.
	sse.Register(api, huma.Operation{
		OperationID: "sse",
		Method:      http.MethodGet,
		Path:        "/sse",
	}, map[string]any{
		// Map each event name to a message type.
		"message":    &DefaultMessage{},
		"userCreate": UserCreatedEvent{},
		"userDelete": UserDeletedEvent{},
	}, func(ctx context.Context, input *struct{}, send sse.Sender) {
		// Use `send.Data` to send a message with the event type set to the
		// corresponding registered type from the map above. For this example,
		// it will send "message" as the type.
		send.Data(DefaultMessage{Message: "Hello, world!"})

		// Use `send` for more control, letting you set an ID and retry interval.
		// The event type is still controlled by the map above and type passed
		// as data below, in this case "userCreate" is sent.
		send(sse.Message{
			ID:    5,
			Retry: 1000,
			Data:  UserCreatedEvent{UserID: 1, Username: "foo"},
		})

		// Example "userDelete" event type.
		send.Data(UserDeletedEvent{UserID: 2, Username: "bar"})

		// Unknown event type gets sent as the default. Still uses JSON encoding!
		send.Data("unknown event")
	})
}
