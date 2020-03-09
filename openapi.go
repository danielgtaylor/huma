package huma

import (
	"fmt"
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
	def         interface{}
	typ         reflect.Type
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
	// TODO: support setting default value
	return &Param{
		Name:        name,
		Description: description,
		In:          "query",
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

// Response describes an OpenAPI 3 response
type Response struct {
	Description string
	ContentType string
	StatusCode  int
	Schema      *Schema
}

// ResponseEmpty creates a new response with an empty body.
func ResponseEmpty(statusCode int, description string) *Response {
	return &Response{
		Description: description,
		StatusCode:  statusCode,
	}
}

// ResponseText creates a new string response model.
func ResponseText(statusCode int, description string) *Response {
	return &Response{
		Description: description,
		ContentType: "text/plain",
		StatusCode:  statusCode,
	}
}

// ResponseJSON creates a new JSON response model.
func ResponseJSON(statusCode int, description string) *Response {
	return &Response{
		Description: description,
		ContentType: "application/json",
		StatusCode:  statusCode,
	}
}

// ResponseBinary creates a new binary response model.
func ResponseBinary(statusCode int, contentType string, description string) *Response {
	return &Response{
		Description: description,
		ContentType: contentType,
		StatusCode:  statusCode,
	}
}

// ResponseError creates a new error response model.
func ResponseError(status int, description string) *Response {
	return &Response{
		Description: description,
		ContentType: "application/json",
		StatusCode:  status,
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
	RequestModel       interface{}
	RequestSchema      *Schema
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
					if resp.StatusCode == 400 {
						found400 = true
					}
				}

				if op.RequestSchema != nil && !found400 {
					// Add a 400-level response in case parsing the request fails.
					s, _ := GenerateSchema(reflect.ValueOf(ErrorInvalidModel{}).Type())
					responses = append(responses, &Response{
						Description: "Invalid input",
						ContentType: "application/json",
						StatusCode:  400,
						Schema:      s,
					})
				}

				for _, resp := range op.Responses {
					status := fmt.Sprintf("%v", resp.StatusCode)
					openapi.Set(resp.Description, "paths", path, method, "responses", status, "description")

					if resp.Schema != nil {
						openapi.Set(resp.Schema, "paths", path, method, "responses", status, "content", resp.ContentType, "schema")
					}
				}

			}
		}

		c.Data(200, "application/json; charset=utf-8", openapi.BytesIndent("", "  "))
	}
}
