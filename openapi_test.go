package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/istreamlabs/huma/schema"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

var paramFuncsTable = []struct {
	n           string
	param       OperationOption
	name        string
	description string
	in          paramLocation
	required    bool
	internal    bool
	def         interface{}
	example     interface{}
}{
	{"PathParam", PathParam("test", "desc"), "test", "desc", inPath, true, false, nil, nil},
	{"PathParamSchema", PathParam("test", "desc", Schema(schema.Schema{})), "test", "desc", inPath, true, false, nil, nil},
	{"PathParamExample", PathParam("test", "desc", Example(123)), "test", "desc", inPath, true, false, nil, 123},
	{"QueryParam", QueryParam("test", "desc", "def"), "test", "desc", inQuery, false, false, "def", nil},
	{"QueryParamSchema", QueryParam("test", "desc", "def", Schema(schema.Schema{})), "test", "desc", inQuery, false, false, "def", nil},
	{"QueryParamExample", QueryParam("test", "desc", "def", Example("foo")), "test", "desc", inQuery, false, false, "def", "foo"},
	{"QueryParamInternal", QueryParam("test", "desc", "def", Internal()), "test", "desc", inQuery, false, true, "def", nil},
	{"HeaderParam", HeaderParam("test", "desc", "def"), "test", "desc", inHeader, false, false, "def", nil},
	{"HeaderParamSchema", HeaderParam("test", "desc", "def", Schema(schema.Schema{})), "test", "desc", inHeader, false, false, "def", nil},
	{"HeaderParamExample", HeaderParam("test", "desc", "def", Example("foo")), "test", "desc", inHeader, false, false, "def", "foo"},
	{"HeaderParamInternal", HeaderParam("test", "desc", "def", Internal()), "test", "desc", inHeader, false, true, "def", nil},
}

func TestParamFuncs(outer *testing.T) {
	for _, tt := range paramFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			op := newOperation()
			local.param.applyOperation(op)
			param := op.params[0]
			assert.Equal(t, local.name, param.Name)
			assert.Equal(t, local.description, param.Description)
			assert.Equal(t, local.in, param.In)
			assert.Equal(t, local.required, param.Required)
			assert.Equal(t, local.internal, param.Internal)
			assert.Equal(t, local.def, param.def)
			assert.Equal(t, local.example, param.Example)
		})
	}
}

var responseFuncsTable = []struct {
	n           string
	resp        OperationOption
	statusCode  int
	description string
	headers     []string
	contentType string
}{
	{"ResponseEmpty", Response(204, "desc", Headers("head1", "head2")), 204, "desc", []string{"head1", "head2"}, ""},
	{"ResponseText", ResponseText(200, "desc", Headers("head1", "head2")), 200, "desc", []string{"head1", "head2"}, "application/json"},
	{"ResponseJSON", ResponseJSON(200, "desc", Headers("head1", "head2")), 200, "desc", []string{"head1", "head2"}, "application/json"},
	{"ResponseError", ResponseJSON(200, "desc", Headers("head1", "head2")), 200, "desc", []string{"head1", "head2"}, "application/json"},
}

func TestResponseFuncs(outer *testing.T) {
	for _, tt := range responseFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			op := newOperation()
			local.resp.applyOperation(op)
			resp := op.responses[0]
			assert.Equal(t, local.statusCode, resp.StatusCode)
			assert.Equal(t, local.description, resp.Description)
			assert.Equal(t, local.headers, resp.Headers)
		})
	}
}

var serverFuncsTable = []struct {
	n           string
	option      RouterOption
	url         string
	description string
}{
	{"DevServer", DevServer("url"), "url", "Development server"},
	{"ProdServer", ProdServer("url"), "url", "Production server"},
}

func TestServerFuncs(outer *testing.T) {
	for _, tt := range serverFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			r := NewTestRouter(t, local.option)
			assert.Equal(t, local.url, r.api.Servers[0].URL)
			assert.Equal(t, local.description, r.api.Servers[0].Description)
		})
	}
}

var securityFuncsTable = []struct {
	n            string
	option       RouterOption
	typ          string
	name         string
	in           string
	scheme       string
	bearerFormat string
}{
	{"BasicAuth", BasicAuth("test"), "http", "", "", "basic", ""},
	{"APIKeyAuth", APIKeyAuth("test", "name", "in"), "apiKey", "name", "in", "", ""},
	{"JWTBearerAuth", JWTBearerAuth("test"), "http", "", "", "bearer", "JWT"},
}

func TestSecurityFuncs(outer *testing.T) {
	for _, tt := range securityFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			r := NewTestRouter(t, local.option)
			assert.Equal(t, local.typ, r.api.SecuritySchemes["test"].Type)
			assert.Equal(t, local.name, r.api.SecuritySchemes["test"].Name)
			assert.Equal(t, local.in, r.api.SecuritySchemes["test"].In)
			assert.Equal(t, local.scheme, r.api.SecuritySchemes["test"].Scheme)
			assert.Equal(t, local.bearerFormat, r.api.SecuritySchemes["test"].BearerFormat)
		})
	}
}

func TestOpenAPIHandler(t *testing.T) {
	type HelloRequest struct {
		Name string `json:"name" example:"world"`
	}

	type HelloResponse struct {
		Message string `json:"message" example:"Hello, world"`
	}

	r := NewTestRouter(t,
		ContactEmail("Support", "support@example.com"),
		DevServer("http://localhost:8888"),
		BasicAuth("basic"),
		Extra("x-foo", "bar"),
	)

	dep1 := Dependency(DependencyOptions(
		QueryParam("q", "Test query param", ""),
		ResponseHeader("dep", "description"),
	), func(q string) (string, string, error) {
		return "header", "foo", nil
	})

	dep2 := Dependency(dep1, func(q string) (string, error) {
		return q, nil
	})

	r.Resource("/hello",
		dep2,
		SecurityRef("basic"),
		QueryParam("greet", "Whether to greet or not", false),
		HeaderParam("user", "User from auth token", "", Internal()),
		ResponseHeader("etag", "Content hash for caching"),
		ResponseJSON(200, "Successful response", Headers("etag")),
		Extra("x-foo", "bar"),
	).Put("Get a welcome message", func(q string, greet bool, user string, body *HelloRequest) (string, *HelloResponse) {
		return "etag", &HelloResponse{
			Message: "Hello",
		}
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/openapi.json", nil)
	r.ServeHTTP(w, req)

	// Confirm it loads without errors.
	data := w.Body.Bytes()
	_, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(data)
	assert.NoError(t, err, string(data))
}
