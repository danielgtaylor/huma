package huma

type Middleware func(next func(Context)) func(Context)

// Middlewares is a list of middleware functions that can be attached to an
// API and will be called for all incoming requests.
type Middlewares []Middleware

// Handler builds and returns a handler func from the chain of middlewares,
// with `endpoint func` as the final handler.
func (m Middlewares) Handler(endpoint func(Context)) func(Context) {
	return m.chain(endpoint)
}

// chain builds a Middleware composed of an inline middleware stack and endpoint
// handler in the order they are passed.
func (m Middlewares) chain(endpoint func(Context)) func(Context) {
	// Return ahead of time if there aren't any middlewares for the chain
	if len(m) == 0 {
		return endpoint
	}

	// Wrap the end handler with the middleware chain
	w := m[len(m)-1](endpoint)
	for i := len(m) - 2; i >= 0; i-- {
		w = m[i](w)
	}
	return w
}
