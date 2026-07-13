// Package sse provides utilities for working with Server Sent Events (SSE).
package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	for t.Kind() == reflect.Pointer {
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

// bodyStreamer is an optional interface an adapter's context may implement when
// its response writer can't be flushed synchronously from within the handler
// (e.g. fasthttp/Fiber). StreamBody is called with a writer used to stream the
// response body; the response stays open until the callback returns. The writer
// flushes on demand when it implements http.Flusher and honors write deadlines
// when it implements writeDeadliner.
type bodyStreamer interface {
	StreamBody(func(io.Writer))
}

// Message is a single SSE message. There is no `event` field as this is
// handled by the `eventTypeMap` when registering the operation.
type Message struct {
	ID    int
	Data  any
	Retry int
	// Comment, if set, is written as one or more SSE comment lines (each line
	// prefixed with a colon and ignored by clients). It may accompany an event
	// or, when `Data` is nil, form a comment-only message such as a heartbeat
	// used to keep the connection alive.
	Comment string
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

// Comment sends an SSE comment to the client. Comments are ignored by clients
// and are commonly used as heartbeats to keep the connection alive. This is
// equivalent to calling `sender(Message{Comment: comment})`.
func (s Sender) Comment(comment string) error {
	return s(Message{Comment: comment})
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
					Extensions: map[string]any{
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
			Extensions: map[string]any{
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
				// Commit response headers immediately so the client's
				// EventSource.onopen fires without waiting for the first event.
				ctx.SetStatus(http.StatusOK)

				// Adapters whose response writer can't be flushed synchronously
				// (e.g. Fiber/fasthttp) implement bodyStreamer and stream through
				// a callback; everything else writes to BodyWriter directly.
				if bs, ok := ctx.(bodyStreamer); ok {
					bs.StreamBody(func(w io.Writer) {
						stream(ctx.Context(), w, typeToEvent, input, f)
					})

					return
				}

				stream(ctx.Context(), ctx.BodyWriter(), typeToEvent, input, f)
			},
		}, nil
	})
}

// stream runs the SSE send loop against a single writer: it discovers the
// writer's flush and write-deadline support, builds the send function, and
// invokes the user handler. It is shared by the direct-write path and the
// bodyStreamer callback path.
func stream[I any](reqCtx context.Context, w io.Writer, typeToEvent map[reflect.Type]string, input *I, f func(ctx context.Context, input *I, send Sender)) {
	encoder := json.NewEncoder(w)

	// Get the flusher/deadliner from the writer if possible.
	var flusher http.Flusher
	flushCheck := w

	for {
		if fl, ok := flushCheck.(http.Flusher); ok {
			flusher = fl
			break
		}
		if u, ok := flushCheck.(unwrapper); ok {
			flushCheck = u.Unwrap()
		} else {
			break
		}
	}
	if flusher != nil {
		flusher.Flush()
	}

	var deadliner writeDeadliner
	deadlineCheck := w

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

	flush := func() error {
		if flusher == nil {
			fmt.Fprintln(os.Stderr, "error: unable to flush")
			return fmt.Errorf("unable to flush: %w", http.ErrNotSupported)
		}

		flusher.Flush()

		return nil
	}

	send := func(msg Message) error {
		if deadliner != nil {
			if err := deadliner.SetWriteDeadline(time.Now().Add(WriteTimeout)); err != nil {
				fmt.Fprintf(os.Stderr, "warning: unable to set write deadline: %v\n", err)
			}
		} else {
			fmt.Fprintln(os.Stderr, "write deadline not supported by underlying writer")
		}

		// Write optional fields.
		if msg.ID > 0 {
			w.Write(fmt.Appendf(nil, "id: %d\n", msg.ID))
		}
		if msg.Retry > 0 {
			w.Write(fmt.Appendf(nil, "retry: %d\n", msg.Retry))
		}

		if msg.Comment != "" {
			// CR, LF, and CRLF are all SSE line terminators. Normalize them to
			// LF so every line of the comment is re-emitted as its own ": "
			// comment line and can't inject other fields.
			comment := strings.ReplaceAll(msg.Comment, "\r\n", "\n")
			comment = strings.ReplaceAll(comment, "\r", "\n")
			for line := range strings.SplitSeq(comment, "\n") {
				w.Write(fmt.Appendf(nil, ": %s\n", line))
			}
		}

		if msg.Data == nil {
			w.Write([]byte("\n"))
			return flush()
		}

		event, ok := typeToEvent[deref(reflect.TypeOf(msg.Data))]
		if !ok {
			fmt.Fprintf(os.Stderr, "error: unknown event type %v\n", reflect.TypeOf(msg.Data))
			debug.PrintStack()
		}
		if event != "" && event != "message" {
			// `message` is the default, so no need to transmit it.
			w.Write([]byte("event: " + event + "\n"))
		}

		// Write the message data.
		if _, err := w.Write([]byte("data: ")); err != nil {
			return err
		}
		if err := encoder.Encode(msg.Data); err != nil {
			w.Write([]byte(`{"error": "encode error: `))
			w.Write([]byte(err.Error()))
			w.Write([]byte("\"}\n\n"))
			return err
		}
		w.Write([]byte("\n"))
		return flush()
	}

	// Call the user-provided SSE handler.
	f(reqCtx, input, send)
}
