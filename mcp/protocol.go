package mcp

import (
	"encoding/json"
)

// JSON-RPC 2.0 and MCP error codes used by this package.
const (
	codeParseError                 = -32700
	codeInvalidRequest             = -32600
	codeMethodNotFound             = -32601
	codeInvalidParams              = -32602
	codeInternalError              = -32603
	codeHeaderMismatch             = -32020
	codeUnsupportedProtocolVersion = -32022
)

// jsonRPCRequest is an incoming JSON-RPC 2.0 request or notification. A message
// with no ID is a notification and expects no response.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// requestParams holds the request fields read before dispatch: the per-request
// protocol metadata of the stateless revision, and the name mirrored into the
// Mcp-Name header on Streamable HTTP.
type requestParams struct {
	Meta struct {
		ProtocolVersion    string          `json:"io.modelcontextprotocol/protocolVersion"`
		ClientCapabilities json.RawMessage `json:"io.modelcontextprotocol/clientCapabilities"`
	} `json:"_meta"`
	Name string `json:"name"`
}

// jsonRPCResponse is an outgoing JSON-RPC 2.0 response. Exactly one of Result or
// Error is set. ID is always present: JSON-RPC requires null when the request's
// ID could not be determined, such as for a parse error.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

// jsonRPCError is a JSON-RPC 2.0 error object. status, when set, is the HTTP
// status the Streamable HTTP transport must answer with; the stateless revision
// assigns 400 or 404 to some errors.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`

	status int
}

func successResponse(id json.RawMessage, result any) *jsonRPCResponse {
	return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func errorResponse(id json.RawMessage, err *jsonRPCError) *jsonRPCResponse {
	return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: err}
}

// result holds the fields every result carries. The stateless revision requires
// resultType and asks servers to identify themselves in _meta; handshake-era
// clients ignore both.
type result struct {
	ResultType string      `json:"resultType"`
	Meta       *resultMeta `json:"_meta,omitempty"`
}

type resultMeta struct {
	ServerInfo implementation `json:"io.modelcontextprotocol/serverInfo"`
}

// cacheable holds the caching hints the stateless revision requires on
// server/discover and list results.
type cacheable struct {
	TTLMs      int64  `json:"ttlMs"`
	CacheScope string `json:"cacheScope"`
}

type initializeParams struct {
	ProtocolVersion string `json:"protocolVersion"`
}

type initializeResult struct {
	result
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      implementation     `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

type discoverResult struct {
	result
	cacheable
	SupportedVersions []string           `json:"supportedVersions"`
	Capabilities      serverCapabilities `json:"capabilities"`
	Instructions      string             `json:"instructions,omitempty"`
}

type implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type serverCapabilities struct {
	Tools *toolsCapability `json:"tools,omitempty"`
}

type toolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type toolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type toolsListResult struct {
	result
	cacheable
	Tools []toolDefinition `json:"tools"`
}

type callToolParams struct {
	Name      string                     `json:"name"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

// contentBlock is a single piece of tool result content. This package emits
// text blocks only.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type callToolResult struct {
	result
	Content           []contentBlock `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

func marshalResponse(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// Fall back to a generic internal error that is always marshalable.
		b, _ = json.Marshal(errorResponse(nil, &jsonRPCError{Code: codeInternalError, Message: "internal error"}))
	}
	return b
}
