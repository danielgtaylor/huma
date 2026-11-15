package mcp

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// ProtocolVersion is the latest MCP protocol revision this package implements:
// the stateless revision, where every request carries its protocol version and
// client capabilities in _meta.
//
// Clients of earlier, handshake-based revisions are served too. A request is
// handled under the stateless revision when it declares a version in _meta, or
// names one in the MCP-Protocol-Version header that no handshake-based revision
// uses; anything else is handled as the handshake-based revision negotiated by
// initialize.
const ProtocolVersion = "2026-07-28"

// cacheTTL is the freshness hint on cacheable results. Tools are fixed once the
// API's operations are registered, so clients may reuse a list for a while.
const cacheTTL = 5 * time.Minute

// handshakeVersions are the handshake-based revisions served to clients that
// open with initialize, newest first. A client requesting any other version is
// offered the first.
var handshakeVersions = []string{"2025-11-25", "2025-06-18"}

// supportedVersions are all revisions served, as advertised by server/discover
// and in UnsupportedProtocolVersionError.
var supportedVersions = append([]string{ProtocolVersion}, handshakeVersions...)

// handshakeEra are the revisions that negotiate with initialize rather than
// declaring a version per request, including those older than this package
// serves. Their clients send no _meta, so the header alone identifies them.
var handshakeEra = append(slices.Clone(handshakeVersions), "2025-03-26", "2024-11-05")

// isStateless reports whether a request uses the stateless revision.
func isStateless(params requestParams, header func(string) string) bool {
	if params.Meta.ProtocolVersion != "" {
		return true
	}
	v := header("MCP-Protocol-Version")
	return v != "" && !slices.Contains(handshakeEra, v)
}

// checkStateless validates a stateless-revision request before dispatch: the
// required _meta fields, the Streamable HTTP headers that mirror the body, and
// the requested version.
func checkStateless(req jsonRPCRequest, params requestParams, header func(string) string) *jsonRPCError {
	if params.Meta.ProtocolVersion == "" || params.Meta.ClientCapabilities == nil {
		return &jsonRPCError{
			Code:    codeInvalidParams,
			Message: "missing required _meta fields io.modelcontextprotocol/protocolVersion and io.modelcontextprotocol/clientCapabilities",
			status:  http.StatusBadRequest,
		}
	}

	if mismatch := headerMismatch(req, params, header); mismatch != "" {
		return &jsonRPCError{Code: codeHeaderMismatch, Message: "Header mismatch: " + mismatch, status: http.StatusBadRequest}
	}

	if params.Meta.ProtocolVersion != ProtocolVersion {
		return unsupportedVersion(params.Meta.ProtocolVersion)
	}

	return nil
}

// unsupportedVersion is the error for a requested revision this package does
// not serve.
func unsupportedVersion(requested string) *jsonRPCError {
	return &jsonRPCError{
		Code:    codeUnsupportedProtocolVersion,
		Message: "Unsupported protocol version",
		Data:    map[string]any{"supported": supportedVersions, "requested": requested},
		status:  http.StatusBadRequest,
	}
}

// headerMismatch describes the first Streamable HTTP request header that is
// missing or disagrees with the body, or returns "" if they all match. The body
// is what executes, so a header that differs could misdirect an intermediary
// routing on it.
func headerMismatch(req jsonRPCRequest, params requestParams, header func(string) string) string {
	if v := header("MCP-Protocol-Version"); v != params.Meta.ProtocolVersion {
		return fmt.Sprintf("MCP-Protocol-Version header value %q does not match body value %q", v, params.Meta.ProtocolVersion)
	}
	if v := header("Mcp-Method"); v != req.Method {
		return fmt.Sprintf("Mcp-Method header value %q does not match body value %q", v, req.Method)
	}
	if req.Method == "tools/call" {
		if v, ok := decodeHeaderValue(header("Mcp-Name")); !ok || v != params.Name {
			return fmt.Sprintf("Mcp-Name header value %q does not match body value %q", header("Mcp-Name"), params.Name)
		}
	}
	return ""
}

// decodeHeaderValue undoes the =?base64?...?= sentinel encoding clients use for
// header values that are not plain ASCII, reporting false if it is malformed.
func decodeHeaderValue(v string) (string, bool) {
	inner, ok := strings.CutPrefix(v, "=?base64?")
	if !ok {
		return v, true
	}
	inner, ok = strings.CutSuffix(inner, "?=")
	if !ok {
		return "", false
	}
	b, err := base64.StdEncoding.DecodeString(inner)
	if err != nil {
		return "", false
	}
	return string(b), true
}
