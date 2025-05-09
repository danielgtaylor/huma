// Package sse provides utilities for working with Server Sent Events (SSE).
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"
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

type unwrapper interface {
	Unwrap() http.ResponseWriter
}

type writeDeadliner interface {
	SetWriteDeadline(time.Time) error
}

// Message is a single SSE message. There is no `event` field as this is
// handled by the `eventTypeMap` when registering the operation.
type Message struct {
	ID    int
	Data  any
	Retry int
}

// Sender is a send function for sending SSE messages to the client. It is
// callable but also provides a `sender.Data(...)` convenience method if
// you don't need to set the other fields in the message.
type Sender func(Message) error

// Data sends a message with the given data to the client. This is equivalent
// to calling `sender(Message{Data: data})`.
func (s Sender) Data(data any) error {
	return s(Message{Data: data})
}

// Register a new SSE operation. The `eventTypeMap` maps from event name to
// the type of the data that will be sent. The `f` function is called with
// the context, input, and a `send` function that can be used to send messages
// to the client. Flushing is handled automatically as long as the adapter's
// `BodyWriter` implements `http.Flusher`.
func Register[I any](api huma.API, op huma.Operation, eventTypeMap map[string]any, f func(ctx context.Context, input *I, send Sender)) {
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
	dataSchemas := make([]*huma.Schema, 0, len(eventTypeMap))
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

	slices.SortFunc(dataSchemas, func(b, c *huma.Schema) int {
		return strings.Compare(b.Title, c.Title)
	})

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
				ctx.SetHeader("Content-Type", "text/event-stream")
				bw := ctx.BodyWriter()
				encoder := json.NewEncoder(bw)

				// Get the flusher/deadliner from the response writer if possible.
				var flusher http.Flusher
				flushCheck := bw
				for {
					if f, ok := flushCheck.(http.Flusher); ok {
						flusher = f
						break
					}
					if u, ok := flushCheck.(unwrapper); ok {
						flushCheck = u.Unwrap()
					} else {
						break
					}
				}

				var deadliner writeDeadliner
				deadlineCheck := bw
				for {
					if d, ok := deadlineCheck.(writeDeadliner); ok {
						deadliner = d
						break
					}
					if u, ok := deadlineCheck.(unwrapper); ok {
						deadlineCheck = u.Unwrap()
					} else {
						break
					}
				}

				send := func(msg Message) error {
					if deadliner != nil {
						if err := deadliner.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
							fmt.Fprintf(os.Stderr, "warning: unable to set write deadline: %v\n", err)
						}
					} else {
						fmt.Fprintln(os.Stderr, "write deadline not supported by underlying writer")
					}

					// Write optional fields
					if msg.ID > 0 {
						bw.Write(fmt.Appendf(nil, "id: %d\n", msg.ID))
					}
					if msg.Retry > 0 {
						bw.Write(fmt.Appendf(nil, "retry: %d\n", msg.Retry))
					}

					event, ok := typeToEvent[deref(reflect.TypeOf(msg.Data))]
					if !ok {
						fmt.Fprintf(os.Stderr, "error: unknown event type %v\n", reflect.TypeOf(msg.Data))
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
						bw.Write([]byte(`{"error": "encode error: `))
						bw.Write([]byte(err.Error()))
						bw.Write([]byte("\"}\n\n"))
						return err
					}
					bw.Write([]byte("\n"))
					if flusher != nil {
						flusher.Flush()
					} else {
						fmt.Fprintln(os.Stderr, "error: unable to flush")
						return fmt.Errorf("unable to flush: %w", http.ErrNotSupported)
					}
					return nil
				}

				// Call the user-provided SSE handler.
				f(ctx.Context(), input, send)
			},
		}, nil
	})
}
