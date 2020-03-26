package huma

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPathParam(t *testing.T) {
	p := PathParam("test", "desc")
	assert.Equal(t, "test", p.Name)
	assert.Equal(t, "desc", p.Description)
	assert.Equal(t, "path", p.In)
	assert.Equal(t, true, p.Required)
}

func TestQueryParam(t *testing.T) {
	p := QueryParam("test", "desc", "default")
	assert.Equal(t, "test", p.Name)
	assert.Equal(t, "desc", p.Description)
	assert.Equal(t, "query", p.In)
	assert.Equal(t, false, p.Required)
	assert.Equal(t, "default", p.def)
}

func TestHeaderParam(t *testing.T) {
	p := HeaderParam("test", "desc", "default")
	assert.Equal(t, "test", p.Name)
	assert.Equal(t, "desc", p.Description)
	assert.Equal(t, "header", p.In)
	assert.Equal(t, false, p.Required)
	assert.Equal(t, "default", p.def)
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

	r.Register(&Operation{
		ID:          "put-hello",
		Method:      http.MethodPut,
		Path:        "/hello",
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
