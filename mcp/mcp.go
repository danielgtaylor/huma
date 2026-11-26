// Package mcp exposes Huma operations as Model Context Protocol (MCP) tools.
//
// It is an optional add-on: importing it has no effect on a Huma API until you
// call [Attach]. Operations opt in to being exposed as tools via the [Tool]
// operation modifier, so nothing is exposed to an MCP client unless you say so.
//
//	router := chi.NewMux()
//	api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))
//
//	huma.Get(api, "/greeting/{name}", greetingHandler, mcp.Tool())
//
//	mcp.Attach(api, mcp.Options{Name: "my-api", Version: "1.0.0"})
//	// MCP is now served over Streamable HTTP at /mcp.
//
// Tool calls are executed by round-tripping through the API's router, so all of
// Huma's request parsing, validation, middleware, and response marshalling are
// reused unchanged.
//
// # Authorization
//
// Because tool calls re-enter the API through its own router, authentication is
// whatever your existing middleware already does, enforced per operation, and
// identical whether the caller is curl or an MCP client. Nothing here needs to
// understand your tokens.
//
// It only implements the resource-server half of MCP authorization: discovery
// metadata (RFC 9728), [Server.SetChallenge], and forwarding the caller's
// credentials. Point [ProtectedResourceMetadata.AuthorizationServers] at
// whatever issues your tokens.
//
// One rule this package cannot enforce for you: your middleware MUST reject
// tokens that were not issued for this resource. Accepting a token minted for
// another audience makes the server a confused deputy, letting a token intended
// for some other service drive these tools.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
)

// maxBodyBytes caps a Streamable HTTP request body, matching Huma's default
// [huma.Operation.MaxBodyBytes].
const maxBodyBytes = 1024 * 1024

// metadataKey is the [huma.Operation] metadata key under which the per-operation
// tool configuration is stored by the [Tool] modifier.
const metadataKey = "mcp-tool"

// toolConfig holds the per-operation options set via [Tool] or [Exclude].
type toolConfig struct {
	description string
	exclude     bool
	name        string

	// operationID is the ID before any group prefixed it.
	operationID string
}

// ToolOption customizes how a single operation is exposed as an MCP tool.
type ToolOption func(*toolConfig)

// WithDescription overrides the tool description. By default the operation
// summary and description are combined.
func WithDescription(description string) ToolOption {
	return func(c *toolConfig) { c.description = description }
}

// WithName overrides the tool name. By default the operation ID is used
// (sanitized to the MCP tool-name character set).
func WithName(name string) ToolOption {
	return func(c *toolConfig) { c.name = name }
}

// Tool marks a Huma operation to be exposed as an MCP tool. Pass it as a
// trailing operation handler to [huma.Get], [huma.Post], etc., or apply it to a
// [huma.Operation] before calling [huma.Register]. Streaming (SSE) operations
// and ones whose request body is not JSON are never exposed, since they cannot
// work as tool calls.
//
//	huma.Post(api, "/things", createThing, mcp.Tool())
//	huma.Get(api, "/things/{id}", getThing, mcp.Tool(mcp.WithName("get_thing")))
func Tool(opts ...ToolOption) func(o *huma.Operation) {
	cfg := &toolConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return func(o *huma.Operation) {
		marked := *cfg
		marked.operationID = o.OperationID
		mark(o, &marked)
	}
}

// Exclude marks a Huma operation to never be exposed as an MCP tool. It is only
// meaningful when [Options.ExposeVisible] or [Options.ExposeHidden] is set; in
// the default opt-in mode, operations are already excluded unless marked with
// [Tool].
func Exclude() func(o *huma.Operation) {
	return func(o *huma.Operation) {
		mark(o, &toolConfig{exclude: true})
	}
}

// WithTool returns a copy of op marked as an MCP tool. Use it with
// [huma.Register], which (unlike [huma.Get], [huma.Post], etc.) takes an
// operation value directly and does not accept trailing operation modifiers:
//
//	huma.Register(api, mcp.WithTool(huma.Operation{
//		OperationID: "create-ticket",
//		Method:      http.MethodPost,
//		Path:        "/tickets",
//		// ...
//	}), createTicketHandler)
//
// It is equivalent to applying [Tool](opts...) to the operation.
func WithTool(op huma.Operation, opts ...ToolOption) huma.Operation {
	Tool(opts...)(&op)
	return op
}

// WithExclude returns a copy of op marked with [Exclude], for use with
// [huma.Register] like [WithTool].
func WithExclude(op huma.Operation) huma.Operation {
	Exclude()(&op)
	return op
}

// mark stores cfg in a copy of o's metadata, since operations built from one
// template share its map.
func mark(o *huma.Operation, cfg *toolConfig) {
	o.Metadata = maps.Clone(o.Metadata)
	if o.Metadata == nil {
		o.Metadata = map[string]any{}
	}
	o.Metadata[metadataKey] = cfg
}

// Options configures an MCP server attached to a Huma API.
type Options struct {
	// AllowedOrigins lists exact Origin values allowed on the transport, guarding
	// against DNS rebinding; others get 403. Requests without an Origin are
	// allowed. Empty allows any; set it whenever ForwardHeaders includes "Cookie".
	AllowedOrigins []string

	// ExposeHidden exposes every Hidden operation unless marked with [Exclude].
	// Operations added later are exposed too; pair it with Filter to offer each
	// caller only the tools it can use.
	//
	// Neither expose flag covers the MCP transport or Huma's docs and schema
	// routes.
	ExposeHidden bool

	// ExposeVisible exposes every non-hidden operation unless marked with
	// [Exclude]. Operations added later are exposed too.
	ExposeVisible bool

	// Filter, if set, decides per request which tools the caller can see and
	// call. It receives the incoming MCP request's context, carrying whatever
	// your middleware attached (such as the authenticated user), and the
	// operation behind the tool. A tool it rejects is left out of tools/list and
	// reported as unknown by tools/call, so a caller is only offered what it can
	// use, e.g. admin tools for admins.
	//
	// It narrows what is offered; it is not the authorization boundary. Every
	// tool call still runs through the operation's own middleware.
	Filter func(ctx context.Context, op *huma.Operation) bool

	// ForwardHeaders lists request header names copied from the incoming MCP
	// request onto each internal tool-execution request. This is how a caller's
	// credentials reach the operation's own middleware (which re-runs during the
	// tool call and enforces auth per operation). Defaults to
	// {"Authorization"}. Set to a non-nil empty slice to forward nothing.
	// Header parameters named here are not tool arguments, so the caller's value
	// is always the one sent.
	//
	// Cookie is not forwarded by default: browsers attach it on their own, so any
	// page that can reach this endpoint could run tools as the visitor. Set
	// AllowedOrigins if you add it.
	ForwardHeaders []string

	// Instructions is optional guidance returned to clients during
	// initialization describing how to use the exposed tools.
	Instructions string

	// Name of the MCP server, reported to clients during initialization. If
	// empty, the API's OpenAPI title is used.
	Name string

	// Path is the HTTP route for the Streamable HTTP transport. Defaults to
	// "/mcp".
	Path string

	// ProtectedResource, if set, is served at ResourceMetadataPath (RFC 9728) and
	// referenced by [Server.SetChallenge]. No tokens are validated or issued.
	ProtectedResource *ProtectedResourceMetadata

	// PublicURL is the external base URL (e.g. "https://api.example.com") used in
	// resource metadata and challenges. Set it in production: otherwise it comes
	// from the client-controlled Host header, which clients bind token audience to.
	PublicURL string

	// ResourceMetadataPath is the route at which ProtectedResource is served.
	// Defaults to "/.well-known/oauth-protected-resource" followed by Path, where
	// RFC 9728 looks for it when PublicURL has no path. The endpoint is public
	// (no Security), as required for discovery.
	ResourceMetadataPath string

	// Security is set on the transport operation so middleware keyed on
	// Operation().Security protects the MCP endpoint itself. Tools keep their own
	// auth.
	Security []map[string][]string

	// Version of the MCP server, reported to clients during initialization. If
	// empty, the API's OpenAPI version is used.
	Version string
}

// Server is an MCP server backed by a set of Huma operations. Create one with
// [Attach]. It is safe for concurrent use.
type Server struct {
	api  huma.API
	opts Options

	// mu guards the fields below it.
	mu    sync.RWMutex
	index map[string]*toolEntry
	tools []*toolEntry
}

// Attach wires an MCP server onto an existing Huma API and returns a drop-in
// replacement API to register operations against, plus the [Server].
//
// Register operations against the returned API (not the original) so they can
// be exposed as MCP tools. Because it intercepts operation registration via
// [huma.OperationDocumenter], it captures Hidden operations too, which never
// appear in the OpenAPI document:
//
//	api := humachi.New(router, config)
//	api, server := mcp.Attach(api, mcp.Options{Name: "my-api", Version: "1.0.0"})
//
//	huma.Get(api, "/admin/reset", resetHandler, mcp.Tool(), func(o *huma.Operation) {
//		o.Hidden = true // hidden from docs, still callable as an MCP tool
//	})
//
// Documented operations registered against the original API are still found;
// Hidden ones are not. api must be the root API, not a [huma.Group]: create
// groups from the API Attach returns.
//
// Attach registers the Streamable HTTP transport (see [Options.Path]).
func Attach(api huma.API, opts Options) (huma.API, *Server) {
	if _, ok := api.(*huma.Group); ok {
		panic("mcp: attach to the root API, then create groups from the API it returns")
	}

	oapi := api.OpenAPI()

	if opts.Name == "" && oapi.Info != nil {
		opts.Name = oapi.Info.Title
	}
	if opts.Version == "" && oapi.Info != nil {
		opts.Version = oapi.Info.Version
	}
	if opts.Path == "" {
		opts.Path = "/mcp"
	}
	if opts.ForwardHeaders == nil {
		opts.ForwardHeaders = []string{"Authorization"}
	}
	if opts.ResourceMetadataPath == "" {
		opts.ResourceMetadataPath = "/.well-known/oauth-protected-resource" + opts.Path
	}

	opts.PublicURL = strings.TrimRight(opts.PublicURL, "/")

	s := &Server{
		api:   api,
		index: map[string]*toolEntry{},
		opts:  opts,
	}

	if err := s.syncDocumented(); err != nil {
		panic(err)
	}

	// Documented operations arrive here, even when registered against the
	// original API; Hidden ones only reach documentingAPI.
	oapi.OnAddOperation = append(oapi.OnAddOperation, func(_ *huma.OpenAPI, op *huma.Operation) {
		if err := s.consider(op); err != nil {
			panic(err)
		}
	})

	s.registerHTTP()

	return &documentingAPI{API: api, server: s}, s
}

// documentingAPI wraps a [huma.API] so the MCP server sees Hidden operations,
// which never reach the OpenAPI document, while leaving documentation behavior
// unchanged.
type documentingAPI struct {
	huma.API
	server *Server
}

var _ huma.API = (*documentingAPI)(nil)

// configProvider mirrors Huma's internal interface so getConfig keeps working
// when an operation is registered against the wrapper.
type configProvider interface {
	Config() huma.Config
}

// DocumentOperation implements [huma.OperationDocumenter]. It considers a
// Hidden operation as a tool, then reproduces Huma's normal documentation
// behavior for the wrapped API, whose OnAddOperation hook covers the rest.
func (w *documentingAPI) DocumentOperation(op *huma.Operation) {
	if op.Hidden {
		if err := w.server.consider(op); err != nil {
			panic(err)
		}
	}

	if doc, ok := w.API.(huma.OperationDocumenter); ok {
		doc.DocumentOperation(op)
		return
	}
	if !op.Hidden {
		w.API.OpenAPI().AddOperation(op)
	}
}

// Config forwards the wrapped API's configuration so that Huma's internal
// getConfig lookups resolve correctly through the wrapper.
func (w *documentingAPI) Config() huma.Config {
	if cp, ok := w.API.(configProvider); ok {
		return cp.Config()
	}
	return huma.Config{}
}

// consider inspects an operation and adds it to the tool catalog when it should
// be exposed. An explicit [Tool] always includes and an explicit [Exclude]
// always excludes; otherwise the [Options.ExposeVisible] / [Options.ExposeHidden]
// flags decide based on whether the operation is Hidden. A duplicate tool name
// is an error.
func (s *Server) consider(op *huma.Operation) error {
	cfg, marked := op.Metadata[metadataKey].(*toolConfig)
	switch {
	case marked && cfg.exclude, !toolable(op):
		return nil
	case marked:
	case op.Hidden && s.opts.ExposeHidden, !op.Hidden && s.opts.ExposeVisible:
		cfg = &toolConfig{}
	default:
		return nil
	}

	entry := newToolEntry(op, cfg, s.api.OpenAPI().Components.Schemas, s.opts.ForwardHeaders)

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.index[entry.name]; ok {
		return fmt.Errorf("duplicate MCP tool name %s: operations %s and %s", entry.name, existing.op.OperationID, op.OperationID)
	}

	s.tools = append(s.tools, entry)
	s.index[entry.name] = entry

	return nil
}

// toolable reports whether an operation can work as a tool call: a streamed
// response never completes, and only a JSON request body becomes arguments.
func toolable(op *huma.Operation) bool {
	for _, resp := range op.Responses {
		if resp.Content["text/event-stream"] != nil {
			return false
		}
	}

	return op.RequestBody == nil || op.RequestBody.Content["application/json"] != nil
}

// registerHTTP registers the Streamable HTTP transport on the API's router.
// The transport operation carries [Options.Security] so that API middleware
// keyed off Operation().Security can gate the MCP endpoint.
func (s *Server) registerHTTP() {
	a := s.api.Adapter()
	security := s.opts.Security

	a.Handle(&huma.Operation{Method: http.MethodPost, Path: s.opts.Path, Security: security}, s.serveTransport(s.handleHTTPPost))

	// The Streamable HTTP spec allows a server that does not offer a
	// server-initiated SSE stream to reject GET with 405.
	a.Handle(&huma.Operation{Method: http.MethodGet, Path: s.opts.Path, Security: security}, s.serveTransport(func(ctx huma.Context) {
		ctx.SetHeader("Allow", "POST")
		ctx.SetStatus(http.StatusMethodNotAllowed)
	}))

	// Serve OAuth Protected Resource Metadata for discovery, if configured. It
	// is registered without API middleware and without a Security requirement so
	// that it is always publicly reachable, as discovery requires.
	if s.opts.ProtectedResource != nil {
		a.Handle(&huma.Operation{Method: http.MethodGet, Path: s.opts.ResourceMetadataPath}, s.handleResourceMetadata)
	}
}

// serveTransport wraps a transport handler with the Origin check and the API's
// middleware. Middleware is resolved per request because auth middleware is
// usually added after Attach, and a captured chain would leave the transport
// unauthenticated.
func (s *Server) serveTransport(handler func(huma.Context)) func(huma.Context) {
	return func(ctx huma.Context) {
		if !s.originAllowed(ctx.Header("Origin")) {
			ctx.SetStatus(http.StatusForbidden)
			return
		}

		ctx = huma.WithValue(ctx, toolPrefixKey{}, s.toolPrefix(ctx))
		s.api.Middlewares().Handler(handler)(ctx)
	}
}

// originAllowed reports whether a request bearing this Origin may use the
// transport. See [Options.AllowedOrigins] for why this is checked at all. An
// absent Origin, or no configured list, allows the request.
func (s *Server) originAllowed(origin string) bool {
	if origin == "" || len(s.opts.AllowedOrigins) == 0 {
		return true
	}

	for _, allowed := range s.opts.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}

	return false
}

// handleHTTPPost processes a single Streamable HTTP POST containing one or more
// JSON-RPC messages.
func (s *Server) handleHTTPPost(ctx huma.Context) {
	reader := ctx.BodyReader()
	if reader == nil {
		reader = bytes.NewReader(nil)
	}

	// The transport is registered on the adapter directly, so Huma's per-operation
	// body limit does not apply; enforce its default here, as Huma would.
	body, err := io.ReadAll(http.MaxBytesReader(nil, io.NopCloser(reader), maxBodyBytes))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			ctx.SetStatus(http.StatusRequestEntityTooLarge)
			return
		}
		ctx.SetStatus(http.StatusBadRequest)
		return
	}

	// Capture headers to forward onto internal tool-execution requests (e.g. the
	// caller's Authorization), so each tool call is authenticated as the caller.
	reqCtx := withOrigin(withForwardedHeaders(ctx, s.opts.ForwardHeaders), ctx)

	// Turn a tool's 401 into a transport challenge only when ProtectedResource is
	// set, so APIs that don't opt in keep their current tool errors.
	var challenged *bool
	if s.opts.ProtectedResource != nil {
		reqCtx, challenged = withChallengeFlag(reqCtx)
	}

	resp := s.handleRPC(reqCtx, body, ctx.Header)

	if challenged != nil && *challenged {
		s.SetChallenge(ctx)
		ctx.SetStatus(http.StatusUnauthorized)
		return
	}

	if resp == nil {
		// The payload was a notification; acknowledge it.
		ctx.SetStatus(http.StatusAccepted)
		return
	}

	// Set explicitly rather than left to the implicit 200 of the first write:
	// middleware reading ctx.Status() (request logs, metrics) would see 0.
	status := http.StatusOK
	if resp.Error != nil && resp.Error.status != 0 {
		status = resp.Error.status
	}
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(status)
	ctx.BodyWriter().Write(marshalResponse(resp))
}

// handleRPC parses and dispatches a single JSON-RPC message and returns the
// response, or nil for a notification. header reads the request's HTTP headers.
//
// Batches are rejected: no revision this package serves allows them.
func (s *Server) handleRPC(ctx context.Context, data []byte, header func(string) string) *jsonRPCResponse {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil
	}
	if data[0] == '[' {
		return errorResponse(nil, &jsonRPCError{Code: codeInvalidRequest, Message: "batch requests are not supported"})
	}

	var req jsonRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return errorResponse(nil, &jsonRPCError{Code: codeParseError, Message: "parse error"})
	}

	// Notifications get no response, and no method here acts on one.
	if len(req.ID) == 0 {
		return nil
	}

	// Malformed params are left for the method to reject; only the fields
	// needed to pick the revision are read here.
	var params requestParams
	_ = json.Unmarshal(req.Params, &params)

	if isStateless(params, header) {
		if rpcErr := checkStateless(req, params, header); rpcErr != nil {
			return errorResponse(req.ID, rpcErr)
		}

		switch req.Method {
		case "server/discover":
			return successResponse(req.ID, s.handleDiscover())
		case "tools/list", "tools/call":
			return s.dispatchTools(ctx, req)
		default:
			return errorResponse(req.ID, &jsonRPCError{Code: codeMethodNotFound, Message: "method not found: " + req.Method, status: http.StatusNotFound})
		}
	}

	// Streamable HTTP requires a 400 for a version header this server does not
	// serve, such as one from a revision older than handshakeVersions.
	if v := header("MCP-Protocol-Version"); v != "" && !slices.Contains(handshakeVersions, v) {
		return errorResponse(req.ID, unsupportedVersion(v))
	}

	switch req.Method {
	case "initialize":
		return successResponse(req.ID, s.handleInitialize(req))
	case "tools/list", "tools/call":
		return s.dispatchTools(ctx, req)
	case "ping":
		return successResponse(req.ID, s.complete())
	default:
		return errorResponse(req.ID, &jsonRPCError{Code: codeMethodNotFound, Message: "method not found: " + req.Method})
	}
}

// dispatchTools handles the tool methods, which both revisions share.
func (s *Server) dispatchTools(ctx context.Context, req jsonRPCRequest) *jsonRPCResponse {
	if req.Method == "tools/list" {
		return successResponse(req.ID, s.handleToolsList(ctx))
	}

	result, rpcErr := s.handleToolsCall(ctx, req.Params)
	if rpcErr != nil {
		return errorResponse(req.ID, rpcErr)
	}
	return successResponse(req.ID, result)
}

// complete returns the fields shared by every successful result.
func (s *Server) complete() result {
	return result{ResultType: "complete", Meta: &resultMeta{ServerInfo: implementation{Name: s.opts.Name, Version: s.opts.Version}}}
}

// handleDiscover answers server/discover, which stateless-revision clients may
// call to learn the supported versions before anything else.
func (s *Server) handleDiscover() discoverResult {
	return discoverResult{
		result:            s.complete(),
		cacheable:         cacheable{TTLMs: cacheTTL.Milliseconds(), CacheScope: "public"},
		SupportedVersions: supportedVersions,
		Capabilities:      serverCapabilities{Tools: &toolsCapability{ListChanged: false}},
		Instructions:      s.opts.Instructions,
	}
}

// handleInitialize opens a handshake-based session. It answers with the
// requested version when that is one served, and otherwise with the newest,
// as the spec requires; the client decides whether it can proceed.
func (s *Server) handleInitialize(req jsonRPCRequest) initializeResult {
	var params initializeParams
	_ = json.Unmarshal(req.Params, &params)

	version := handshakeVersions[0]
	if slices.Contains(handshakeVersions, params.ProtocolVersion) {
		version = params.ProtocolVersion
	}

	return initializeResult{
		result:          s.complete(),
		ProtocolVersion: version,
		Capabilities: serverCapabilities{
			Tools: &toolsCapability{ListChanged: false},
		},
		ServerInfo: implementation{
			Name:    s.opts.Name,
			Version: s.opts.Version,
		},
		Instructions: s.opts.Instructions,
	}
}

func (s *Server) handleToolsList(ctx context.Context) toolsListResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tools := make([]toolDefinition, 0, len(s.tools))
	for _, e := range s.tools {
		if !s.allowed(ctx, e) {
			continue
		}
		tools = append(tools, toolDefinition{
			Name:        e.name,
			Description: e.description,
			InputSchema: e.inputSchema,
		})
	}

	// A Filter makes the list depend on the caller, so shared caches must not
	// serve one caller's list to another.
	scope := "public"
	if s.opts.Filter != nil {
		scope = "private"
	}

	return toolsListResult{
		result:    s.complete(),
		cacheable: cacheable{TTLMs: cacheTTL.Milliseconds(), CacheScope: scope},
		Tools:     tools,
	}
}

func (s *Server) handleToolsCall(ctx context.Context, params json.RawMessage) (*callToolResult, *jsonRPCError) {
	var p callToolParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &jsonRPCError{Code: codeInvalidParams, Message: "invalid params"}
	}

	entry := s.lookup(p.Name)

	// A filtered tool is reported exactly as a missing one, so a caller cannot
	// probe for tools it was not offered.
	if entry == nil || !s.allowed(ctx, entry) {
		return nil, &jsonRPCError{Code: codeInvalidParams, Message: "unknown tool: " + p.Name}
	}

	res := entry.call(ctx, s.api, p.Arguments)
	res.result = s.complete()
	return res, nil
}

// allowed reports whether [Options.Filter] offers the tool to this caller.
func (s *Server) allowed(ctx context.Context, e *toolEntry) bool {
	return s.opts.Filter == nil || s.opts.Filter(ctx, e.op)
}

func (s *Server) lookup(name string) *toolEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.index[name]
}

// syncDocumented considers the operations documented before Attach, in path
// order so the tool list is the same on every start.
func (s *Server) syncDocumented() error {
	paths := s.api.OpenAPI().Paths

	var errs []error
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		item := paths[path]
		for _, op := range []*huma.Operation{
			item.Get, item.Post, item.Put, item.Patch,
			item.Delete, item.Head, item.Options, item.Trace,
		} {
			if op == nil {
				continue
			}
			if err := s.consider(op); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}
