package huma

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/danielgtaylor/huma/schema"
	"github.com/gin-gonic/gin"
)

// RouterOption sets an option on the router or OpenAPI top-level structure.
type RouterOption interface {
	ApplyRouter(r *Router)
}

// routerOption is a shorthand struct used to create API options easily.
type routerOption struct {
	handler func(*Router)
}

func (o *routerOption) ApplyRouter(router *Router) {
	o.handler(router)
}

// RouterOptions composes together a set of options into one.
func RouterOptions(options ...RouterOption) RouterOption {
	return &routerOption{func(r *Router) {
		for _, option := range options {
			option.ApplyRouter(r)
		}
	}}
}

// ResourceOption sets an option on the resource to be used in sub-resources
// and operations.
type ResourceOption interface {
	ApplyResource(r *Resource)
}

// resourceOption is a shorthand struct used to create resource options easily.
type resourceOption struct {
	handler func(*Resource)
}

func (o *resourceOption) ApplyResource(r *Resource) {
	o.handler(r)
}

// ResourceOptions composes together a set of options into one.
func ResourceOptions(options ...ResourceOption) ResourceOption {
	return &resourceOption{func(r *Resource) {
		for _, option := range options {
			option.ApplyResource(r)
		}
	}}
}

// OperationOption sets an option on an operation or resource object.
type OperationOption interface {
	ResourceOption
	applyOperation(o *openAPIOperation)
}

// operationOption is a shorthand struct used to create operation options
// easily. Options created with it can be applied to either operations or
// resources.
type operationOption struct {
	handler func(*openAPIOperation)
}

func (o *operationOption) ApplyResource(r *Resource) {
	o.handler(r.openAPIOperation)
}

func (o *operationOption) applyOperation(op *openAPIOperation) {
	o.handler(op)
}

// OperationOptions composes together a set of options into one.
func OperationOptions(options ...OperationOption) OperationOption {
	return &operationOption{func(o *openAPIOperation) {
		for _, option := range options {
			option.applyOperation(o)
		}
	}}
}

// DependencyOption sets an option on a dependency, operation, or resource
// object.
type DependencyOption interface {
	OperationOption
	applyDependency(d *openAPIDependency)
}

// dependencyOption is a shorthand struct used to create dependency options
// easily. Options created with it can be applied to dependencies, operations,
// and resources.
type dependencyOption struct {
	handler func(*openAPIDependency)
}

func (o *dependencyOption) ApplyResource(r *Resource) {
	o.handler(r.openAPIDependency)
}

func (o *dependencyOption) applyOperation(op *openAPIOperation) {
	o.handler(op.openAPIDependency)
}

func (o *dependencyOption) applyDependency(d *openAPIDependency) {
	o.handler(d)
}

// DependencyOptions composes together a set of options into one.
func DependencyOptions(options ...DependencyOption) DependencyOption {
	return &dependencyOption{func(d *openAPIDependency) {
		for _, option := range options {
			option.applyDependency(d)
		}
	}}
}

// ParamOption sets an option on an OpenAPI parameter.
type ParamOption interface {
	applyParam(*openAPIParam)
}

type paramOption struct {
	apply func(*openAPIParam)
}

func (o *paramOption) applyParam(p *openAPIParam) {
	o.apply(p)
}

// ResponseHeaderOption sets an option on an OpenAPI response header.
type ResponseHeaderOption interface {
	applyResponseHeader(*openAPIResponseHeader)
}

// ResponseOption sets an option on an OpenAPI response.
type ResponseOption interface {
	applyResponse(*openAPIResponse)
}

type responseOption struct {
	apply func(*openAPIResponse)
}

func (o *responseOption) applyResponse(r *openAPIResponse) {
	o.apply(r)
}

// sharedOption sets an option on any combination of objects.
type sharedOption struct {
	Set func(v interface{})
}

func (o *sharedOption) ApplyRouter(r *Router) {
	o.Set(r)
}

func (o *sharedOption) ApplyResource(r *Resource) {
	o.Set(r)
}

func (o *sharedOption) applyOperation(op *openAPIOperation) {
	o.Set(op)
}

func (o *sharedOption) applyParam(p *openAPIParam) {
	o.Set(p)
}

func (o *sharedOption) applyResponseHeader(r *openAPIResponseHeader) {
	o.Set(r)
}

func (o *sharedOption) applyResponse(r *openAPIResponse) {
	o.Set(r)
}

// Schema manually sets a JSON Schema on the object. If the top-level `type` is
// blank then the type will be guessed from the handler function. If no schema
// is set then one will be generated for you.
func Schema(s schema.Schema) interface {
	ParamOption
	ResponseHeaderOption
	ResponseOption
} {
	// Note: schema is pass by value rather than a pointer to prevent
	// issues with modification after being passed.
	return &sharedOption{func(v interface{}) {
		switch cast := v.(type) {
		case *openAPIParam:
			cast.Schema = &s
		case *openAPIResponseHeader:
			cast.Schema = &s
		case *openAPIResponse:
			cast.Schema = &s
		}
	}}
}

// SecurityRef adds a security reference by name with optional scopes.
func SecurityRef(name string, scopes ...string) interface {
	RouterOption
	OperationOption
} {
	if scopes == nil {
		scopes = []string{}
	}

	return &sharedOption{
		Set: func(v interface{}) {
			req := openAPISecurityRequirement{name: scopes}

			switch cast := v.(type) {
			case *Router:
				cast.api.Security = append(cast.api.Security, req)
			case *Resource:
				cast.security = append(cast.security, req)
			case *openAPIOperation:
				cast.security = append(cast.security, req)
			}
		},
	}
}

// Extra sets extra values in the generated OpenAPI 3 spec.
func Extra(pairs ...interface{}) interface {
	RouterOption
	OperationOption
} {
	extra := map[string]interface{}{}

	if len(pairs)%2 > 0 {
		panic(fmt.Errorf("requires key-value pairs but got: %v", pairs))
	}

	for i := 0; i < len(pairs); i += 2 {
		k := pairs[i].(string)
		v := pairs[i+1]
		extra[k] = v
	}

	return &sharedOption{
		Set: func(v interface{}) {
			var x map[string]interface{}

			switch cast := v.(type) {
			case *Router:
				x = cast.api.Extra
			case *Resource:
				x = cast.extra
			case *openAPIOperation:
				x = cast.extra
			}

			for k, v := range extra {
				x[k] = v
			}
		},
	}
}

// ProdServer sets the production server URL on the API.
func ProdServer(url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Servers = append(r.api.Servers, &openAPIServer{url, "Production server"})
	}}
}

// DevServer sets the development server URL on the API.
func DevServer(url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Servers = append(r.api.Servers, &openAPIServer{url, "Development server"})
	}}
}

// ContactFull sets the API contact information.
func ContactFull(name, url, email string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &openAPIContact{name, url, email}
	}}
}

// ContactURL sets the API contact name & URL information.
func ContactURL(name, url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &openAPIContact{Name: name, URL: url}
	}}
}

// ContactEmail sets the API contact name & email information.
func ContactEmail(name, email string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &openAPIContact{Name: name, Email: email}
	}}
}

// BasicAuth adds a named HTTP Basic Auth security scheme.
func BasicAuth(name string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.SecuritySchemes[name] = &openAPISecurityScheme{
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
		r.api.SecuritySchemes[name] = &openAPISecurityScheme{
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
		r.api.SecuritySchemes[name] = &openAPISecurityScheme{
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
		}
	}}
}

// Gin replaces the underlying Gin engine for this router.
func Gin(engine *gin.Engine) RouterOption {
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

// PreStart registers a function to run before server start. Multiple can be
// passed and they will run in the order they were added.
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
func DocsHandler(f Handler) RouterOption {
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

// SimpleDependency adds a new dependency with just a value or function.
func SimpleDependency(handler interface{}) DependencyOption {
	dep := &openAPIDependency{
		handler: handler,
	}

	return &dependencyOption{func(d *openAPIDependency) {
		d.dependencies = append(d.dependencies, dep)
	}}
}

// Dependency adds a dependency.
func Dependency(option DependencyOption, handler interface{}) DependencyOption {
	dep := newDependency(option, handler)
	return &dependencyOption{func(d *openAPIDependency) {
		d.dependencies = append(d.dependencies, dep)
	}}
}

// Example sets an example value, used for documentation and mocks.
func Example(value interface{}) ParamOption {
	return &paramOption{func(p *openAPIParam) {
		p.Example = value
	}}
}

// Internal marks this parameter as internal-only, meaning it will not be
// included in the OpenAPI 3 JSON. Useful for things like auth headers set
// by a load balancer / gateway that never get seen by end-users.
func Internal() ParamOption {
	return &paramOption{func(p *openAPIParam) {
		p.Internal = true
	}}
}

// Deprecated marks this parameter as deprecated.
func Deprecated() ParamOption {
	return &paramOption{func(p *openAPIParam) {
		p.Deprecated = true
	}}
}

func newParamOption(name, description string, required bool, def interface{}, in paramLocation, options ...ParamOption) DependencyOption {
	p := newOpenAPIParam(name, description, in, options...)
	p.Required = required
	p.def = def

	return &dependencyOption{func(d *openAPIDependency) {
		d.params = append(d.params, p)
	}}
}

// PathParam adds a new required path parameter
func PathParam(name string, description string, options ...ParamOption) DependencyOption {
	return newParamOption(name, description, true, nil, inPath, options...)
}

// QueryParam returns a new optional query string parameter
func QueryParam(name string, description string, defaultValue interface{}, options ...ParamOption) DependencyOption {
	return newParamOption(name, description, false, defaultValue, inQuery, options...)
}

// HeaderParam returns a new optional header parameter
func HeaderParam(name string, description string, defaultValue interface{}, options ...ParamOption) DependencyOption {
	return newParamOption(name, description, false, defaultValue, inHeader, options...)
}

// ResponseHeader returns a new response header
func ResponseHeader(name, description string) DependencyOption {
	r := &openAPIResponseHeader{
		Name:        name,
		Description: description,
	}

	return &dependencyOption{func(d *openAPIDependency) {
		d.responseHeaders = append(d.responseHeaders, r)
	}}
}

// OperationID manually sets the operation's unique ID. If not set, it will
// be auto-generated from the resource path and operation verb.
func OperationID(id string) OperationOption {
	return &operationOption{func(o *openAPIOperation) {
		o.id = id
	}}
}

// Tags sets one or more text tags on the operation.
func Tags(values ...string) OperationOption {
	return &operationOption{func(o *openAPIOperation) {
		o.tags = append(o.tags, values...)
	}}
}

// RequestContentType sets the request content type on the operation.
func RequestContentType(name string) OperationOption {
	return &operationOption{func(o *openAPIOperation) {
		o.requestContentType = name
	}}
}

// RequestSchema sets the request body schema on the operation.
func RequestSchema(schema *schema.Schema) OperationOption {
	return &operationOption{func(o *openAPIOperation) {
		o.requestSchema = schema
	}}
}

// ContentType sets the content type for this response. If blank, an empty
// response is returned.
func ContentType(value string) ResponseOption {
	return &responseOption{func(r *openAPIResponse) {
		r.ContentType = value
	}}
}

// Headers sets a list of allowed response headers.
func Headers(values ...string) ResponseOption {
	return &responseOption{func(r *openAPIResponse) {
		r.Headers = values
	}}
}

// Response adds a new response to the operation.
func Response(statusCode int, description string, options ...ResponseOption) OperationOption {
	r := newOpenAPIResponse(statusCode, description, options...)

	return &operationOption{func(o *openAPIOperation) {
		o.responses = append(o.responses, r)
	}}
}

// ResponseText adds a new string response to the operation. Alias for
func ResponseText(statusCode int, description string, options ...ResponseOption) OperationOption {
	options = append(options, ContentType("text/plain"))
	return Response(statusCode, description, options...)
}

// ResponseJSON adds a new JSON response model to the operation.
func ResponseJSON(statusCode int, description string, options ...ResponseOption) OperationOption {
	options = append(options, ContentType("application/json"))
	return Response(statusCode, description, options...)
}

// ResponseError adds a new error response model. This uses the RFC7807
// application/problem+json response content type.
func ResponseError(statusCode int, description string, options ...ResponseOption) OperationOption {
	options = append(options, ContentType("application/problem+json"))
	return Response(statusCode, description, options...)
}

// MaxBodyBytes sets the max number of bytes read from a request body before
// the handler aborts and returns an error. Applies to all sub-resources.
func MaxBodyBytes(value int64) OperationOption {
	return &operationOption{func(o *openAPIOperation) {
		o.maxBodyBytes = value
	}}
}

// BodyReadTimeout sets the duration after which the read is aborted and an
// error is returned.
func BodyReadTimeout(value time.Duration) OperationOption {
	return &operationOption{func(o *openAPIOperation) {
		o.bodyReadTimeout = value
	}}
}
