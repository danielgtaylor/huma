package huma

type Middlewares []func(Handler) Handler

// The HandlerFunc type is an adapter to allow the use of
// ordinary functions as Middleware handlers. If f is a function
// with the appropriate signature, HandlerFunc(f) is a
// Handler that calls f.
type HandlerFunc func(api API, ctx Context)

// Handle calls f(api, ctx).
func (f HandlerFunc) Handle(api API, ctx Context) {
	f(api, ctx)
}

type Handler interface {
	Handle(api API, ctx Context)
}

// Chain returns a Middlewares type from a slice of middleware handlers.
func Chain(middlewares ...func(Handler) Handler) Middlewares {
	return Middlewares(middlewares)
}

// Handler builds and returns a Handler from the chain of middlewares,
// with `h Handler` as the final handler.
func (mws Middlewares) Handler(h Handler) Handler {
	return &ChainHandler{h, chain(mws, h), mws}
}

// HandlerFunc builds and returns a Middleware from the chain of middlewares,
// with `h HandlerFunc` as the final handler.
func (mws Middlewares) HandlerFunc(h HandlerFunc) Handler {
	return &ChainHandler{h, chain(mws, h), mws}
}

// ChainHandler is a http.Handler with support for handler composition and
// execution.
type ChainHandler struct {
	Endpoint    Handler
	chain       Handler
	Middlewares Middlewares
}

func (c *ChainHandler) Handle(api API, ctx Context) {
	c.chain.Handle(api, ctx)
}

// chain builds a Middleware composed of an inline middleware stack and endpoint
// handler in the order they are passed.
func chain(middlewares []func(Handler) Handler, endpoint Handler) Handler {
	// Return ahead of time if there aren't any middlewares for the chain
	if len(middlewares) == 0 {
		return endpoint
	}

	// Wrap the end handler with the middleware chain
	h := middlewares[len(middlewares)-1](endpoint)
	for i := len(middlewares) - 2; i >= 0; i-- {
		h = middlewares[i](h)
	}

	return h
}
