package mcp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

var (
	rxRef             = regexp.MustCompile(`"\$ref":"([^"]+)"`)
	rxToolNameInvalid = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
)

// forwardedHeadersKey is the context key under which headers captured from the
// incoming MCP request are stored for propagation onto tool-execution requests.
type forwardedHeadersKey struct{}

// withForwardedHeaders returns a context carrying the named headers read from
// the incoming MCP request, for propagation onto internal tool-execution
// requests. Header names absent from the request are skipped.
func withForwardedHeaders(ctx huma.Context, names []string) context.Context {
	h := http.Header{}
	for _, name := range names {
		if v := ctx.Header(name); v != "" {
			h.Set(name, v)
		}
	}
	if len(h) == 0 {
		return ctx.Context()
	}
	return context.WithValue(ctx.Context(), forwardedHeadersKey{}, h)
}

// originKey is the context key under which the incoming MCP request's
// connection details are stored for tool-execution requests.
type originKey struct{}

// origin holds the incoming request's connection details. Without them a
// tool-execution request looks like an anonymous plain-HTTP localhost call to
// middleware keyed on the client address (rate limiters, IP allowlists) and to
// anything building absolute URLs from the host (such as $schema links).
type origin struct {
	host       string
	remoteAddr string
	tls        *tls.ConnectionState
}

// withOrigin returns ctx carrying the connection details of the request in req.
func withOrigin(ctx context.Context, req huma.Context) context.Context {
	return context.WithValue(ctx, originKey{}, origin{host: req.Host(), remoteAddr: req.RemoteAddr(), tls: req.TLS()})
}

// argLoc identifies where a tool argument maps back to in the underlying HTTP
// request.
type argLoc int

const (
	locPath argLoc = iota
	locQuery
	locHeader
	locCookie
	locBodyField // a single property flattened out of the request body
	locBody      // the whole request body (non-object or name-collision case)
)

// argBinding records how one top-level property in the tool's input schema maps
// back onto the HTTP request that executes the operation.
type argBinding struct {
	arg string // property name in the MCP input schema
	loc argLoc
	key string // path/query/header/cookie name, or body field name

	// explode and deepObject mirror the parameter's serialization, so array and
	// object values are sent in the form Huma parses: ?tags=a,b by default,
	// ?tags=a&tags=b when exploded, ?filter[k]=v for deepObject.
	explode    bool
	deepObject bool

	// nullable marks a body or body field whose schema accepts null, so an
	// explicit null is sent rather than treated as omitted.
	nullable bool
}

// toolEntry is a compiled MCP tool derived from a Huma operation.
type toolEntry struct {
	op          *huma.Operation
	name        string
	description string
	inputSchema json.RawMessage
	bindings    []argBinding
}

func newToolEntry(op *huma.Operation, cfg *toolConfig, reg huma.Registry, forward []string) *toolEntry {
	name := cfg.name
	switch {
	case name == "":
		name = op.OperationID
	case cfg.operationID != "" && op.OperationID != cfg.operationID && strings.HasSuffix(op.OperationID, "-"+cfg.operationID):
		// Prefix the name as huma.PrefixModifier prefixed the ID.
		name = strings.TrimSuffix(op.OperationID, cfg.operationID) + name
	}
	name = sanitizeToolName(name)

	description := cfg.description
	if description == "" {
		description = strings.TrimSpace(op.Summary + "\n\n" + op.Description)
	}

	schema, bindings := buildInputSchema(op, reg, forward)

	return &toolEntry{
		op:          op,
		name:        name,
		description: description,
		inputSchema: schema,
		bindings:    bindings,
	}
}

// sanitizeToolName coerces a string into the MCP tool-name character set
// (letters, digits, underscore, hyphen) and length limit.
func sanitizeToolName(name string) string {
	name = rxToolNameInvalid.ReplaceAllString(name, "_")
	if len(name) > 128 {
		name = name[:128]
	}
	if name == "" {
		name = "tool"
	}
	return name
}

// buildInputSchema synthesizes a single JSON Schema for the tool from the
// operation's parameters and request body, returning it alongside the bindings
// needed to reverse the mapping at call time. Header parameters named in forward
// are left out, since the caller's own header fills them.
func buildInputSchema(op *huma.Operation, reg huma.Registry, forward []string) (json.RawMessage, []argBinding) {
	properties := map[string]json.RawMessage{}
	var required []string
	var bindings []argBinding

	add := func(schema *huma.Schema, isRequired bool, b argBinding) {
		raw, err := json.Marshal(schema)
		if err != nil {
			return
		}
		properties[b.arg] = raw
		if isRequired {
			required = append(required, b.arg)
		}
		bindings = append(bindings, b)
	}

	for _, p := range op.Parameters {
		if p.In == "header" && slices.ContainsFunc(forward, func(h string) bool { return strings.EqualFold(h, p.Name) }) {
			continue
		}

		loc := locQuery
		switch p.In {
		case "path":
			loc = locPath
		case "header":
			loc = locHeader
		case "cookie":
			loc = locCookie
		}
		schema := p.Schema
		if schema != nil && p.Description != "" && schema.Description == "" {
			// Surface the parameter description on the property so the LLM sees it.
			cp := *schema
			cp.Description = p.Description
			schema = &cp
		}
		add(schema, p.Required, argBinding{
			arg:        freeName(properties, p.Name),
			loc:        loc,
			key:        p.Name,
			explode:    p.Explode != nil && *p.Explode,
			deepObject: p.Style == "deepObject",
		})
	}

	if op.RequestBody != nil {
		if mt := op.RequestBody.Content["application/json"]; mt != nil && mt.Schema != nil {
			bindBody(mt.Schema, op.RequestBody.Required, reg, properties, add)
		}
	}

	root := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		root["required"] = required
	}

	rootJSON, _ := json.Marshal(root)

	// Attach any referenced component schemas under $defs and rewrite refs to
	// point at them, so the tool schema is self-contained.
	defs := collectDefs(rootJSON, reg)
	if len(defs) > 0 {
		root["$defs"] = defs
		rootJSON, _ = json.Marshal(root)
	}
	rootJSON = rxRef.ReplaceAllFunc(rootJSON, func(match []byte) []byte {
		name := refName(string(rxRef.FindSubmatch(match)[1]))
		if _, ok := defs[name]; !ok {
			return match
		}
		return []byte(`"$ref":"#/$defs/` + name + `"`)
	})

	return rootJSON, bindings
}

// bindBody merges the request body schema into the tool's top-level properties.
// Object bodies are flattened field-by-field (nicer for LLMs); anything else,
// or a name collision with an existing parameter, is nested under "body".
// Fields are added in name order so the published schema is stable.
func bindBody(
	bodySchema *huma.Schema,
	bodyRequired bool,
	reg huma.Registry,
	properties map[string]json.RawMessage,
	add func(schema *huma.Schema, isRequired bool, b argBinding),
) {
	resolved := resolveSchema(reg, bodySchema)

	// Only named fields can be flattened, so extra keys keep the body whole.
	flatten := resolved != nil && resolved.Type == "object" && len(resolved.Properties) > 0 &&
		(resolved.AdditionalProperties == nil || resolved.AdditionalProperties == false)
	if flatten {
		for name := range resolved.Properties {
			if _, exists := properties[name]; exists {
				flatten = false // collision: fall back to nesting
				break
			}
		}
	}

	if flatten {
		// An optional body may be omitted, so its fields can't be required.
		reqSet := map[string]bool{}
		if bodyRequired {
			for _, r := range resolved.Required {
				reqSet[r] = true
			}
		}
		for _, name := range slices.Sorted(maps.Keys(resolved.Properties)) {
			field := resolved.Properties[name]
			// Huma does not require readOnly fields on writes.
			add(field, reqSet[name] && !field.ReadOnly, argBinding{arg: name, loc: locBodyField, key: name, nullable: field.Nullable})
		}
		return
	}

	// Non-object body (array/scalar), an unresolvable ref, or a collision: expose
	// the whole body.
	add(bodySchema, bodyRequired, argBinding{arg: freeName(properties, "body"), loc: locBody, nullable: resolved != nil && resolved.Nullable})
}

// freeName prefixes name with underscores until no property uses it.
func freeName(properties map[string]json.RawMessage, name string) string {
	for {
		if _, exists := properties[name]; !exists {
			return name
		}
		name = "_" + name
	}
}

// resolveSchema follows $ref chains to the concrete schema in the registry. It
// returns nil for a cycle or a ref the registry cannot resolve.
func resolveSchema(reg huma.Registry, s *huma.Schema) *huma.Schema {
	seen := map[string]bool{}
	for s != nil && s.Ref != "" {
		if seen[s.Ref] {
			return nil
		}
		seen[s.Ref] = true
		s = reg.SchemaFromRef(s.Ref)
	}

	return s
}

// collectDefs returns the schemas reachable from root's refs, keyed by name for
// $defs. Refs resolve through the registry, so any ref prefix works.
func collectDefs(root []byte, reg huma.Registry) map[string]json.RawMessage {
	defs := map[string]json.RawMessage{}
	queue := refs(root)

	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]
		name := refName(ref)
		if _, done := defs[name]; done {
			continue
		}
		schema := reg.SchemaFromRef(ref)
		if schema == nil {
			continue
		}
		b, err := json.Marshal(schema)
		if err != nil {
			continue
		}
		defs[name] = b
		queue = append(queue, refs(b)...)
	}

	return defs
}

// refName returns a ref's last path segment.
func refName(ref string) string {
	return ref[strings.LastIndex(ref, "/")+1:]
}

// refs returns every $ref value in data.
func refs(data []byte) []string {
	matches := rxRef.FindAllSubmatch(data, -1)
	found := make([]string, 0, len(matches))
	for _, m := range matches {
		found = append(found, string(m[1]))
	}
	return found
}

// call executes the tool by reconstructing the HTTP request from the arguments
// and dispatching it through the API's router, then translating the response
// into an MCP tool result.
func (e *toolEntry) call(ctx context.Context, api huma.API, args map[string]json.RawMessage) *callToolResult {
	req, err := e.buildRequest(ctx, args)
	if err != nil {
		return errorResult("build request: " + err.Error())
	}

	rec := httptest.NewRecorder()
	api.Adapter().ServeHTTP(rec, req)

	// An unauthenticated inner request is an auth challenge rather than a tool
	// failure; let the transport surface it as one where it can.
	recordChallenge(ctx, rec.Code)

	text := rec.Body.String()
	result := &callToolResult{
		Content: []contentBlock{{Type: "text", Text: text}},
		IsError: rec.Code >= 400,
	}

	// On success, include a structured view when the body is a JSON object, so
	// MCP clients that consume structuredContent get typed data. Error bodies
	// are left as text only.
	if !result.IsError {
		var obj map[string]any
		if json.Unmarshal(rec.Body.Bytes(), &obj) == nil {
			result.StructuredContent = obj
		}
	}

	return result
}

// isolated keeps a context's cancellation but drops its values: routers like
// chi reuse routing state found on the context, which would route the inner
// request as the MCP endpoint. Tracing spans and request IDs are dropped too.
type isolated struct{ context.Context }

func (isolated) Value(any) any { return nil }

func (e *toolEntry) buildRequest(ctx context.Context, args map[string]json.RawMessage) (*http.Request, error) {
	path := e.op.Path
	query := url.Values{}
	header := http.Header{}
	var cookies []string
	bodyFields := map[string]json.RawMessage{}
	var wholeBody json.RawMessage

	for _, b := range e.bindings {
		raw, ok := args[b.arg]
		// LLMs send null for "no value", which validation would reject.
		if ok && !b.nullable && string(bytes.TrimSpace(raw)) == "null" {
			ok = false
		}
		if !ok {
			// Left unset, the {name} placeholder would be routed as a literal
			// value and reach the handler.
			if b.loc == locPath {
				return nil, fmt.Errorf("missing required path argument %q", b.arg)
			}
			continue
		}
		switch b.loc {
		case locPath:
			path = strings.ReplaceAll(path, "{"+b.key+"}", url.PathEscape(strings.Join(rawToValues(raw), ",")))
		case locQuery:
			setQuery(query, b, raw)
		case locHeader:
			header.Set(b.key, strings.Join(rawToValues(raw), ","))
		case locCookie:
			// Valid rejects what String would otherwise drop and log.
			cookie := &http.Cookie{Name: b.key, Value: strings.Join(rawToValues(raw), ",")}
			if err := cookie.Valid(); err != nil {
				return nil, fmt.Errorf("cookie argument %q: %w", b.arg, err)
			}
			cookies = append(cookies, cookie.String())
		case locBodyField:
			bodyFields[b.key] = raw
		case locBody:
			wholeBody = raw
		}
	}

	var bodyBytes []byte
	switch {
	case wholeBody != nil:
		bodyBytes = wholeBody
	case len(bodyFields) > 0:
		bodyBytes, _ = json.Marshal(bodyFields)
	case e.op.RequestBody != nil && e.op.RequestBody.Required &&
		slices.ContainsFunc(e.bindings, func(b argBinding) bool { return b.loc == locBodyField }):
		// A required body must be sent even with no fields set.
		bodyBytes = []byte("{}")
	}

	u := path
	if enc := query.Encode(); enc != "" {
		u += "?" + enc
	}

	req, err := http.NewRequestWithContext(isolated{ctx}, e.op.Method, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	if o, ok := ctx.Value(originKey{}).(origin); ok {
		req.Host = o.host
		req.RemoteAddr = o.remoteAddr
		req.TLS = o.tls
	}

	// Forwarded headers (e.g. Authorization) never overlap header arguments,
	// since buildInputSchema leaves those out.
	forwarded, _ := ctx.Value(forwardedHeadersKey{}).(http.Header)
	for k, vs := range forwarded {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if len(cookies) > 0 {
		// Merge cookie params with any forwarded Cookie header.
		if existing := req.Header.Get("Cookie"); existing != "" {
			cookies = append([]string{existing}, cookies...)
		}
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	}
	// Request JSON responses so the result body is deterministic to parse,
	// regardless of any other content types the API also supports.
	req.Header.Set("Accept", "application/json")
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// setQuery adds a query argument in the serialization Huma parses for that
// parameter: deepObject keys for an object, repeated keys for an exploded
// array, and comma-separated values otherwise.
func setQuery(query url.Values, b argBinding, raw json.RawMessage) {
	if b.deepObject {
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) == nil {
			for k, v := range obj {
				query.Set(b.key+"["+k+"]", rawToString(v))
			}
			return
		}
	}

	values := rawToValues(raw)
	if b.explode {
		for _, v := range values {
			query.Add(b.key, v)
		}
		return
	}
	query.Set(b.key, strings.Join(values, ","))
}

// rawToValues converts a JSON argument value to its string elements: one per
// item for an array, otherwise the single value from [rawToString].
func rawToValues(raw json.RawMessage) []string {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return []string{rawToString(raw)}
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, rawToString(item))
	}
	return values
}

// rawToString converts a JSON argument value to the string form expected in a
// URL path/query or header. JSON strings are unquoted; other scalars are used
// verbatim.
func rawToString(raw json.RawMessage) string {
	s := string(bytes.TrimSpace(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			return str
		}
	}
	return s
}

func errorResult(msg string) *callToolResult {
	return &callToolResult{
		Content: []contentBlock{{Type: "text", Text: msg}},
		IsError: true,
	}
}
