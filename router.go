package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Jeffail/gabs/v2"
	"github.com/danielgtaylor/huma/schema"
	"github.com/go-chi/chi"
)

type contextKey string

// connContextKey is used to get/set the underlying `net.Conn` from a request
// context value.
var connContextKey contextKey = "huma-request-conn"

// opIDContextKey is used to get the operation name after request routing
// has finished.
var opIDContextKey contextKey = "huma-operation-id"

// GetConn gets the underlying `net.Conn` from a context.
func GetConn(ctx context.Context) net.Conn {
	conn := ctx.Value(connContextKey)
	if conn != nil {
		return conn.(net.Conn)
	}
	return nil
}

// Router is the entrypoint to your API.
type Router struct {
	mux       *chi.Mux
	resources []*Resource

	title           string
	version         string
	description     string
	contact         oaContact
	servers         []oaServer
	securitySchemes map[string]oaSecurityScheme
	security        []map[string][]string

	autoConfig *AutoConfig

	// Documentation, OpenAPI spec, and schemas routing handlers
	docsPrefix    string
	docsSuffix    string
	schemasSuffix string
	specSuffix    string
	docsHandler   http.Handler
	docsAreSetup  bool

	// Tracks the currently running server for graceful shutdown.
	server     *http.Server
	serverLock sync.Mutex

	// Allows modification of the generated OpenAPI.
	openapiHook func(*gabs.Container)

	// Router-global defaults
	defaultBodyReadTimeout   time.Duration
	defaultServerIdleTimeout time.Duration

	// Information for creating non-relative links & schema refs.
	urlPrefix             string
	disableSchemaProperty bool
}

// OpenAPI returns an OpenAPI 3 representation of the API, which can be
// modified as needed and rendered to JSON via `.String()`.
func (r *Router) OpenAPI() *gabs.Container {
	doc := gabs.New()

	doc.Set("3.0.3", "openapi")
	doc.Set(r.title, "info", "title")
	doc.Set(r.version, "info", "version")

	if r.contact.Name != "" || r.contact.Email != "" || r.contact.URL != "" {
		doc.Set(r.contact, "info", "contact")
	}

	if r.description != "" {
		doc.Set(r.description, "info", "description")
	}

	if len(r.servers) > 0 {
		doc.Set(r.servers, "servers")
	}

	components := &oaComponents{
		Schemas:         map[string]*schema.Schema{},
		SecuritySchemes: r.securitySchemes,
	}

	paths, _ := doc.Object("paths")
	for _, res := range r.resources {
		paths.Merge(res.toOpenAPI(components))
	}

	doc.Set(components, "components")

	if len(r.security) > 0 {
		doc.Set(r.security, "security")
	}

	if r.autoConfig != nil {
		doc.Set(r.autoConfig, "x-cli-config")
	}

	if r.openapiHook != nil {
		r.openapiHook(doc)
	}

	return doc
}

// Contact sets the API's contact information.
func (r *Router) Contact(name, email, url string) {
	r.contact.Name = name
	r.contact.Email = email
	r.contact.URL = url
}

// ServerLink adds a new server link to this router for documentation.
func (r *Router) ServerLink(description, uri string) {
	r.servers = append(r.servers, oaServer{
		Description: description,
		URL:         uri,
	})
}

// GatewayBasicAuth documents that the API gateway handles auth using HTTP Basic.
func (r *Router) GatewayBasicAuth(name string) {
	r.securitySchemes[name] = oaSecurityScheme{
		Type:   SecuritySchemeHTTP,
		Scheme: "basic",
	}
}

// GatewayBearerFormat documents that the API gateway handles auth using HTTP Bearer.
func (r *Router) GatewayBearerFormat(name string, description string, format string) {
	r.securitySchemes[name] = oaSecurityScheme{
		Type:         SecuritySchemeHTTP,
		Description:  description,
		Scheme:       "bearer",
		BearerFormat: format,
	}
}

// GatewayAPIKey documents that the API gateway handles auth using API Key.
func (r *Router) GatewayAPIKey(name string, description string, keyName string, in APIKeyLocation) {
	r.securitySchemes[name] = oaSecurityScheme{
		Type:        SecuritySchemeApiKey,
		Description: description,
		Name:        keyName,
		In:          in,
	}
}

// GatewayOpenIDConnect documents that the API gateway handles auth using openIdConnect.
func (r *Router) GatewayOpenIDConnect(name string, description string, url string) {
	r.securitySchemes[name] = oaSecurityScheme{
		Type:             SecuritySchemeOpenIdConnect,
		Description:      description,
		OpenIdConnectUrl: url,
	}
}

// GatewayClientCredentials documents that the API gateway handles auth using
// OAuth2 client credentials (pre-shared secret).
func (r *Router) GatewayClientCredentials(name, tokenURL string, scopes map[string]string) {
	r.securitySchemes[name] = oaSecurityScheme{
		Type: "oauth2",
		Flows: &oaFlows{
			ClientCredentials: &oaFlow{
				TokenURL: tokenURL,
				Scopes:   scopes,
			},
		},
	}
}

// GatewayAuthCode documents that the API gateway handles auth using
// OAuth2 authorization code (user login).
func (r *Router) GatewayAuthCode(name, authorizeURL, tokenURL string, scopes map[string]string) {
	r.securitySchemes[name] = oaSecurityScheme{
		Type: "oauth2",
		Flows: &oaFlows{
			AuthorizationCode: &oaFlow{
				AuthorizationURL: authorizeURL,
				TokenURL:         tokenURL,
				Scopes:           scopes,
			},
		},
	}
}

// AutoConfig sets up CLI autoconfiguration via `x-cli-config` for use by CLI
// clients, e.g. using a tool like Restish (https://rest.sh/).
func (r *Router) AutoConfig(autoConfig AutoConfig) {
	r.autoConfig = &autoConfig
}

// SecurityRequirement sets up a security requirement for the entire API by
// name and with the given scopes. Use together with the other auth options
// like GatewayAuthCode. Calling multiple times results in requiring one OR
// the other schemes but not both.
func (r *Router) SecurityRequirement(name string, scopes ...string) {
	if scopes == nil {
		scopes = []string{}
	}

	r.security = append(r.security, map[string][]string{
		name: scopes,
	})
}

// Resource creates a new resource attached to this router at the given path.
// The path can include parameters, e.g. `/things/{thing-id}`. Each resource
// path must be unique.
func (r *Router) Resource(path string) *Resource {
	res := &Resource{
		path:         path,
		mux:          r.mux.Route(path, nil),
		subResources: []*Resource{},
		operations:   []*Operation{},
		tags:         []string{},
		router:       r,
	}

	r.resources = append(r.resources, res)

	return res
}

// Middleware adds a new standard middleware to this router at the root,
// so it will apply to all requests. Middleware can also be applied at the
// resource level.
func (r *Router) Middleware(middlewares ...func(next http.Handler) http.Handler) {
	r.mux.Use(middlewares...)
}

// DocsPath returns the server path to the OpenAPI docs.
func (r *Router) DocsPath() string {
	return fmt.Sprintf("%s/%s", r.docsPrefix, r.docsSuffix)
}

// SchemasPath returns the server path to the OpenAPI Schemas.
func (r *Router) SchemasPath() string {
	return fmt.Sprintf("%s/%s", r.docsPrefix, r.schemasSuffix)
}

// OpenAPIPath returns the server path to the OpenAPI JSON.
func (r *Router) OpenAPIPath() string {
	return fmt.Sprintf("%s/%s.json", r.docsPrefix, r.specSuffix)
}

// DocsPrefix sets the path prefix for where the OpenAPI JSON, schemas,
// and documentation are hosted.
func (r *Router) DocsPrefix(path string) {
	r.docsPrefix = path
}

// DocsSuffix sets the final path suffix for where the OpenAPI documentation
// is hosted. When not specified, the default value of `docs` is appended to the
// DocsPrefix.
func (r *Router) DocsSuffix(suffix string) {
	r.docsSuffix = suffix
}

// SchemasSuffix sets the final path suffix for where the OpenAPI schemas
// are hosted. When not specified, the default value of `schemas` is appended
// to the DocsPrefix.
func (r *Router) SchemasSuffix(suffix string) {
	r.schemasSuffix = suffix
}

// SpecSuffix sets the final path suffix for where the OpenAPI spec is hosted.
// When not specified, the default value of `openapi` is appended to the
// DocsPrefix.
func (r *Router) SpecSuffix(suffix string) {
	r.specSuffix = suffix
}

// DocsHandler sets the http.Handler to render documentation. It defaults to
// using RapiDoc.
func (r *Router) DocsHandler(handler http.Handler) {
	r.docsHandler = handler
}

// OpenAPIHook provides a function to run after generating the OpenAPI document
// allowing you to modify it as needed.
func (r *Router) OpenAPIHook(hook func(*gabs.Container)) {
	r.openapiHook = hook
}

// replaceRef recursively replaces refs in a JSON Schema to point to a new
// location.
func replaceRef(schema map[string]interface{}, from, to string) {
	if schema["$ref"] != nil {
		schema["$ref"] = strings.Replace(schema["$ref"].(string), from, to, -1) + ".json"
	}

	for _, v := range schema {
		if m, ok := v.(map[string]interface{}); ok {
			replaceRef(m, from, to)
		} else if s, ok := v.([]interface{}); ok {
			for _, item := range s {
				if m, ok := item.(map[string]interface{}); ok {
					replaceRef(m, from, to)
				}
			}
		}
	}
}

// Set up the docs & OpenAPI routes.
func (r *Router) setupDocs() {
	// Precompute the OpenAPI document once on startup and then serve the cached
	// version of it.
	spec := r.OpenAPI()

	var schemas map[string]interface{}
	b, _ := json.Marshal(spec.Search("components").Data().(*oaComponents).Schemas)
	json.Unmarshal(b, &schemas)
	for _, v := range schemas {
		// Convert $ref links for standalone JSON files.
		// #/components/schemas/MyType -> ./MyType.json
		replaceRef(v.(map[string]interface{}), "#/components/schemas", ".")
	}

	// Register the docs handlers if needed.
	if !r.mux.Match(chi.NewRouteContext(), http.MethodGet, r.OpenAPIPath()) {
		r.mux.Get(r.OpenAPIPath(), func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/vnd.oai.openapi+json")
			w.Write(spec.Bytes())
		})
	}

	if !r.mux.Match(chi.NewRouteContext(), http.MethodGet, r.SchemasPath()+"/{schema-id}.json") {
		r.mux.Get(r.SchemasPath()+"/{schema-id}.json", func(w http.ResponseWriter, req *http.Request) {
			id := chi.URLParam(req, "schema-id")
			schema := schemas[id]
			if schema == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			b, _ := json.Marshal(schema)
			w.Header().Set("Content-Type", "application/schema+json")
			w.Write(b)
		})
	}

	if !r.mux.Match(chi.NewRouteContext(), http.MethodGet, r.DocsPath()) {
		r.mux.Get(r.DocsPath(), r.docsHandler.ServeHTTP)
	}

	r.docsAreSetup = true
}

func (r *Router) listen(addr, certFile, keyFile string) error {
	// Setup docs on startup so we can fail fast if the handler is broken in
	// some way.
	r.setupDocs()

	// Start the server.
	r.serverLock.Lock()
	if r.server == nil {
		r.server = &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       r.defaultServerIdleTimeout,
			Handler:           r,
			ConnContext: func(ctx context.Context, c net.Conn) context.Context {
				return context.WithValue(ctx, connContextKey, c)
			},
		}
	} else {
		r.server.Addr = addr

		// Wrap the ConnContext method to inject the current connection into the
		// request context. This is useful to e.g. set deadlines.
		orig := r.server.ConnContext
		r.server.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
			if orig != nil {
				ctx = orig(ctx, c)
			}
			return context.WithValue(ctx, connContextKey, c)
		}
	}
	r.serverLock.Unlock()

	if certFile != "" {
		return r.server.ListenAndServeTLS(certFile, keyFile)
	}

	return r.server.ListenAndServe()
}

// Listen starts the server listening on the specified `host:port` address.
func (r *Router) Listen(addr string) error {
	return r.listen(addr, "", "")
}

// ListenTLS listens for new connections using HTTPS & HTTP2
func (r *Router) ListenTLS(addr, certFile, keyFile string) error {
	return r.listen(addr, certFile, keyFile)
}

// ServeHTTP handles an incoming request and is compatible with the standard
// library `http` package.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !r.docsAreSetup {
		r.setupDocs()
	}

	r.mux.ServeHTTP(w, req)
}

// Shutdown gracefully shuts down the server.
func (r *Router) Shutdown(ctx context.Context) error {
	r.serverLock.Lock()
	defer r.serverLock.Unlock()

	if r.server == nil {
		panic("no server started")
	}
	return r.server.Shutdown(ctx)
}

// GetTitle returns the server API title.
func (r *Router) GetTitle() string {
	return r.title
}

// GetVersion returns the server version.
func (r *Router) GetVersion() string {
	return r.version
}

// DefaultBodyReadTimeout sets the amount of time an operation has to read
// the body of the incoming request before it is aborted. Defaults to 15
// seconds if not set.
func (r *Router) DefaultBodyReadTimeout(timeout time.Duration) {
	r.defaultBodyReadTimeout = timeout
}

// DefaultServerIdleTimeout sets the server's `IdleTimeout` value on startup.
// Defaults to 15 seconds if not set.
func (r *Router) DefaultServerIdleTimeout(timeout time.Duration) {
	r.defaultServerIdleTimeout = timeout
}

// URLPrefix sets the prefix to use when crafting non-relative links. If unset,
// then the incoming requests `Host` header is used and the scheme defaults to
// `https` unless the host starts with `localhost`. Do not include a
// trailing slash in the prefix. Examples:
// - https://example.com/v1
// - http://localhost
func (r *Router) URLPrefix(value string) {
	r.urlPrefix = value
}

// DisableSchemaProperty disables the creation of a `$schema` property in
// returned object response models.
func (r *Router) DisableSchemaProperty() {
	r.disableSchemaProperty = true
}

const (
	DefaultDocsSuffix    = "docs"
	DefaultSchemasSuffix = "schemas"
	DefaultSpecSuffix    = "openapi"
)

// New creates a new Huma router to which you can attach resources,
// operations, middleware, etc.
func New(docs, version string) *Router {
	title, desc := splitDocs(docs)

	r := &Router{
		mux:                      chi.NewRouter(),
		resources:                []*Resource{},
		title:                    title,
		description:              desc,
		version:                  version,
		servers:                  []oaServer{},
		securitySchemes:          map[string]oaSecurityScheme{},
		security:                 []map[string][]string{},
		defaultBodyReadTimeout:   15 * time.Second,
		defaultServerIdleTimeout: 15 * time.Second,
		docsSuffix:               DefaultDocsSuffix,
		schemasSuffix:            DefaultSchemasSuffix,
		specSuffix:               DefaultSpecSuffix,
	}

	r.docsHandler = RapiDocHandler(r)

	// Error handlers
	r.mux.NotFound(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ContextFromRequest(w, r)
		ctx.WriteError(http.StatusNotFound, fmt.Sprintf("Cannot find %s", r.URL.String()))
	}))

	r.mux.MethodNotAllowed(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := ContextFromRequest(w, r)
		ctx.WriteError(http.StatusMethodNotAllowed, fmt.Sprintf("No handler for method %s", r.Method))
	}))

	r.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Inject the operation info before other middleware so that the later
			// middleware will have access to it.
			req = req.WithContext(context.WithValue(req.Context(), opIDContextKey, &OperationInfo{}))

			next.ServeHTTP(w, req)

			// Automatically add links to OpenAPI and docs.
			if req.URL.Path == "/" {
				link := w.Header().Get("link")
				if link != "" {
					link += ", "
				}
				link += `<` + r.OpenAPIPath() + `>; rel="service-desc", <` + r.DocsPath() + `>; rel="service-doc"`
				w.Header().Set("link", link)
			}
		})
	})

	return r
}
