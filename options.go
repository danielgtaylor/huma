package huma

import (
	"net/http"

	"github.com/Jeffail/gabs"
	"github.com/gin-gonic/gin"
)

// RouterOption sets an option on the router or OpenAPI top-level structure.
type RouterOption interface {
	ApplyRouter(r *Router)
}

// ResourceOption sets an option on the resource to be used in sub-resources
// and operations.
type ResourceOption interface {
	ApplyResource(r *Resource)
}

// SharedOption sets an option on either a router/API or resource.
type SharedOption interface {
	RouterOption
	ResourceOption
}

type extraOption struct {
	extra map[string]interface{}
}

func (o *extraOption) ApplyRouter(r *Router) {
	for k, v := range o.extra {
		r.api.Extra[k] = v
	}
}

func (o *extraOption) ApplyResource(r *Resource) {
	// for k, v := range o.extra {
	// 	r.extra[k] = v
	// }
}

// Extra sets extra values in the generated OpenAPI 3 spec.
func Extra(pairs ...interface{}) SharedOption {
	extra := map[string]interface{}{}

	for i := 0; i < len(pairs); i += 2 {
		k := pairs[i].(string)
		v := pairs[i+1]
		extra[k] = v
	}

	return &extraOption{extra}
}

// routerOption is a shorthand struct used to create API options easily.
type routerOption struct {
	handler func(*Router)
}

func (o *routerOption) ApplyRouter(router *Router) {
	o.handler(router)
}

// ProdServer sets the production server URL on the API.
func ProdServer(url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Servers = append(r.api.Servers, &Server{url, "Production server"})
	}}
}

// DevServer sets the development server URL on the API.
func DevServer(url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Servers = append(r.api.Servers, &Server{url, "Development server"})
	}}
}

// ContactFull sets the API contact information.
func ContactFull(name, url, email string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &Contact{name, url, email}
	}}
}

// ContactURL sets the API contact name & URL information.
func ContactURL(name, url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &Contact{Name: name, URL: url}
	}}
}

// ContactEmail sets the API contact name & email information.
func ContactEmail(name, email string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &Contact{Name: name, Email: email}
	}}
}

// BasicAuth adds a named HTTP Basic Auth security scheme.
func BasicAuth(name string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.SecuritySchemes[name] = &SecurityScheme{
			Type:   "http",
			Scheme: "basic",
		}
	}}
}

// APIKeyAuth adds a named pre-shared API key security scheme. The location of
// the API key parameter is defined with `in` and can be one of `query`,
// `header`, or `cookie`.
func APIKeyAuth(name, keyName, in string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.SecuritySchemes[name] = &SecurityScheme{
			Type: "apiKey",
			Name: keyName,
			In:   in,
		}
	}}
}

// JWTBearerAuth adds a named JWT bearer auth scheme using the Authorization
// header.
func JWTBearerAuth(name string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.SecuritySchemes[name] = &SecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		}
	}}
}

// WithGin replaces the underlying Gin engine for this router.
func WithGin(engine *gin.Engine) RouterOption {
	return &routerOption{func(r *Router) {
		r.engine = engine
	}}
}

// GinMiddleware attaches middleware to the router.
func GinMiddleware(middleware ...gin.HandlerFunc) RouterOption {
	return &routerOption{func(r *Router) {
		r.engine.Use(middleware...)
	}}
}

// PreStart registers a function to run before server start.
func PreStart(f func()) RouterOption {
	return &routerOption{func(r *Router) {
		r.prestart = append(r.prestart, f)
	}}
}

// HTTPServer sets a custom `http.Server`. This can be used to set custom
// server-wide timeouts for example.
func HTTPServer(server *http.Server) RouterOption {
	return &routerOption{func(r *Router) {
		r.server = server
	}}
}

// DocsHandler sets the documentation rendering handler function. You can
// use `huma.RapiDocHandler`, `huma.ReDocHandler`, `huma.SwaggerUIHandler`, or
// provide your own (e.g. with custom auth or branding).
func DocsHandler(f func(*gin.Context, *OpenAPI)) RouterOption {
	return &routerOption{func(r *Router) {
		r.docsHandler = f
	}}
}

// OpenAPIHook registers a function to be called after the OpenAPI spec is
// generated but before being sent to the client.
func OpenAPIHook(f func(*gabs.Container)) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Hook = f
	}}
}
