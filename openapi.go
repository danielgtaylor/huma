package huma

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/Jeffail/gabs"
	"github.com/danielgtaylor/huma/schema"
	"github.com/gin-gonic/gin"
)

// paramLocation describes where in the HTTP request the parameter comes from.
type paramLocation string

// Parameter locations supported by OpenAPI 3
const (
	inPath   paramLocation = "path"
	inQuery  paramLocation = "query"
	inHeader paramLocation = "header"
)

// openAPIParam describes an OpenAPI 3 parameter
type openAPIParam struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	In          paramLocation  `json:"in"`
	Required    bool           `json:"required,omitempty"`
	Schema      *schema.Schema `json:"schema,omitempty"`
	Deprecated  bool           `json:"deprecated,omitempty"`
	Example     interface{}    `json:"example,omitempty"`
	Explode     *bool          `json:"explode,omitempty"`

	// Internal params are excluded from the OpenAPI document and can set up
	// params sent between a load balander / proxy and the service internally.
	Internal bool

	def interface{}
	typ reflect.Type
}

// newOpenAPIParam returns a new parameter instance.
func newOpenAPIParam(name, description string, in paramLocation, options ...ParamOption) *openAPIParam {
	p := &openAPIParam{
		Name:        name,
		Description: description,
		In:          in,
	}

	if in == inQuery {
		p.Explode = new(bool)
	}

	for _, option := range options {
		option.applyParam(p)
	}

	return p
}

// openAPIResponse describes an OpenAPI 3 response
type openAPIResponse struct {
	Description string
	ContentType string
	StatusCode  int
	Schema      *schema.Schema
	Headers     []string
}

// newOpenAPIResponse returns a new response instance.
func newOpenAPIResponse(statusCode int, description string, options ...ResponseOption) *openAPIResponse {
	r := &openAPIResponse{
		StatusCode:  statusCode,
		Description: description,
	}

	for _, option := range options {
		option.applyResponse(r)
	}

	return r
}

// openAPIResponseHeader describes a response header
type openAPIResponseHeader struct {
	Name        string         `json:"-"`
	Description string         `json:"description,omitempty"`
	Schema      *schema.Schema `json:"schema,omitempty"`
}

// openAPISecurityRequirement defines the security schemes and scopes required to use
// an operation.
type openAPISecurityRequirement map[string][]string

// openAPIOperation describes an OpenAPI 3 operation on a path
type openAPIOperation struct {
	*openAPIDependency
	id                 string
	summary            string
	description        string
	tags               []string
	security           []openAPISecurityRequirement
	requestContentType string
	requestSchema      *schema.Schema
	responses          []*openAPIResponse
	extra              map[string]interface{}

	// maxBodyBytes limits the size of the request body that will be read before
	// an error is returned. Defaults to 1MiB if set to zero. Set to -1 for
	// unlimited.
	maxBodyBytes int64

	// bodyReadTimeout sets the duration until reading the body is given up and
	// aborted with an error. Defaults to 15 seconds if the body is automatically
	// read and parsed into a struct, otherwise unset. Set to -1 for unlimited.
	bodyReadTimeout time.Duration
}

// newOperation creates a new operation with the given options applied.
func newOperation(options ...OperationOption) *openAPIOperation {
	op := &openAPIOperation{
		openAPIDependency: &openAPIDependency{
			dependencies:    make([]*openAPIDependency, 0),
			params:          make([]*openAPIParam, 0),
			responseHeaders: make([]*openAPIResponseHeader, 0),
		},
		tags:      make([]string, 0),
		security:  make([]openAPISecurityRequirement, 0),
		responses: make([]*openAPIResponse, 0),
		extra:     make(map[string]interface{}),
	}

	for _, option := range options {
		option.applyOperation(op)
	}

	return op
}

// Copy creates a new shallow copy of the operation. New arrays are created for
// e.g. parameters so they can be safely appended. Existing params are not
// deeply copied and should not be modified.
func (o *openAPIOperation) Copy() *openAPIOperation {
	extraCopy := map[string]interface{}{}

	for k, v := range o.extra {
		extraCopy[k] = v
	}

	newOp := &openAPIOperation{
		openAPIDependency: &openAPIDependency{
			dependencies:    append([]*openAPIDependency{}, o.dependencies...),
			params:          append([]*openAPIParam{}, o.params...),
			responseHeaders: append([]*openAPIResponseHeader{}, o.responseHeaders...),
			handler:         o.handler,
		},
		id:                 o.id,
		summary:            o.summary,
		description:        o.description,
		tags:               append([]string{}, o.tags...),
		security:           append([]openAPISecurityRequirement{}, o.security...),
		requestContentType: o.requestContentType,
		requestSchema:      o.requestSchema,
		responses:          append([]*openAPIResponse{}, o.responses...),
		extra:              extraCopy,
		maxBodyBytes:       o.maxBodyBytes,
		bodyReadTimeout:    o.bodyReadTimeout,
	}

	return newOp
}

// With applies options to the operation. It makes it easy to set up new params,
// responese headers, responses, etc. It always creates a new copy.
func (o *openAPIOperation) With(options ...OperationOption) *openAPIOperation {
	copy := o.Copy()

	for _, option := range options {
		option.applyOperation(copy)
	}

	return copy
}

// allParams returns a list of all the parameters for this operation, including
// those for dependencies.
func (o *openAPIOperation) allParams() []*openAPIParam {
	params := []*openAPIParam{}
	seen := map[*openAPIParam]bool{}

	for _, p := range o.params {
		seen[p] = true
		params = append(params, p)
	}

	for _, d := range o.dependencies {
		for _, p := range d.allParams() {
			if _, ok := seen[p]; !ok {
				seen[p] = true

				params = append(params, p)
			}
		}
	}

	return params
}

// allResponseHeaders returns a list of all the parameters for this operation,
// including those for dependencies.
func (o *openAPIOperation) allResponseHeaders() []*openAPIResponseHeader {
	headers := []*openAPIResponseHeader{}
	seen := map[*openAPIResponseHeader]bool{}

	for _, h := range o.responseHeaders {
		seen[h] = true
		headers = append(headers, h)
	}

	for _, d := range o.dependencies {
		for _, h := range d.allResponseHeaders() {
			if _, ok := seen[h]; !ok {
				seen[h] = true

				headers = append(headers, h)
			}
		}
	}

	return headers
}

// unsafe returns true if the operation's handler was made with UnsafeHandler.
func (o *openAPIOperation) unsafe() bool {
	if _, ok := o.handler.(*unsafeHandler); ok {
		return true
	}

	return false
}

// openAPIServer describes an OpenAPI 3 API server location
type openAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// openAPIContact information for this API.
type openAPIContact struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
}

// openAPIOAuthFlow describes the URLs and scopes to get tokens via a specific flow.
type openAPIOAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl"`
	TokenURL         string            `json:"tokenUrl"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes"`
}

// openAPIOAuthFlows describes the configuration for each flow type.
type openAPIOAuthFlows struct {
	Implicit          *openAPIOAuthFlow `json:"implicit,omitempty"`
	Password          *openAPIOAuthFlow `json:"password,omitempty"`
	ClientCredentials *openAPIOAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *openAPIOAuthFlow `json:"authorizationCode,omitempty"`
}

// openAPISecurityScheme describes the auth mechanism(s) for this API.
type openAPISecurityScheme struct {
	Type             string             `json:"type"`
	Description      string             `json:"description,omitempty"`
	Name             string             `json:"name,omitempty"`
	In               string             `json:"in,omitempty"`
	Scheme           string             `json:"scheme,omitempty"`
	BearerFormat     string             `json:"bearerFormat,omitempty"`
	Flows            *openAPIOAuthFlows `json:"flows,omitempty"`
	OpenIDConnectURL string             `json:"openIdConnectUrl,omitempty"`
}

// openAPI describes the openAPI 3 API
type openAPI struct {
	Title           string
	Version         string
	Description     string
	Contact         *openAPIContact
	Servers         []*openAPIServer
	SecuritySchemes map[string]*openAPISecurityScheme
	Security        []openAPISecurityRequirement
	Paths           map[string]map[string]*openAPIOperation

	// Extra allows setting extra keys in the OpenAPI root structure.
	Extra map[string]interface{}

	// Hook is a function to add to or modify the OpenAPI document before
	// returning it when accessing `GET /openapi.json`.
	Hook func(*gabs.Container)
}

// openAPIHandler returns a new handler function to generate an OpenAPI spec.
func openAPIHandler(api *openAPI) gin.HandlerFunc {
	respSchema400, _ := schema.Generate(reflect.ValueOf(ErrorInvalidModel{}).Type())

	return func(c *gin.Context) {
		openapi := gabs.New()

		for k, v := range api.Extra {
			openapi.Set(v, k)
		}

		openapi.Set("3.0.1", "openapi")
		openapi.Set(api.Title, "info", "title")
		openapi.Set(api.Version, "info", "version")

		if api.Description != "" {
			openapi.Set(api.Description, "info", "description")
		}

		if api.Contact != nil {
			openapi.Set(api.Contact, "info", "contact")
		}

		if len(api.Servers) > 0 {
			openapi.Set(api.Servers, "servers")
		}

		if len(api.SecuritySchemes) > 0 {
			openapi.Set(api.SecuritySchemes, "components", "securitySchemes")
		}

		if len(api.Security) > 0 {
			openapi.Set(api.Security, "security")
		}

		for path, methods := range api.Paths {
			if strings.Contains(path, ":") {
				// Convert from gin-style params to OpenAPI-style params
				path = paramRe.ReplaceAllString(path, "{$1$2}")
			}

			for method, op := range methods {
				method := strings.ToLower(method)

				for k, v := range op.extra {
					openapi.Set(v, "paths", path, method, k)
				}

				openapi.Set(op.id, "paths", path, method, "operationId")
				if op.summary != "" {
					openapi.Set(op.summary, "paths", path, method, "summary")
				}
				openapi.Set(op.description, "paths", path, method, "description")
				if len(op.tags) > 0 {
					openapi.Set(op.tags, "paths", path, method, "tags")
				}

				if len(op.security) > 0 {
					openapi.Set(op.security, "paths", path, method, "security")
				}

				for _, param := range op.allParams() {
					if param.Internal {
						// Skip internal-only parameters.
						continue
					}
					openapi.ArrayAppend(param, "paths", path, method, "parameters")
				}

				if op.requestSchema != nil {
					ct := op.requestContentType
					if ct == "" {
						ct = "application/json"
					}
					openapi.Set(op.requestSchema, "paths", path, method, "requestBody", "content", ct, "schema")
				}

				responses := make([]*openAPIResponse, 0, len(op.responses))
				found400 := false
				for _, resp := range op.responses {
					responses = append(responses, resp)
					if resp.StatusCode == http.StatusBadRequest {
						found400 = true
					}
				}

				if op.requestSchema != nil && !found400 {
					// Add a 400-level response in case parsing the request fails.
					responses = append(responses, &openAPIResponse{
						Description: "Invalid input",
						ContentType: "application/json",
						StatusCode:  http.StatusBadRequest,
						Schema:      respSchema400,
					})
				}

				headerMap := map[string]*openAPIResponseHeader{}
				for _, header := range op.allResponseHeaders() {
					headerMap[header.Name] = header
				}

				for _, resp := range op.responses {
					status := fmt.Sprintf("%v", resp.StatusCode)
					openapi.Set(resp.Description, "paths", path, method, "responses", status, "description")

					headers := make([]string, 0, len(resp.Headers))
					seen := map[string]bool{}
					for _, name := range resp.Headers {
						headers = append(headers, name)
						seen[name] = true
					}
					for _, dep := range op.dependencies {
						for _, header := range dep.allResponseHeaders() {
							if _, ok := seen[header.Name]; !ok {
								headers = append(headers, header.Name)
								seen[header.Name] = true
							}
						}
					}

					for _, name := range headers {
						header := headerMap[name]
						openapi.Set(header, "paths", path, method, "responses", status, "headers", header.Name)
					}

					if resp.Schema != nil {
						openapi.Set(resp.Schema, "paths", path, method, "responses", status, "content", resp.ContentType, "schema")
					}
				}

			}
		}

		if api.Hook != nil {
			api.Hook(openapi)
		}

		c.Data(200, "application/json; charset=utf-8", openapi.BytesIndent("", "  "))
	}
}
