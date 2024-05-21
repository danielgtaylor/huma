---
description: Stream events from the server to the client over HTTP using Server Sent Events.
---

# Server Sent Events (SSE)

## SSE { .hidden }

The [`sse`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/sse) package provides a helper for streaming [Server-Sent Events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) responses.

SSE is a simple protocol for sending events from the server to the client over HTTP. It is a one-way protocol, meaning that the client cannot send events to the server, but can consume them as they are sent. It is popularly used as a push mechanism for web and other clients.

## Example

The `sse` package provides a simple API for sending events to the client and documents the event types and data structures in the OpenAPI spec if you provide a mapping of message type names to Go structs:

```go title="code.go"
// Register using sse.Register instead of huma.Register
sse.Register(api, huma.Operation{
	OperationID: "sse",
	Method:      http.MethodGet,
	Path:        "/sse",
	Summary:     "Server sent events example",
}, map[string]any{
	// Mapping of event type name to Go struct for that event.
	"message":      DefaultMessage{},
	"userCreate":   UserCreatedEvent{},
	"mailReceived": MailReceivedEvent{},
}, func(ctx context.Context, input *struct{}, send sse.Sender) {
	// Send an event every second for 10 seconds.
	for x := 0; x < 10; x++ {
		send.Data(MailReceivedEvent{UserID: "abc123"})
		time.Sleep(1 * time.Second)
	}
})
```

!!! info "Type Reuse"

    Each event model **must** be a unique Go type. If you want to reuse Go type definitions, you can define a new type referencing another type, e.g. `type MySpecificEvent MyBaseEvent` and it will work as expected.

## Sending Data

The `send Sender` passed to your SSE operation handler provides several ways of sending data to the client:

| Method           | Description                               |
| ---------------- | ----------------------------------------- |
| `send(Message)`  | Send an event using a full message struct |
| `send.Data(any)` | Send a message with the given data        |

Unless you need to set the message ID or retry information, the `send.Data(any)` method is preferred.

## Dive Deeper

-   Reference
    -   [`sse.Register`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/sse#Register)
    -   [`sse.Sender`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/sse#Sender)
-   External Links
    -   [Server Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)
