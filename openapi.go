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

// ParamLocation describes where in the HTTP request the parameter comes from.
type ParamLocation string

// Parameter locations supported by OpenAPI 3
const (
	InPath   ParamLocation = "path"
	InQuery  ParamLocation = "query"
	InHeader ParamLocation = "header"
)

// OpenAPIParam describes an OpenAPI 3 parameter
type OpenAPIParam struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	In          ParamLocation  `json:"in"`
	Required    bool           `json:"required,omitempty"`
	Schema      *schema.Schema `json:"schema,omitempty"`
	Deprecated  bool           `json:"deprecated,omitempty"`
	Example     interface{}    `json:"example,omitempty"`

	// Internal params are excluded from the OpenAPI document and can set up
	// params sent between a load balander / proxy and the service internally.
	Internal bool

	def interface{}
	typ reflect.Type
}

// NewOpenAPIParam returns a new parameter instance.
func NewOpenAPIParam(name, description string, in ParamLocation, options ...ParamOption) *OpenAPIParam {
	p := &OpenAPIParam{
		Name:        name,
		Description: description,
		In:          in,
	}

	for _, option := range options {
		option.ApplyParam(p)
	}

	return p
}

// OpenAPIResponse describes an OpenAPI 3 response
type OpenAPIResponse struct {
	Description string
	ContentType string
	StatusCode  int
	Schema      *schema.Schema
	Headers     []string
}

// NewOpenAPIResponse returns a new response instance.
func NewOpenAPIResponse(statusCode int, description string, options ...ResponseOption) *OpenAPIResponse {
	r := &OpenAPIResponse{
		StatusCode:  statusCode,
		Description: description,
	}

	for _, option := range options {
		option.ApplyResponse(r)
	}

	return r
}

// OpenAPIResponseHeader describes a response header
type OpenAPIResponseHeader struct {
	Name        string         `json:"-"`
	Description string         `json:"description,omitempty"`
	Schema      *schema.Schema `json:"schema,omitempty"`
}

// OpenAPISecurityRequirement defines the security schemes and scopes required to use
// an operation.
type OpenAPISecurityRequirement map[string][]string

// OpenAPIOperation describes an OpenAPI 3 operation on a path
type OpenAPIOperation struct {
	*OpenAPIDependency
	id                 string
	summary            string
	description        string
	tags               []string
	security           []OpenAPISecurityRequirement
	requestContentType string
	requestSchema      *schema.Schema
	responses          []*OpenAPIResponse
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

// ID returns the unique identifier for this operation. If not set manually,
// it is generated from the path and HTTP method.
func (o *OpenAPIOperation) ID() string {
	return o.id
}

// NewOperation creates a new operation with the given options applied.
func NewOperation(options ...OperationOption) *OpenAPIOperation {
	op := &OpenAPIOperation{
		OpenAPIDependency: &OpenAPIDependency{
			dependencies:    make([]*OpenAPIDependency, 0),
			params:          make([]*OpenAPIParam, 0),
			responseHeaders: make([]*OpenAPIResponseHeader, 0),
		},
		tags:      make([]string, 0),
		security:  make([]OpenAPISecurityRequirement, 0),
		responses: make([]*OpenAPIResponse, 0),
		extra:     make(map[string]interface{}),
	}

	for _, option := range options {
		option.ApplyOperation(op)
	}

	return op
}

// Copy creates a new shallow copy of the operation. New arrays are created for
// e.g. parameters so they can be safely appended. Existing params are not
// deeply copied and should not be modified.
func (o *OpenAPIOperation) Copy() *OpenAPIOperation {
	extraCopy := map[string]interface{}{}

	for k, v := range o.extra {
		extraCopy[k] = v
	}

	newOp := &OpenAPIOperation{
		OpenAPIDependency: &OpenAPIDependency{
			dependencies:    append([]*OpenAPIDependency{}, o.dependencies...),
			params:          append([]*OpenAPIParam{}, o.params...),
			responseHeaders: append([]*OpenAPIResponseHeader{}, o.responseHeaders...),
			handler:         o.handler,
		},
		id:                 o.id,
		summary:            o.summary,
		description:        o.description,
		tags:               append([]string{}, o.tags...),
		security:           append([]OpenAPISecurityRequirement{}, o.security...),
		requestContentType: o.requestContentType,
		requestSchema:      o.requestSchema,
		responses:          append([]*OpenAPIResponse{}, o.responses...),
		extra:              extraCopy,
		maxBodyBytes:       o.maxBodyBytes,
		bodyReadTimeout:    o.bodyReadTimeout,
	}

	return newOp
}

// With applies options to the operation. It makes it easy to set up new params,
// responese headers, responses, etc. It always creates a new copy.
func (o *OpenAPIOperation) With(options ...OperationOption) *OpenAPIOperation {
	copy := o.Copy()

	for _, option := range options {
		option.ApplyOperation(copy)
	}

	return copy
}

// allParams returns a list of all the parameters for this operation, including
// those for dependencies.
func (o *OpenAPIOperation) allParams() []*OpenAPIParam {
	params := []*OpenAPIParam{}
	seen := map[*OpenAPIParam]bool{}

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
func (o *OpenAPIOperation) allResponseHeaders() []*OpenAPIResponseHeader {
	headers := []*OpenAPIResponseHeader{}
	seen := map[*OpenAPIResponseHeader]bool{}

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

// OpenAPIServer describes an OpenAPI 3 API server location
type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// OpenAPIContact information for this API.
type OpenAPIContact struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Email string `json:"email"`
}

// OpenAPIOAuthFlow describes the URLs and scopes to get tokens via a specific flow.
type OpenAPIOAuthFlow struct {
	AuthorizationURL string            `json:"authorizationUrl"`
	TokenURL         string            `json:"tokenUrl"`
	RefreshURL       string            `json:"refreshUrl,omitempty"`
	Scopes           map[string]string `json:"scopes"`
}

// OpenAPIOAuthFlows describes the configuration for each flow type.
type OpenAPIOAuthFlows struct {
	Implicit          *OpenAPIOAuthFlow `json:"implicit,omitempty"`
	Password          *OpenAPIOAuthFlow `json:"password,omitempty"`
	ClientCredentials *OpenAPIOAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *OpenAPIOAuthFlow `json:"authorizationCode,omitempty"`
}

// OpenAPISecurityScheme describes the auth mechanism(s) for this API.
type OpenAPISecurityScheme struct {
	Type             string             `json:"type"`
	Description      string             `json:"description,omitempty"`
	Name             string             `json:"name,omitempty"`
	In               string             `json:"in,omitempty"`
	Scheme           string             `json:"scheme,omitempty"`
	BearerFormat     string             `json:"bearerFormat,omitempty"`
	Flows            *OpenAPIOAuthFlows `json:"flows,omitempty"`
	OpenIDConnectURL string             `json:"openIdConnectUrl,omitempty"`
}

// OpenAPI describes the OpenAPI 3 API
type OpenAPI struct {
	Title           string
	Version         string
	Description     string
	Contact         *OpenAPIContact
	Servers         []*OpenAPIServer
	SecuritySchemes map[string]*OpenAPISecurityScheme
	Security        []OpenAPISecurityRequirement
	Paths           map[string]map[string]*OpenAPIOperation

	// Extra allows setting extra keys in the OpenAPI root structure.
	Extra map[string]interface{}

	// Hook is a function to add to or modify the OpenAPI document before
	// returning it when accessing `GET /openapi.json`.
	Hook func(*gabs.Container)
}

// openAPIHandler returns a new handler function to generate an OpenAPI spec.
func openAPIHandler(api *OpenAPI) gin.HandlerFunc {
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

				responses := make([]*OpenAPIResponse, 0, len(op.responses))
				found400 := false
				for _, resp := range op.responses {
					responses = append(responses, resp)
					if resp.StatusCode == http.StatusBadRequest {
						found400 = true
					}
				}

				if op.requestSchema != nil && !found400 {
					// Add a 400-level response in case parsing the request fails.
					responses = append(responses, &OpenAPIResponse{
						Description: "Invalid input",
						ContentType: "application/json",
						StatusCode:  http.StatusBadRequest,
						Schema:      respSchema400,
					})
				}

				headerMap := map[string]*OpenAPIResponseHeader{}
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
