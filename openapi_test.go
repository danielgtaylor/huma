package huma

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

var paramFuncsTable = []struct {
	n           string
	param       *Param
	name        string
	description string
	in          string
	required    bool
	internal    bool
	def         interface{}
	example     interface{}
}{
	{"PathParam", PathParam("test", "desc"), "test", "desc", "path", true, false, nil, nil},
	{"PathParamSchema", PathParam("test", "desc", &Schema{}), "test", "desc", "path", true, false, nil, nil},
	{"PathParamExample", PathParamExample("test", "desc", 123), "test", "desc", "path", true, false, nil, 123},
	{"QueryParam", QueryParam("test", "desc", "def"), "test", "desc", "query", false, false, "def", nil},
	{"QueryParamSchema", QueryParam("test", "desc", "def", &Schema{}), "test", "desc", "query", false, false, "def", nil},
	{"QueryParamExample", QueryParamExample("test", "desc", "def", "foo"), "test", "desc", "query", false, false, "def", "foo"},
	{"QueryParamInternal", QueryParamInternal("test", "desc", "def"), "test", "desc", "query", false, true, "def", nil},
	{"HeaderParam", HeaderParam("test", "desc", "def"), "test", "desc", "header", false, false, "def", nil},
	{"HeaderParamSchema", HeaderParam("test", "desc", "def", &Schema{}), "test", "desc", "header", false, false, "def", nil},
	{"HeaderParamExample", HeaderParamExample("test", "desc", "def", "foo"), "test", "desc", "header", false, false, "def", "foo"},
	{"HeaderParamInternal", HeaderParamInternal("test", "desc", "def"), "test", "desc", "header", false, true, "def", nil},
}

func TestParamFuncs(outer *testing.T) {
	outer.Parallel()
	for _, tt := range paramFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			t.Parallel()
			param := local.param
			assert.Equal(t, local.name, param.Name)
			assert.Equal(t, local.description, param.Description)
			assert.Equal(t, local.in, param.In)
			assert.Equal(t, local.required, param.Required)
			assert.Equal(t, local.internal, param.internal)
			assert.Equal(t, local.def, param.def)
			assert.Equal(t, local.example, param.Example)
		})
	}
}

var responseFuncsTable = []struct {
	n           string
	resp        *Response
	statusCode  int
	description string
	headers     []string
	contentType string
}{
	{"ResponseEmpty", ResponseEmpty(204, "desc", "head1", "head2"), 204, "desc", []string{"head1", "head2"}, ""},
	{"ResponseText", ResponseText(200, "desc", "head1", "head2"), 200, "desc", []string{"head1", "head2"}, "application/json"},
	{"ResponseJSON", ResponseJSON(200, "desc", "head1", "head2"), 200, "desc", []string{"head1", "head2"}, "application/json"},
	{"ResponseError", ResponseJSON(200, "desc", "head1", "head2"), 200, "desc", []string{"head1", "head2"}, "application/json"},
}

func TestResponseFuncs(outer *testing.T) {
	outer.Parallel()
	for _, tt := range responseFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			t.Parallel()
			resp := local.resp
			assert.Equal(t, local.statusCode, resp.StatusCode)
			assert.Equal(t, local.description, resp.Description)
			assert.Equal(t, local.headers, resp.Headers)
		})
	}
}

var serverFuncsTable = []struct {
	n           string
	server      *Server
	url         string
	description string
}{
	{"DevServer", DevServer("url"), "url", "Development server"},
	{"ProdServer", ProdServer("url"), "url", "Production server"},
}

func TestServerFuncs(outer *testing.T) {
	outer.Parallel()
	for _, tt := range serverFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			t.Parallel()
			server := local.server
			assert.Equal(t, local.url, server.URL)
			assert.Equal(t, local.description, server.Description)
		})
	}
}

var securityFuncsTable = []struct {
	n            string
	sec          *SecurityScheme
	typ          string
	name         string
	in           string
	scheme       string
	bearerFormat string
}{
	{"BasicAuth", BasicAuth(), "http", "", "", "basic", ""},
	{"APIKeyAuth", APIKeyAuth("name", "in"), "apiKey", "name", "in", "", ""},
	{"JWTBearerAuth", JWTBearerAuth(), "http", "", "", "bearer", "JWT"},
}

func TestSecurityFuncs(outer *testing.T) {
	outer.Parallel()
	for _, tt := range securityFuncsTable {
		local := tt
		outer.Run(fmt.Sprintf("%v", tt.n), func(t *testing.T) {
			t.Parallel()
			sec := local.sec
			assert.Equal(t, local.typ, sec.Type)
			assert.Equal(t, local.name, sec.Name)
			assert.Equal(t, local.in, sec.In)
			assert.Equal(t, local.scheme, sec.Scheme)
			assert.Equal(t, local.bearerFormat, sec.BearerFormat)
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

	g := gin.New()
	g.Use(gin.Recovery())
	r := NewRouterWithGin(g, &OpenAPI{
		Title:       "OpenAPI Test",
		Description: "My test API...",
		Version:     "1.0.0",
		Contact: &Contact{
			Name:  "Support",
			Email: "support@example.com",
		},
		Servers: []*Server{
			DevServer("http://localhost:8888"),
		},
		SecuritySchemes: map[string]*SecurityScheme{
			"basic": BasicAuth(),
		},
		Extra: map[string]interface{}{
			"x-foo": "bar",
		},
	})

	r.Register(http.MethodPut, "/hello", &Operation{
		ID:          "put-hello",
		Summary:     "Summary message",
		Description: "Get a welcome message",
		Tags:        []string{"Messages"},
		Security:    SecurityRef("basic"),
		Params: []*Param{
			QueryParam("greet", "Whether to greet or not", false),
			HeaderParamInternal("user", "User from auth token", ""),
		},
		ResponseHeaders: []*ResponseHeader{
			Header("etag", "Content hash for caching"),
		},
		Responses: []*Response{
			ResponseJSON(200, "Successful response", "etag"),
		},
		Extra: map[string]interface{}{
			"x-foo": "bar",
		},
		Handler: func(greet bool, body *HelloRequest) (string, *HelloResponse) {
			return "etag", &HelloResponse{
				Message: "Hello",
			}
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/openapi.json", nil)
	r.ServeHTTP(w, req)

	// Confirm it loads without errors.
	data := w.Body.Bytes()
	_, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(data)
	assert.NoError(t, err, string(data))
}
