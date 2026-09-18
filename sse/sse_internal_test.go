package sse

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
)

// flushRecorder is an io.Writer that also implements http.Flusher, standing in
// for the stream writer an adapter hands to bodyStreamer.StreamBody.
type flushRecorder struct {
	bytes.Buffer
	flushes int
}

func (f *flushRecorder) Flush() { f.flushes++ }

// streamContext is a minimal huma.Context that also implements bodyStreamer, so
// the Fiber/fasthttp streaming path can be exercised without importing an
// adapter. It records what the SSE code writes to its stream writer.
type streamContext struct {
	op     *huma.Operation
	header http.Header
	status int
	body   *flushRecorder
}

func (c *streamContext) Operation() *huma.Operation                 { return c.op }
func (c *streamContext) Context() context.Context                   { return context.Background() }
func (c *streamContext) TLS() *tls.ConnectionState                  { return nil }
func (c *streamContext) Version() huma.ProtoVersion                 { return huma.ProtoVersion{} }
func (c *streamContext) Method() string                             { return http.MethodGet }
func (c *streamContext) Host() string                               { return "" }
func (c *streamContext) RemoteAddr() string                         { return "" }
func (c *streamContext) URL() url.URL                               { return url.URL{} }
func (c *streamContext) Param(string) string                        { return "" }
func (c *streamContext) Query(string) string                        { return "" }
func (c *streamContext) Header(string) string                       { return "" }
func (c *streamContext) EachHeader(func(string, string))            {}
func (c *streamContext) BodyReader() io.Reader                      { return http.NoBody }
func (c *streamContext) GetMultipartForm() (*multipart.Form, error) { return nil, nil }
func (c *streamContext) SetReadDeadline(time.Time) error            { return nil }
func (c *streamContext) SetStatus(code int)                         { c.status = code }
func (c *streamContext) Status() int                                { return c.status }
func (c *streamContext) SetHeader(name, value string)               { c.header.Set(name, value) }
func (c *streamContext) AppendHeader(name, value string)            { c.header.Add(name, value) }
func (c *streamContext) BodyWriter() io.Writer                      { return c.body }
func (c *streamContext) StreamBody(fn func(io.Writer))              { fn(c.body) }

// streamAdapter is a huma.Adapter that drives a single registered operation
// through a streamContext, exercising the real Register wiring.
type streamAdapter struct {
	handler func(huma.Context)
	ctx     *streamContext
}

func (a *streamAdapter) Handle(_ *huma.Operation, handler func(huma.Context)) {
	a.handler = handler
}

func (a *streamAdapter) ServeHTTP(http.ResponseWriter, *http.Request) {
	a.handler(a.ctx)
}

// TestRegisterStreamsViaBodyStreamer covers the bodyStreamer branch in Register,
// which real adapters only reach via Fiber/fasthttp. It confirms events and
// comments are written to and flushed through the adapter's stream writer.
func TestRegisterStreamsViaBodyStreamer(t *testing.T) {
	type message struct {
		Text string `json:"text"`
	}

	adapter := &streamAdapter{ctx: &streamContext{
		op:     &huma.Operation{OperationID: "sse", Method: http.MethodGet, Path: "/sse"},
		header: http.Header{},
		body:   &flushRecorder{},
	}}
	api := huma.NewAPI(huma.DefaultConfig("Test", "1.0.0"), adapter)

	Register(api, huma.Operation{
		OperationID: "sse",
		Method:      http.MethodGet,
		Path:        "/sse",
	}, map[string]any{"message": message{}}, func(_ context.Context, _ *struct{}, send Sender) {
		_ = send.Data(message{Text: "hi"})
		_ = send.Comment("ping")
	})

	adapter.ServeHTTP(nil, nil)

	assert.Equal(t, "text/event-stream", adapter.ctx.header.Get("Content-Type"))
	assert.Equal(t, http.StatusOK, adapter.ctx.status)
	assert.Equal(t, "data: {\"text\":\"hi\"}\n\n: ping\n\n", adapter.ctx.body.String())
	assert.Positive(t, adapter.ctx.body.flushes)
}
