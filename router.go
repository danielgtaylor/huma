package huma

import (
	"context"
	"fmt"
	"net"
	"net/http"
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

	// Documentation handler function
	docsPrefix   string
	docsHandler  http.Handler
	docsAreSetup bool

	// Tracks the currently running server for graceful shutdown.
	server     *http.Server
	serverLock sync.Mutex

	// Allows modification of the generated OpenAPI.
	openapiHook func(*gabs.Container)
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
		Type:   "http",
		Scheme: "basic",
	}
}

// GatewayClientCredentials documents that the API gateway handles auth using
// OAuth2 client credentials (pre-shared secret).
func (r *Router) GatewayClientCredentials(name, tokenURL string, scopes map[string]string) {
	r.securitySchemes[name] = oaSecurityScheme{
		Type: "oauth2",
		Flows: oaFlows{
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
		Flows: oaFlows{
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

// OpenAPIPath returns the server path to the OpenAPI JSON.
func (r *Router) OpenAPIPath() string {
	return r.docsPrefix + "/openapi.json"
}

// DocsPrefix sets the path prefix for where the OpenAPI JSON and documentation
// are hosted.
func (r *Router) DocsPrefix(path string) {
	r.docsPrefix = path
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

// Set up the docs & OpenAPI routes.
func (r *Router) setupDocs() {
	// Register the docs handlers if needed.
	if !r.mux.Match(chi.NewRouteContext(), http.MethodGet, r.OpenAPIPath()) {
		r.mux.Get(r.OpenAPIPath(), func(w http.ResponseWriter, req *http.Request) {
			spec := r.OpenAPI()
			w.Header().Set("Content-Type", "application/vnd.oai.openapi+json")
			w.Write(spec.Bytes())
		})
	}

	if !r.mux.Match(chi.NewRouteContext(), http.MethodGet, "/docs") {
		r.mux.Get(r.docsPrefix+"/docs", r.docsHandler.ServeHTTP)
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
			IdleTimeout:       15 * time.Second,
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

// New creates a new Huma router to which you can attach resources,
// operations, middleware, etc.
func New(docs, version string) *Router {
	title, desc := splitDocs(docs)

	r := &Router{
		mux:             chi.NewRouter(),
		resources:       []*Resource{},
		title:           title,
		description:     desc,
		version:         version,
		servers:         []oaServer{},
		securitySchemes: map[string]oaSecurityScheme{},
		security:        []map[string][]string{},
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

	// Automatically add links to OpenAPI and docs.
	r.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req)

			if req.URL.Path == "/" {
				link := w.Header().Get("link")
				if link != "" {
					link += ", "
				}
				link += `<` + r.OpenAPIPath() + `>; rel="service-desc", <` + r.docsPrefix + `/docs>; rel="service-doc"`
				w.Header().Set("link", link)
			}
		})
	})

	return r
}
