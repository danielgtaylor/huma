package huma

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/gin-gonic/gin"
)

// Param describes an OpenAPI 3 parameter
type Param struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	In          string  `json:"in"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`

	// Internal params are excluded from the OpenAPI document and can set up
	// params sent between a load balander / proxy and the service internally.
	internal bool
	def      interface{}
	typ      reflect.Type
}

// PathParam returns a new required path parameter
func PathParam(name string, description string) *Param {
	return &Param{
		Name:        name,
		Description: description,
		In:          "path",
		Required:    true,
	}
}

// QueryParam returns a new optional query string parameter
func QueryParam(name string, description string, defaultValue interface{}) *Param {
	return &Param{
		Name:        name,
		Description: description,
		In:          "query",
		def:         defaultValue,
	}
}

// QueryParamInternal returns a new optional internal query string parameter
func QueryParamInternal(name string, description string, defaultValue interface{}) *Param {
	return &Param{
		Name:        name,
		Description: description,
		In:          "query",
		internal:    true,
		def:         defaultValue,
	}
}

// HeaderParam returns a new optional header parameter
func HeaderParam(name string, description string, defaultValue interface{}) *Param {
	return &Param{
		Name:        name,
		Description: description,
		In:          "header",
		def:         defaultValue,
	}
}

// HeaderParamInternal returns a new optional internal header parameter
func HeaderParamInternal(name string, description string, defaultValue interface{}) *Param {
	return &Param{
		Name:        name,
		Description: description,
		In:          "header",
		internal:    true,
		def:         defaultValue,
	}
}

// Response describes an OpenAPI 3 response
type Response struct {
	Description string
	ContentType string
	StatusCode  int
	Schema      *Schema
	Headers     []string
}

// ResponseEmpty creates a new response with an empty body.
func ResponseEmpty(statusCode int, description string, headers ...string) *Response {
	return &Response{
		Description: description,
		StatusCode:  statusCode,
		Headers:     headers,
	}
}

// ResponseText creates a new string response model.
func ResponseText(statusCode int, description string, headers ...string) *Response {
	return &Response{
		Description: description,
		ContentType: "text/plain",
		StatusCode:  statusCode,
		Headers:     headers,
	}
}

// ResponseJSON creates a new JSON response model.
func ResponseJSON(statusCode int, description string, headers ...string) *Response {
	return &Response{
		Description: description,
		ContentType: "application/json",
		StatusCode:  statusCode,
		Headers:     headers,
	}
}

// ResponseBinary creates a new binary response model.
func ResponseBinary(statusCode int, contentType string, description string, headers ...string) *Response {
	return &Response{
		Description: description,
		ContentType: contentType,
		StatusCode:  statusCode,
		Headers:     headers,
	}
}

// ResponseError creates a new error response model. Alias for ResponseJSON.
func ResponseError(status int, description string, headers ...string) *Response {
	return ResponseJSON(status, description, headers...)
}

// Header describes a response header
type Header struct {
	Name        string  `json:"-"`
	Description string  `json:"description,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

// ResponseHeader returns a new header
func ResponseHeader(name, description string) *Header {
	return &Header{
		Name:        name,
		Description: description,
	}
}

// Operation describes an OpenAPI 3 operation on a path
type Operation struct {
	ID                 string
	Method             string
	Path               string
	Description        string
	Params             []*Param
	RequestContentType string
	RequestSchema      *Schema
	ResponseHeaders    []*Header
	Responses          []*Response
	Handler            interface{}
}

// OpenAPI describes the OpenAPI 3 API
type OpenAPI struct {
	Title   string
	Version string
	// Servers TODO
	Paths map[string][]*Operation
}

// OpenAPIHandler returns a new handler function to generate an OpenAPI spec.
func OpenAPIHandler(api *OpenAPI) func(*gin.Context) {
	respSchema400, _ := GenerateSchema(reflect.ValueOf(ErrorInvalidModel{}).Type())

	return func(c *gin.Context) {
		openapi := gabs.New()
		openapi.Set("3.0.1", "openapi")
		openapi.Set(api.Title, "info", "title")
		openapi.Set(api.Version, "info", "version")

		// spew.Dump(m.paths)

		for path, operations := range api.Paths {
			if strings.Contains(path, ":") {
				// Convert from gin-style params to OpenAPI-style params
				path = paramRe.ReplaceAllString(path, "{$1$2}")
			}

			for _, op := range operations {
				method := strings.ToLower(op.Method)
				openapi.Set(op.ID, "paths", path, method, "operationId")
				openapi.Set(op.Description, "paths", path, method, "description")

				for _, param := range op.Params {
					if param.internal {
						// Skip internal-only parameters.
						continue
					}
					openapi.ArrayAppend(param, "paths", path, method, "parameters")
				}

				if op.RequestSchema != nil {
					ct := op.RequestContentType
					if ct == "" {
						ct = "application/json"
					}
					openapi.Set(op.RequestSchema, "paths", path, method, "requestBody", "content", ct, "schema")
				}

				responses := make([]*Response, 0, len(op.Responses))
				found400 := false
				for _, resp := range op.Responses {
					responses = append(responses, resp)
					if resp.StatusCode == http.StatusBadRequest {
						found400 = true
					}
				}

				if op.RequestSchema != nil && !found400 {
					// Add a 400-level response in case parsing the request fails.
					responses = append(responses, &Response{
						Description: "Invalid input",
						ContentType: "application/json",
						StatusCode:  http.StatusBadRequest,
						Schema:      respSchema400,
					})
				}

				headerMap := map[string]*Header{}
				for _, header := range op.ResponseHeaders {
					headerMap[header.Name] = header
				}

				for _, resp := range op.Responses {
					status := fmt.Sprintf("%v", resp.StatusCode)
					openapi.Set(resp.Description, "paths", path, method, "responses", status, "description")

					for _, name := range resp.Headers {
						header := headerMap[name]
						openapi.Set(header, "paths", path, method, "responses", status, "headers", header.Name)
					}

					if resp.Schema != nil {
						openapi.Set(resp.Schema, "paths", path, method, "responses", status, "content", resp.ContentType, "schema")
					}
				}

			}
		}

		c.Data(200, "application/json; charset=utf-8", openapi.BytesIndent("", "  "))
	}
}
