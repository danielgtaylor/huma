---
description: Stream data back to the client using a long-lived connection to support things like Server Sent Events.
---

# Streaming

## Streaming { .hidden }

The response `Body` can be a callback function taking a [`huma.Context`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Context) to facilitate streaming. The [`huma.StreamResponse`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#StreamResponse) utility makes this easy to return:

```go title="code.go"
func handler(ctx context.Context, input *MyInput) (*huma.StreamResponse, error) {
	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			// Write header info before streaming the body.
			ctx.SetHeader("Content-Type", "text/my-stream")
			writer := ctx.BodyWriter()

			// Update the write deadline to give us extra time.
			if d, ok := writer.(interface{ SetWriteDeadline(time.Time) error }); ok {
				d.SetWriteDeadline(time.Now().Add(5 * time.Second))
			} else {
				fmt.Println("warning: unable to set write deadline")
			}

			// Write the first message, then flush and wait.
			writer.Write([]byte("Hello, I'm streaming!"))
			if f, ok := writer.(http.Flusher); ok {
				f.Flush()
			} else {
				fmt.Println("error: unable to flush")
			}

			time.Sleep(3 * time.Second)

			// Write the second message.
			writer.Write([]byte("Hello, I'm still streaming!"))
		},
	}, nil
}
```

Also take a look at [`http.ResponseController`](https://pkg.go.dev/net/http#ResponseController) which can be used to set timeouts, flush, etc in one simple interface.

!!! info "Server Sent Events"

    The [`sse`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2/sse) package provides a helper for streaming Server-Sent Events (SSE) responses that is easier to use than the above example!

## Dive Deeper

-   Reference
    -   [`huma.Context`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#Context) a router-agnostic request/response context
    -   [`huma.StreamResponse`](https://pkg.go.dev/github.com/danielgtaylor/huma/v2#StreamResponse) for streaming output
-   External Links
    -   [Server Sent Events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events) for one-way streaming
