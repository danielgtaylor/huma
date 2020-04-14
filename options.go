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

// OperationOption sets an option on an operation or resource object.
type OperationOption interface {
	ResourceOption
	ApplyOperation(o *OpenAPIOperation)
}

// operationOption is a shorthand struct used to create operation options
// easily. Options created with it can be applied to either operations or
// resources.
type operationOption struct {
	handler func(*OpenAPIOperation)
}

func (o *operationOption) ApplyResource(r *Resource) {
	o.handler(r.OpenAPIOperation)
}

func (o *operationOption) ApplyOperation(op *OpenAPIOperation) {
	o.handler(op)
}

// DependencyOption sets an option on a dependency, operation, or resource
// object.
type DependencyOption interface {
	OperationOption
	ApplyDependency(d *OpenAPIDependency)
}

// dependencyOption is a shorthand struct used to create dependency options
// easily. Options created with it can be applied to dependencies, operations,
// and resources.
type dependencyOption struct {
	handler func(*OpenAPIDependency)
}

func (o *dependencyOption) ApplyResource(r *Resource) {
	o.handler(r.OpenAPIDependency)
}

func (o *dependencyOption) ApplyOperation(op *OpenAPIOperation) {
	o.handler(op.OpenAPIDependency)
}

func (o *dependencyOption) ApplyDependency(d *OpenAPIDependency) {
	o.handler(d)
}

// DependencyOptions composes together a set of options into one.
func DependencyOptions(options ...DependencyOption) DependencyOption {
	return &dependencyOption{func(d *OpenAPIDependency) {
		for _, option := range options {
			option.ApplyDependency(d)
		}
	}}
}

// ParamOption sets an option on an OpenAPI parameter.
type ParamOption interface {
	ApplyParam(*OpenAPIParam)
}

type paramOption struct {
	apply func(*OpenAPIParam)
}

func (o *paramOption) ApplyParam(p *OpenAPIParam) {
	o.apply(p)
}

// ResponseHeaderOption sets an option on an OpenAPI response header.
type ResponseHeaderOption interface {
	ApplyResponseHeader(*OpenAPIResponseHeader)
}

// ResponseOption sets an option on an OpenAPI response.
type ResponseOption interface {
	ApplyResponse(*OpenAPIResponse)
}

type responseOption struct {
	apply func(*OpenAPIResponse)
}

func (o *responseOption) ApplyResponse(r *OpenAPIResponse) {
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

func (o *sharedOption) ApplyOperation(op *OpenAPIOperation) {
	o.Set(op)
}

func (o *sharedOption) ApplyParam(p *OpenAPIParam) {
	o.Set(p)
}

func (o *sharedOption) ApplyResponseHeader(r *OpenAPIResponseHeader) {
	o.Set(r)
}

func (o *sharedOption) ApplyResponse(r *OpenAPIResponse) {
	o.Set(r)
}

// Schema manually sets a JSON Schema on the object. If the top-level `type` is
// blank then the type will be guessed from the handler function.
func Schema(s *schema.Schema) interface {
	ParamOption
	ResponseHeaderOption
	ResponseOption
} {
	return &sharedOption{func(v interface{}) {
		switch cast := v.(type) {
		case *OpenAPIParam:
			cast.Schema = s
		case *OpenAPIResponseHeader:
			cast.Schema = s
		case *OpenAPIResponse:
			cast.Schema = s
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
			req := OpenAPISecurityRequirement{name: scopes}

			switch cast := v.(type) {
			case *Router:
				cast.api.Security = append(cast.api.Security, req)
			case *Resource:
				cast.security = append(cast.security, req)
			case *OpenAPIOperation:
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
			case *OpenAPIOperation:
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
		r.api.Servers = append(r.api.Servers, &OpenAPIServer{url, "Production server"})
	}}
}

// DevServer sets the development server URL on the API.
func DevServer(url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Servers = append(r.api.Servers, &OpenAPIServer{url, "Development server"})
	}}
}

// ContactFull sets the API contact information.
func ContactFull(name, url, email string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &OpenAPIContact{name, url, email}
	}}
}

// ContactURL sets the API contact name & URL information.
func ContactURL(name, url string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &OpenAPIContact{Name: name, URL: url}
	}}
}

// ContactEmail sets the API contact name & email information.
func ContactEmail(name, email string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.Contact = &OpenAPIContact{Name: name, Email: email}
	}}
}

// BasicAuth adds a named HTTP Basic Auth security scheme.
func BasicAuth(name string) RouterOption {
	return &routerOption{func(r *Router) {
		r.api.SecuritySchemes[name] = &OpenAPISecurityScheme{
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
		r.api.SecuritySchemes[name] = &OpenAPISecurityScheme{
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
		r.api.SecuritySchemes[name] = &OpenAPISecurityScheme{
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

// SimpleDependency adds a new dependency with just a value or function.
func SimpleDependency(handler interface{}) DependencyOption {
	dep := &OpenAPIDependency{
		handler: handler,
	}

	return &dependencyOption{func(d *OpenAPIDependency) {
		d.dependencies = append(d.dependencies, dep)
	}}
}

// Dependency adds a dependency.
func Dependency(option DependencyOption, handler interface{}) DependencyOption {
	dep := NewDependency(option, handler)
	return &dependencyOption{func(d *OpenAPIDependency) {
		d.dependencies = append(d.dependencies, dep)
	}}
}

// Example sets an example value, used for documentation and mocks.
func Example(value interface{}) ParamOption {
	return &paramOption{func(p *OpenAPIParam) {
		p.Example = value
	}}
}

// Internal marks this parameter as internal-only, meaning it will not be
// included in the OpenAPI 3 JSON. Useful for things like auth headers set
// by a load balancer / gateway.
func Internal() ParamOption {
	return &paramOption{func(p *OpenAPIParam) {
		p.Internal = true
	}}
}

// Deprecated marks this parameter as deprecated.
func Deprecated() ParamOption {
	return &paramOption{func(p *OpenAPIParam) {
		p.Deprecated = true
	}}
}

func newParamOption(name, description string, required bool, def interface{}, in ParamLocation, options ...ParamOption) DependencyOption {
	p := NewOpenAPIParam(name, description, in, options...)
	p.Required = required
	p.def = def

	return &dependencyOption{func(d *OpenAPIDependency) {
		d.params = append(d.params, p)
	}}
}

// PathParam adds a new required path parameter
func PathParam(name string, description string, options ...ParamOption) DependencyOption {
	return newParamOption(name, description, true, nil, InPath, options...)
}

// QueryParam returns a new optional query string parameter
func QueryParam(name string, description string, defaultValue interface{}, options ...ParamOption) DependencyOption {
	return newParamOption(name, description, false, defaultValue, InQuery, options...)
}

// HeaderParam returns a new optional header parameter
func HeaderParam(name string, description string, defaultValue interface{}, options ...ParamOption) DependencyOption {
	return newParamOption(name, description, false, defaultValue, InHeader, options...)
}

// ResponseHeader returns a new response header
func ResponseHeader(name, description string) DependencyOption {
	r := &OpenAPIResponseHeader{
		Name:        name,
		Description: description,
	}

	return &dependencyOption{func(d *OpenAPIDependency) {
		d.responseHeaders = append(d.responseHeaders, r)
	}}
}

// OperationID manually sets the operation's unique ID. If not set, it will
// be auto-generated from the resource path and operation verb.
func OperationID(id string) OperationOption {
	return &operationOption{func(o *OpenAPIOperation) {
		o.id = id
	}}
}

// Tags sets one or more text tags on the operation.
func Tags(values ...string) OperationOption {
	return &operationOption{func(o *OpenAPIOperation) {
		o.tags = append(o.tags, values...)
	}}
}

// RequestContentType sets the request content type on the operation.
func RequestContentType(name string) OperationOption {
	return &operationOption{func(o *OpenAPIOperation) {
		o.requestContentType = name
	}}
}

// RequestSchema sets the request body schema on the operation.
func RequestSchema(schema *schema.Schema) OperationOption {
	return &operationOption{func(o *OpenAPIOperation) {
		o.requestSchema = schema
	}}
}

// ContentType sets the content type for this response. If blank, an empty
// response is returned.
func ContentType(value string) ResponseOption {
	return &responseOption{func(r *OpenAPIResponse) {
		r.ContentType = value
	}}
}

// Headers sets a list of allowed response headers.
func Headers(values ...string) ResponseOption {
	return &responseOption{func(r *OpenAPIResponse) {
		r.Headers = values
	}}
}

// Response adds a new response to the operation.
func Response(statusCode int, description string, options ...ResponseOption) OperationOption {
	r := NewOpenAPIResponse(statusCode, description, options...)

	return &operationOption{func(o *OpenAPIOperation) {
		o.responses = append(o.responses, r)
	}}
}

// ResponseText adds a new string response to the operation.
func ResponseText(statusCode int, description string, options ...ResponseOption) OperationOption {
	options = append(options, ContentType("text/plain"))
	return Response(statusCode, description, options...)
}

// ResponseJSON adds a new JSON response model to the operation.
func ResponseJSON(statusCode int, description string, options ...ResponseOption) OperationOption {
	options = append(options, ContentType("application/json"))
	return Response(statusCode, description, options...)
}

// ResponseError adds a new error response model. Alias for ResponseJSON
// mainly useful for documentation purposes.
func ResponseError(statusCode int, description string, options ...ResponseOption) OperationOption {
	return ResponseJSON(statusCode, description, options...)
}

// MaxBodyBytes sets the max number of bytes read from a request body before
// the handler aborts and returns an error. Applies to all sub-resources.
func MaxBodyBytes(value int64) OperationOption {
	return &operationOption{func(o *OpenAPIOperation) {
		o.maxBodyBytes = value
	}}
}

// BodyReadTimeout sets the duration after which the read is aborted and an
// error is returned.
func BodyReadTimeout(value time.Duration) OperationOption {
	return &operationOption{func(o *OpenAPIOperation) {
		o.bodyReadTimeout = value
	}}
}
