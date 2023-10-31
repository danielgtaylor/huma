// This example shows how to use Server Sent Events (SSE) with Huma to send
// messages to a client over a long-lived connection.
//
//	# Connect and start getting messages
//	curl localhost:8888/sse
//
// You can connect an arbitrary number of clients in many tabs/terminals and
// they will all receive the same data from a single producer on the server.
// Disconnect (Ctrl-C) & reconnect to continue the sequence.
//
//	                      /--> channel -> Client 1
//	Producer -> SSE message -> channel -> Client 2
//	                      \--> channel -> Client 3
package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/sse"
	"github.com/go-chi/chi/v5"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" default:"8888"`
}

// First, define your SSE message types. These can be any struct you want and
// will be serialized as JSON and sent to the client as the message data.
type NumberMessage struct {
	Value int `json:"value"`
}

type SpecialMessage struct {
	Value string `json:"value"`
}

// Producer is a struct that will produce messages to send to clients. It runs
// in a goroutine and produces the FizzBuzz sequence from 1 to 1000 over and
// over, generating one message per 500ms.
type Producer struct {
	// Cancel is a channel that can be used to stop the producer.
	Cancel chan bool

	// clients keeps track of all connected clients, which enables us to send
	// messages to each of them when they are produced.
	clients []chan any

	// mu is a mutex to protect the clients slice from concurrent access.
	mu sync.Mutex
}

// AddClient adds a new client to the producer. Each message that is produced
// will be sent to each registered client.
func (p *Producer) AddClient(client chan any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.clients = append(p.clients, client)
}

// Produce the FizzBuzz sequence forever, starting from 1 again once 1000 is
// reached.
func (p *Producer) Produce() {
	for i := 1; true; i = (i + 1) % 1000 {
		// Emit our message!
		if i%3 == 0 && i%5 == 0 {
			p.emit(SpecialMessage{"fizzbuzz"})
		} else if i%3 == 0 {
			p.emit(SpecialMessage{"fizz"})
		} else if i%5 == 0 {
			p.emit(SpecialMessage{"buzz"})
		} else {
			p.emit(NumberMessage{i})
		}

		// Now, we want to either wait for 500ms or until we are canceled. If
		// canceled, we need to shut everything down nicely.
		select {
		case <-time.After(500 * time.Millisecond):
			// Not canceled, so continue the loop.
		case <-p.Cancel:
			fmt.Println("Stopping producer...")
			p.mu.Lock()
			defer p.mu.Unlock()
			for _, client := range p.clients {
				// Close each client channel to signal that we are done. This should
				// let each connected client disconnect gracefully.
				close(client)
			}
			return
		}
	}
}

// emit a message to all registered clients.
func (p *Producer) emit(data any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Loop backwards so we can remove dead clients without messing up the
	// indexes / traversal. This has a side-effect which may be undesirable of
	// the most recently connected clients getting the messages first.
	for i := len(p.clients) - 1; i >= 0; i-- {
		select {
		case p.clients[i] <- data:
			// Do nothing, send was successful.
		default:
			// Could not send...remove this client as it is dead!
			close(p.clients[i])
			p.clients = append(p.clients[:i], p.clients[i+1:]...)
		}
	}
}

func main() {
	// Create a CLI app which takes a port option.
	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		// Create a producer to generate messages for clients.
		p := Producer{Cancel: make(chan bool, 1)}

		// Register the SSE operation. This is similar to registering a normal
		// operation but takes in a map of message types to structs and a special
		// handler function.
		sse.Register(api, huma.Operation{
			OperationID: "sse",
			Method:      http.MethodGet,
			Path:        "/sse",
		}, map[string]any{
			// Register each message type with an event name.
			"number":  NumberMessage{},
			"special": SpecialMessage{},
		}, func(ctx context.Context, input *struct{}, send sse.Sender) {
			// Register this connection as a new client and start sending messages
			// as they come in from the producer.
			c := make(chan any, 1)
			p.AddClient(c)

			for {
				select {
				case data, ok := <-c:
					if !ok {
						// Channel was closed, so we are done.
						return
					}
					if err := send.Data(data); err != nil {
						return
					}
				case <-ctx.Done():
					// Context was canceled, so we are done.
					return
				}
			}
		})

		// Tell the CLI how to start & stop your server.
		s := http.Server{
			Addr:    fmt.Sprintf(":%d", options.Port),
			Handler: router,
		}

		hooks.OnStart(func() {
			// Start the producer in a goroutine, then start the HTTP server.
			go p.Produce()
			s.ListenAndServe()
		})

		hooks.OnStop(func() {
			p.Cancel <- true
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.Shutdown(ctx)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
