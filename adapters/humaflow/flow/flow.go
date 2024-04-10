// Package flow is a delightfully simple, readable, and tiny HTTP router for Go web applications. Its features include:
//
// * Use named parameters, wildcards and (optionally) regexp patterns in your routes.
// * Create route groups which use different middleware (a bit like chi).
// * Customizable handlers for 404 Not Found and 405 Method Not Allowed responses.
// * Automatic handling of OPTIONS and HEAD requests.
// * Works with http.Handler, http.HandlerFunc, and standard Go middleware.
//
// Example code:
//
//	package main
//
//	import (
//		"fmt"
//		"log"
//		"net/http"
//
//		"github.com/alexedwards/flow"
//	)
//
//	func main() {
//		mux := flow.New()
//
//		// The Use() method can be used to register middleware. Middleware declared at
//		// the top level will used on all routes (including error handlers and OPTIONS
//		// responses).
//		mux.Use(exampleMiddleware1)
//
//		// Routes can use multiple HTTP methods.
//		mux.HandleFunc("/profile/:name", exampleHandlerFunc1, "GET", "POST")
//
//		// Optionally, regular expressions can be used to enforce a specific pattern
//		// for a named parameter.
//		mux.HandleFunc("/profile/:name/:age|^[0-9]{1,3}$", exampleHandlerFunc2, "GET")
//
//		// The wildcard ... can be used to match the remainder of a request path.
//		// Notice that HTTP methods are also optional (if not provided, all HTTP
//		// methods will match the route).
//		mux.Handle("/static/...", exampleHandler)
//
//		// You can create route 'groups'.
//		mux.Group(func(mux *flow.Mux) {
//			// Middleware declared within in the group will only be used on the routes
//			// in the group.
//			mux.Use(exampleMiddleware2)
//
//			mux.HandleFunc("/admin", exampleHandlerFunc3, "GET")
//
//			// Groups can be nested.
//			mux.Group(func(mux *flow.Mux) {
//				mux.Use(exampleMiddleware3)
//
//				mux.HandleFunc("/admin/passwords", exampleHandlerFunc4, "GET")
//			})
//		})
//
//		err := http.ListenAndServe(":2323", mux)
//		log.Fatal(err)
//	}
package flow

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// AllMethods is a slice containing all HTTP request methods.
var AllMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace}

var compiledRXPatterns = map[string]*regexp.Regexp{}

type contextKey string

// Param is used to retrieve the value of a named parameter or wildcard from the
// request context. It returns the empty string if no matching parameter is
// found.
func Param(ctx context.Context, param string) string {
	s, ok := ctx.Value(contextKey(param)).(string)
	if !ok {
		return ""
	}

	return s
}

// Mux is a http.Handler which dispatches requests to different handlers.
type Mux struct {
	NotFound         http.Handler
	MethodNotAllowed http.Handler
	Options          http.Handler
	routes           *[]route
	middlewares      []func(http.Handler) http.Handler
}

// New returns a new initialized Mux instance.
func New() *Mux {
	return &Mux{
		NotFound: http.NotFoundHandler(),
		MethodNotAllowed: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}),
		Options: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		routes: &[]route{},
	}
}

// Handle registers a new handler for the given request path pattern and HTTP
// methods.
func (m *Mux) Handle(pattern string, handler http.Handler, methods ...string) {
	if slices.Contains(methods, http.MethodGet) && !slices.Contains(methods, http.MethodHead) {
		methods = append(methods, http.MethodHead)
	}

	if len(methods) == 0 {
		methods = AllMethods
	}

	for _, method := range methods {
		route := route{
			method:   strings.ToUpper(method),
			segments: strings.Split(pattern, "/"),
			wildcard: strings.HasSuffix(pattern, "/..."),
			handler:  m.wrap(handler),
		}

		*m.routes = append(*m.routes, route)
	}

	// Compile any regular expression patterns and add them to the
	// compiledRXPatterns map.
	for _, segment := range strings.Split(pattern, "/") {
		if strings.HasPrefix(segment, ":") {
			_, rxPattern, containsRx := strings.Cut(segment, "|")
			if containsRx {
				compiledRXPatterns[rxPattern] = regexp.MustCompile(rxPattern)
			}
		}
	}
}

// HandleFunc is an adapter which allows using a http.HandlerFunc as a handler.
func (m *Mux) HandleFunc(pattern string, fn http.HandlerFunc, methods ...string) {
	m.Handle(pattern, fn, methods...)
}

// Use registers middleware with the Mux instance. Middleware must have the
// signature `func(http.Handler) http.Handler`.
func (m *Mux) Use(mw ...func(http.Handler) http.Handler) {
	m.middlewares = append(m.middlewares, mw...)
}

// Group is used to create 'groups' of routes in a Mux. Middleware registered
// inside the group will only be used on the routes in that group. See the
// example code at the start of the package documentation for how to use this
// feature.
func (m *Mux) Group(fn func(*Mux)) {
	mm := *m
	fn(&mm)
}

// ServeHTTP makes the router implement the http.Handler interface.
func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	urlSegments := strings.Split(r.URL.Path, "/")
	allowedMethods := []string{}

	for _, route := range *m.routes {
		ctx, ok := route.match(r.Context(), urlSegments)
		if ok {
			if r.Method == route.method {
				route.handler.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			if !slices.Contains(allowedMethods, route.method) {
				allowedMethods = append(allowedMethods, route.method)
			}
		}
	}

	if len(allowedMethods) > 0 {
		w.Header().Set("Allow", strings.Join(append(allowedMethods, http.MethodOptions), ", "))
		if r.Method == http.MethodOptions {
			m.wrap(m.Options).ServeHTTP(w, r)
		} else {
			m.wrap(m.MethodNotAllowed).ServeHTTP(w, r)
		}
		return
	}

	m.wrap(m.NotFound).ServeHTTP(w, r)
}

func (m *Mux) wrap(handler http.Handler) http.Handler {
	for i := len(m.middlewares) - 1; i >= 0; i-- {
		handler = m.middlewares[i](handler)
	}

	return handler
}

type route struct {
	method   string
	segments []string
	wildcard bool
	handler  http.Handler
}

func (r *route) match(ctx context.Context, urlSegments []string) (context.Context, bool) {
	if !r.wildcard && len(urlSegments) != len(r.segments) {
		return ctx, false
	}

	for i, routeSegment := range r.segments {
		if i > len(urlSegments)-1 {
			return ctx, false
		}

		if routeSegment == "..." {
			ctx = context.WithValue(ctx, contextKey("..."), strings.Join(urlSegments[i:], "/"))
			return ctx, true
		}

		if strings.HasPrefix(routeSegment, ":") {
			key, rxPattern, containsRx := strings.Cut(strings.TrimPrefix(routeSegment, ":"), "|")

			if containsRx {
				if compiledRXPatterns[rxPattern].MatchString(urlSegments[i]) {
					ctx = context.WithValue(ctx, contextKey(key), urlSegments[i])
					continue
				}
			}

			if !containsRx && urlSegments[i] != "" {
				ctx = context.WithValue(ctx, contextKey(key), urlSegments[i])
				continue
			}

			return ctx, false
		}

		if urlSegments[i] != routeSegment {
			return ctx, false
		}
	}

	return ctx, true
}
