// Package sse provides utilities for working with Server Sent Events (SSE).
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// WriteTimeout is the timeout for writing to the client.
var WriteTimeout = 5 * time.Second

// deref follows pointers until it finds a non-pointer type.
func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

// Message is a single SSE message. There is no `event` field as this is
// handled by the `eventTypeMap` when registering the operation.
type Message struct {
	ID    int
	Data  any
	Retry int
}

// Register a new SSE operation. The `eventTypeMap` maps from event name to
// the type of the data that will be sent. The `f` function is called with
// the context, input, and a `send` function that can be used to send messages
// to the client. Flushing is handled automatically as long as the adapter's
// `BodyWriter` implements `http.Flusher`.
func Register[I any](api huma.API, op huma.Operation, eventTypeMap map[string]any, f func(ctx context.Context, input *I, send func(Message) error)) {
	// Start by defining the SSE schema & operation response.
	if op.Responses == nil {
		op.Responses = map[string]*huma.Response{}
	}
	if op.Responses["200"] == nil {
		op.Responses["200"] = &huma.Response{}
	}
	if op.Responses["200"].Content == nil {
		op.Responses["200"].Content = map[string]*huma.MediaType{}
	}

	typeToEvent := make(map[reflect.Type]string, len(eventTypeMap))
	dataSchemas := make([]*huma.Schema, len(eventTypeMap))
	for k, v := range eventTypeMap {
		vt := deref(reflect.TypeOf(v))
		typeToEvent[vt] = k
		required := []string{"data"}
		if k != "" && k != "message" {
			required = append(required, "event")
		}
		s := &huma.Schema{
			Title: "Event " + k,
			Type:  huma.TypeObject,
			Properties: map[string]*huma.Schema{
				"id": {
					Type:        huma.TypeInteger,
					Description: "The event ID.",
				},
				"event": {
					Type:        huma.TypeString,
					Description: "The event name.",
					Extensions: map[string]interface{}{
						"const": k,
					},
				},
				"data": api.OpenAPI().Components.Schemas.Schema(vt, true, k),
				"retry": {
					Type:        huma.TypeInteger,
					Description: "The retry time in milliseconds.",
				},
			},
			Required: required,
		}

		dataSchemas = append(dataSchemas, s)
	}

	schema := &huma.Schema{
		Title:       "Server Sent Events",
		Description: "Each oneOf object in the array represents one possible Server Sent Events (SSE) message, serialized as UTF-8 text according to the SSE specification.",
		Type:        huma.TypeArray,
		Items: &huma.Schema{
			Extensions: map[string]interface{}{
				"oneOf": dataSchemas,
			},
		},
	}
	op.Responses["200"].Content["text/event-stream"] = &huma.MediaType{
		Schema: schema,
	}

	// Register the operation with the API, using the built-in streaming
	// response callback functionality. This will call the user's `f` function
	// and provide a `send` function to simplify sending messages.
	huma.Register(api, op, func(ctx context.Context, input *I) (*huma.StreamResponse, error) {
		return &huma.StreamResponse{
			Body: func(ctx huma.Context) {
				ctx.WriteHeader("Content-Type", "text/event-stream")
				bw := ctx.BodyWriter()
				encoder := json.NewEncoder(bw)
				send := func(msg Message) error {
					if d, ok := bw.(interface{ SetWriteDeadline(time.Time) error }); ok {
						d.SetWriteDeadline(time.Now().Add(WriteTimeout))
					} else {
						fmt.Println("warning: unable to set write deadline")
					}

					// Write optional fields
					if msg.ID > 0 {
						bw.Write([]byte(fmt.Sprintf("id: %d\n", msg.ID)))
					}
					if msg.Retry > 0 {
						bw.Write([]byte(fmt.Sprintf("retry: %d\n", msg.Retry)))
					}

					event, ok := typeToEvent[deref(reflect.TypeOf(msg.Data))]
					if !ok {
						fmt.Println("error: unknown event type", reflect.TypeOf(msg.Data))
						debug.PrintStack()
					}
					if event != "" && event != "message" {
						// `message` is the default, so no need to transmit it.
						bw.Write([]byte("event: " + event + "\n"))
					}

					// Write the message data.
					if _, err := bw.Write([]byte("data: ")); err != nil {
						return err
					}
					if err := encoder.Encode(msg.Data); err != nil {
						return err
					}
					bw.Write([]byte("\n"))
					if f, ok := bw.(http.Flusher); ok {
						f.Flush()
					} else {
						fmt.Println("warning: unable to flush")
						return fmt.Errorf("unable to flush: %w", http.ErrNotSupported)
					}
					return nil
				}

				// Call the user-provided SSE handler.
				f(ctx.GetContext(), input, send)
			},
		}, nil
	})
}
