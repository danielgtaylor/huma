package huma_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIMarshal(t *testing.T) {
	// Simple spot check to make sure we are generating valid YAML and that
	// the OpenAPI generally works as expected.

	num := 1.0

	v := huma.OpenAPI{
		OpenAPI: "3.0.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
			Contact: &huma.Contact{
				Name: "Daniel Taylor",
			},
			License: &huma.License{
				Name: "MIT",
			},
		},
		ExternalDocs: &huma.ExternalDocs{
			URL: "https://example.com",
		},
		Tags: []*huma.Tag{
			{
				Name: "test",
			},
		},
		Servers: []*huma.Server{
			{
				URL: "https://example.com/{foo}",
				Variables: map[string]*huma.ServerVariable{
					"foo": {
						Default: "bar",
						Enum:    []string{"bar", "baz"},
					},
				},
			},
		},
		Components: &huma.Components{
			Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer),
			SecuritySchemes: map[string]*huma.SecurityScheme{
				"oauth2": {
					Type: "oauth2",
					Flows: &huma.OAuthFlows{
						ClientCredentials: &huma.OAuthFlow{
							AuthorizationURL: "https://example.com/oauth2/authorize",
							TokenURL:         "https://example.com/oauth2/token",
							Scopes: map[string]string{
								"test": "Test scope",
							},
						},
						Implicit: &huma.OAuthFlow{
							AuthorizationURL: "https://example.com/oauth2/authorize",
							Scopes:           map[string]string{},
						},
					},
				},
			},
		},
		Paths: map[string]*huma.PathItem{
			"/test": {
				Get: &huma.Operation{
					Responses: map[string]*huma.Response{
						"200": {
							Description: "OK",
							Content: map[string]*huma.MediaType{
								"application/json": {
									Examples: map[string]*huma.Example{
										"test": {
											Value: `{"test": "example"}`,
										},
									},
									Encoding: map[string]*huma.Encoding{
										"test": {
											ContentType: "application/json",
										},
									},
									Schema: &huma.Schema{
										Type: "object",
										Properties: map[string]*huma.Schema{
											"test": {
												Type:    "integer",
												Minimum: &num,
											},
										},
									},
								},
							},
							Links: map[string]*huma.Link{
								"related": {
									OperationID: "another-operation",
								},
							},
						},
					},
				},
			},
		},
		Extensions: map[string]any{
			"x-test": 123,
		},
	}

	// This will marshal to JSON, then convert to YAML.
	out, err := v.YAML()
	require.NoError(t, err)

	expected := `components:
  schemas: {}
  securitySchemes:
    oauth2:
      flows:
        clientCredentials:
          authorizationUrl: https://example.com/oauth2/authorize
          scopes:
            test: Test scope
          tokenUrl: https://example.com/oauth2/token
        implicit:
          authorizationUrl: https://example.com/oauth2/authorize
          scopes: {}
      type: oauth2
externalDocs:
  url: https://example.com
info:
  contact:
    name: Daniel Taylor
  license:
    name: MIT
  title: Test API
  version: 1.0.0
openapi: 3.0.0
paths:
  /test:
    get:
      responses:
        "200":
          content:
            application/json:
              encoding:
                test:
                  contentType: application/json
              examples:
                test:
                  value: "{\"test\": \"example\"}"
              schema:
                properties:
                  test:
                    minimum: 1
                    type: integer
                type: object
          description: OK
          links:
            related:
              operationId: another-operation
servers:
  - url: https://example.com/{foo}
    variables:
      foo:
        default: bar
        enum:
          - bar
          - baz
tags:
  - name: test
x-test: 123
`

	require.Equal(t, expected, string(out))
}

func TestDowngrade(t *testing.T) {
	// Test that we can downgrade a v3 OpenAPI document to v2.
	v31 := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: map[string]*huma.PathItem{
			"/test": {
				Get: &huma.Operation{
					Responses: map[string]*huma.Response{
						"200": {
							Description: "OK",
							Content: map[string]*huma.MediaType{
								"application/json": {
									Schema: &huma.Schema{
										Type: "object",
										Properties: map[string]*huma.Schema{
											"test": {
												Type:             "integer",
												ExclusiveMinimum: new(0.0),
												ExclusiveMaximum: new(100.0),
												Nullable:         true,
												Examples:         []any{100},
											},
											"encoding": {
												Type:            huma.TypeString,
												ContentEncoding: "base64",
											},
										},
									},
								},
								"application/octet-stream": {},
							},
						},
					},
				},
			},
		},
	}

	v30, err := v31.Downgrade()
	require.NoError(t, err)

	expected := `{
		"openapi": "3.0.3",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"paths": {
			"/test": {
				"get": {
					"responses": {
						"200": {
							"description": "OK",
							"content": {
								"application/json": {
									"schema": {
										"type": "object",
										"properties": {
											"test": {
												"type": "integer",
												"nullable": true,
												"minimum": 0,
												"exclusiveMinimum": true,
												"maximum": 100,
												"exclusiveMaximum": true,
												"example": 100
											},
											"encoding": {
												"type": "string",
												"format": "base64"
											}
										}
									}
								},
								"application/octet-stream": {
									"schema": {
										"type": "string",
										"format": "binary"
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	// Check that the downgrade worked as expected.
	assert.JSONEq(t, expected, string(v30))
}

func TestAddOperationForceUniqueOperationIDs(t *testing.T) {
	oapi := &huma.OpenAPI{}
	oapi.AddOperation(&huma.Operation{
		OperationID: "test",
		Method:      http.MethodGet,
		Path:        "/test",
	})

	assert.PanicsWithValue(t, "duplicate operation ID: test", func() {
		oapi.AddOperation(&huma.Operation{
			OperationID: "test",
			Method:      http.MethodPost,
			Path:        "/test",
		})
	})
}

func TestAddOperationNormalizeOperationIDs(t *testing.T) {
	oapi := &huma.OpenAPI{}
	oapi.AddOperation(&huma.Operation{
		OperationID: "test with spaces",
		Method:      http.MethodGet,
		Path:        "/test",
	})

	assert.Equal(t, "test-with-spaces", oapi.Paths["/test"].Get.OperationID)
}

// TestHiddenOperationSchemasOmitted verifies that schemas which are only used by
// `Hidden` operations are pruned from the exported OpenAPI document, while
// schemas reachable from visible operations (including transitively-referenced
// and shared types) are kept. The underlying registry is left intact so request
// validation for hidden routes continues to work.
func TestHiddenOperationSchemasOmitted(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	// Shared is used by both a visible and a hidden operation, so it must be
	// kept. Nested is only referenced transitively by the visible response.
	type Shared struct {
		Value string `json:"value"`
	}
	type Nested struct {
		Count int `json:"count"`
	}
	type VisibleResponse struct {
		Shared Shared `json:"shared"`
		Nested Nested `json:"nested"`
	}
	type VisibleResp struct {
		Body VisibleResponse
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-visible",
		Method:      http.MethodGet,
		Path:        "/visible",
	}, func(ctx context.Context, _ *struct{}) (*VisibleResp, error) {
		return &VisibleResp{}, nil
	})

	// SecretAdmin is used only by the hidden operation and must be omitted from
	// the exported document.
	type SecretAdmin struct {
		Token  string `json:"token" minLength:"5"`
		Shared Shared `json:"shared"`
	}
	type HiddenInput struct {
		Body SecretAdmin
	}

	huma.Register(api, huma.Operation{
		OperationID: "admin-only",
		Method:      http.MethodPost,
		Path:        "/admin",
		Hidden:      true,
	}, func(ctx context.Context, _ *HiddenInput) (*struct{}, error) {
		return nil, nil
	})

	b, err := api.OpenAPI().YAML()
	require.NoError(t, err)
	spec := string(b)

	// The hidden operation's path is not documented...
	assert.NotContains(t, spec, "/admin")
	// ...and neither is the schema used only by it.
	assert.NotContains(t, spec, "SecretAdmin")

	// Schemas reachable from the visible operation remain, including
	// transitively-referenced and shared types.
	assert.Contains(t, spec, "VisibleResponse")
	assert.Contains(t, spec, "Nested")
	assert.Contains(t, spec, "Shared")

	// The underlying registry is untouched, so validation for the hidden route
	// still resolves its schema: an invalid body is rejected...
	resp := api.Post("/admin", map[string]any{"token": "x", "shared": map[string]any{"value": "y"}})
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

	// ...and a valid body is accepted.
	resp = api.Post("/admin", map[string]any{"token": "longenough", "shared": map[string]any{"value": "y"}})
	assert.Less(t, resp.Code, 300)
}

// fullOpenAPIDoc exercises every OpenAPI object that can be decoded.
const fullOpenAPIDoc = `{
	"openapi": "3.1.0",
	"info": {
		"title": "Test API",
		"description": "Desc",
		"termsOfService": "https://example.com/tos",
		"contact": {"name": "Me", "url": "https://example.com", "email": "me@example.com", "x-contact": 1},
		"license": {"name": "MIT", "identifier": "MIT", "url": "https://example.com/license"},
		"version": "1.0.0"
	},
	"jsonSchemaDialect": "https://spec.example.com/dialect",
	"servers": [{
		"url": "https://example.com/{foo}",
		"description": "Main",
		"variables": {"foo": {"enum": ["bar", "baz"], "default": "bar", "description": "Foo"}}
	}],
	"paths": {
		"/things/{id}": {
			"summary": "Things",
			"description": "A thing",
			"parameters": [{"$ref": "#/components/parameters/Id"}],
			"servers": [{"url": "https://things.example.com"}],
			"get": {
				"tags": ["things"],
				"summary": "Get a thing",
				"description": "Gets a thing",
				"externalDocs": {"description": "More", "url": "https://example.com/docs"},
				"operationId": "get-thing",
				"parameters": [{
					"name": "verbose",
					"in": "query",
					"description": "Verbose",
					"required": true,
					"deprecated": true,
					"allowEmptyValue": true,
					"style": "form",
					"explode": false,
					"allowReserved": true,
					"schema": {"type": ["boolean", "null"]},
					"example": true,
					"examples": {"yes": {"summary": "Yes", "value": true}}
				}],
				"responses": {
					"200": {
						"description": "OK",
						"headers": {"X-Rate": {"description": "Rate", "schema": {"type": "integer"}}},
						"content": {"application/json": {
							"schema": {"$ref": "#/components/schemas/Thing"},
							"example": {"id": "a"},
							"examples": {"one": {"externalValue": "https://example.com/one.json"}},
							"encoding": {"id": {"contentType": "text/plain", "style": "form", "explode": true, "allowReserved": true}}
						}},
						"links": {"self": {
							"operationId": "get-thing",
							"parameters": {"id": "$response.body#/id"},
							"requestBody": "$response.body",
							"description": "Self",
							"server": {"url": "https://example.com"}
						}}
					},
					"default": {"$ref": "#/components/responses/Error"}
				},
				"deprecated": true,
				"security": [{"oauth2": ["read"]}, {}],
				"servers": [{"url": "https://get.example.com"}],
				"callbacks": {"onEvent": {"{$request.body#/url}": {"post": {
					"requestBody": {"content": {"application/json": {"schema": {"type": "string"}}}},
					"responses": {"204": {"description": "No content"}}
				}}}},
				"x-op": "ext"
			},
			"put": {"operationId": "put-thing"},
			"post": {"operationId": "post-thing"},
			"delete": {"operationId": "delete-thing"},
			"options": {"operationId": "options-thing"},
			"head": {"operationId": "head-thing"},
			"patch": {"operationId": "patch-thing", "requestBody": {
				"description": "Patch",
				"required": true,
				"content": {"application/merge-patch+json": {"schema": {"type": "object"}}}
			}},
			"trace": {"operationId": "trace-thing"},
			"x-path": [1, 2]
		}
	},
	"webhooks": {"hook": {"post": {"operationId": "hook-post"}}},
	"components": {
		"schemas": {
			"Thing": {
				"type": "object",
				"properties": {
					"id": {"type": "string", "minLength": 1},
					"parent": {"$ref": "#/components/schemas/Thing"}
				},
				"required": ["id"],
				"additionalProperties": false
			}
		},
		"responses": {"Error": {"description": "Error"}},
		"parameters": {"Id": {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}},
		"examples": {"E": {"value": 1}},
		"requestBodies": {"B": {"content": {"text/plain": {"schema": {"type": "string"}}}}},
		"headers": {"H": {"description": "A header", "schema": {"type": "string"}}},
		"securitySchemes": {
			"oauth2": {
				"type": "oauth2",
				"description": "OAuth",
				"flows": {
					"implicit": {"authorizationUrl": "https://example.com/auth", "tokenUrl": "", "scopes": {"read": "Read"}},
					"password": {"tokenUrl": "https://example.com/token", "scopes": {}},
					"clientCredentials": {"tokenUrl": "https://example.com/token", "refreshUrl": "https://example.com/refresh", "scopes": {}},
					"authorizationCode": {"authorizationUrl": "https://example.com/auth", "tokenUrl": "https://example.com/token", "scopes": {}, "x-flow": true}
				}
			},
			"key": {"type": "apiKey", "name": "api_key", "in": "header"},
			"bearer": {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
			"oidc": {"type": "openIdConnect", "openIdConnectUrl": "https://example.com/.well-known/openid-configuration"}
		},
		"links": {"L": {"operationRef": "#/paths/~1things~1{id}/get"}},
		"callbacks": {"C": {"post": {"operationId": "callback-post"}}},
		"pathItems": {"P": {"get": {"operationId": "pathitem-get"}}},
		"x-components": "ext"
	},
	"security": [{"oauth2": ["read", "write"]}],
	"tags": [{"name": "things", "description": "Things", "externalDocs": {"url": "https://example.com/things"}, "x-tag": 1}],
	"externalDocs": {"description": "Docs", "url": "https://example.com"},
	"x-root": {"nested": true}
}`

func TestOpenAPIUnmarshal(t *testing.T) {
	var o huma.OpenAPI
	require.NoError(t, json.Unmarshal([]byte(fullOpenAPIDoc), &o))

	assert.Equal(t, "3.1.0", o.OpenAPI)
	assert.Equal(t, "Test API", o.Info.Title)
	assert.Equal(t, "me@example.com", o.Info.Contact.Email)
	assert.Equal(t, float64(1), o.Info.Contact.Extensions["x-contact"])
	assert.Equal(t, "MIT", o.Info.License.Identifier)
	assert.Equal(t, "https://spec.example.com/dialect", o.JSONSchemaDialect)
	assert.Equal(t, []string{"bar", "baz"}, o.Servers[0].Variables["foo"].Enum)
	assert.Equal(t, map[string]any{"nested": true}, o.Extensions["x-root"])
	assert.Equal(t, "Docs", o.ExternalDocs.Description)
	assert.Equal(t, "Things", o.Tags[0].Description)
	assert.Equal(t, []map[string][]string{{"oauth2": {"read", "write"}}}, o.Security)
	require.Contains(t, o.Webhooks, "hook")
	assert.Equal(t, "hook-post", o.Webhooks["hook"].Post.OperationID)

	item := o.Paths["/things/{id}"]
	require.NotNil(t, item)
	assert.Equal(t, "Things", item.Summary)
	assert.Equal(t, "#/components/parameters/Id", item.Parameters[0].Ref)
	assert.Equal(t, []any{float64(1), float64(2)}, item.Extensions["x-path"])
	assert.Equal(t, "put-thing", item.Put.OperationID)
	assert.Equal(t, "post-thing", item.Post.OperationID)
	assert.Equal(t, "delete-thing", item.Delete.OperationID)
	assert.Equal(t, "options-thing", item.Options.OperationID)
	assert.Equal(t, "head-thing", item.Head.OperationID)
	assert.Equal(t, "patch-thing", item.Patch.OperationID)
	assert.Equal(t, "trace-thing", item.Trace.OperationID)
	assert.True(t, item.Patch.RequestBody.Required)

	get := item.Get
	assert.Equal(t, "get-thing", get.OperationID)
	assert.Equal(t, "ext", get.Extensions["x-op"])
	assert.True(t, get.Deprecated)
	assert.Equal(t, []map[string][]string{{"oauth2": {"read"}}, {}}, get.Security)
	assert.Equal(t, "https://get.example.com", get.Servers[0].URL)
	require.Len(t, get.Parameters, 1)
	param := get.Parameters[0]
	assert.Equal(t, "query", param.In)
	assert.True(t, param.Required)
	require.NotNil(t, param.Explode)
	assert.False(t, *param.Explode)
	assert.True(t, param.Schema.Nullable)
	assert.Equal(t, huma.TypeBoolean, param.Schema.Type)
	assert.Equal(t, true, param.Examples["yes"].Value)

	ok := get.Responses["200"]
	assert.Equal(t, "integer", ok.Headers["X-Rate"].Schema.Type)
	media := ok.Content["application/json"]
	assert.Equal(t, "#/components/schemas/Thing", media.Schema.Ref)
	assert.Equal(t, "text/plain", media.Encoding["id"].ContentType)
	assert.Equal(t, "https://example.com/one.json", media.Examples["one"].ExternalValue)
	assert.Equal(t, "$response.body#/id", ok.Links["self"].Parameters["id"])
	assert.Equal(t, "https://example.com", ok.Links["self"].Server.URL)
	assert.Equal(t, "#/components/responses/Error", get.Responses["default"].Ref)
	cb := get.Callbacks["onEvent"]["{$request.body#/url}"]
	require.NotNil(t, cb)
	assert.Equal(t, "No content", cb.Post.Responses["204"].Description)

	c := o.Components
	require.NotNil(t, c.Schemas)
	thing := c.Schemas.SchemaFromRef("#/components/schemas/Thing")
	require.NotNil(t, thing)
	assert.Equal(t, huma.TypeObject, thing.Type)
	assert.Equal(t, []string{"id"}, thing.Required)
	assert.Equal(t, "ext", c.Extensions["x-components"].(string))
	assert.Equal(t, "Error", c.Responses["Error"].Description)
	assert.Equal(t, "id", c.Parameters["Id"].Name)
	assert.Equal(t, float64(1), c.Examples["E"].Value)
	assert.Contains(t, c.RequestBodies["B"].Content, "text/plain")
	assert.Equal(t, "A header", c.Headers["H"].Description)
	assert.Equal(t, "#/paths/~1things~1{id}/get", c.Links["L"].OperationRef)
	assert.Equal(t, "callback-post", c.Callbacks["C"].Post.OperationID)
	assert.Equal(t, "pathitem-get", c.PathItems["P"].Get.OperationID)

	flows := c.SecuritySchemes["oauth2"].Flows
	assert.Equal(t, "https://example.com/auth", flows.Implicit.AuthorizationURL)
	assert.Equal(t, "Read", flows.Implicit.Scopes["read"])
	assert.Equal(t, "https://example.com/token", flows.Password.TokenURL)
	assert.Equal(t, "https://example.com/refresh", flows.ClientCredentials.RefreshURL)
	assert.Equal(t, true, flows.AuthorizationCode.Extensions["x-flow"])
	assert.Equal(t, "api_key", c.SecuritySchemes["key"].Name)
	assert.Equal(t, "header", c.SecuritySchemes["key"].In)
	assert.Equal(t, "JWT", c.SecuritySchemes["bearer"].BearerFormat)
	assert.Equal(t, "https://example.com/.well-known/openid-configuration", c.SecuritySchemes["oidc"].OpenIDConnectURL)

	// Re-marshaling must reproduce the original document, except that unused
	// schemas are pruned (none here: `Thing` is referenced).
	out, err := json.Marshal(&o)
	require.NoError(t, err)
	assert.JSONEq(t, fullOpenAPIDoc, string(out))
}

func TestOpenAPIUnmarshalGeneratedDocument(t *testing.T) {
	// A document generated by Huma must survive a round trip, e.g. for tools
	// that merge the specs of several services.
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	type Item struct {
		ID    string   `json:"id" format:"uuid"`
		Name  *string  `json:"name,omitempty" maxLength:"10"`
		Notes []string `json:"notes,omitempty"`
	}
	type Output struct {
		Body []Item
	}

	huma.Get(api, "/items/{id}", func(ctx context.Context, input *struct {
		ID    string `path:"id"`
		Limit int    `query:"limit" minimum:"1" default:"10"`
	}) (*Output, error) {
		return &Output{}, nil
	})
	huma.Post(api, "/items", func(ctx context.Context, input *struct {
		Body Item
	}) (*Output, error) {
		return &Output{}, nil
	})

	original, err := json.Marshal(api.OpenAPI())
	require.NoError(t, err)

	var decoded huma.OpenAPI
	require.NoError(t, json.Unmarshal(original, &decoded))
	require.NotNil(t, decoded.Components.Schemas)
	assert.NotEmpty(t, decoded.Components.Schemas.Map())

	roundTrip, err := json.Marshal(&decoded)
	require.NoError(t, err)
	assert.JSONEq(t, string(original), string(roundTrip))

	// The decoded schemas are usable by the registry.
	item := decoded.Components.Schemas.SchemaFromRef("#/components/schemas/Item")
	require.NotNil(t, item)
	assert.Equal(t, huma.TypeObject, item.Type)
}

func TestOpenAPIUnmarshalErrors(t *testing.T) {
	var o huma.OpenAPI
	assert.Error(t, json.Unmarshal([]byte(`{"openapi": 3}`), &o))
	assert.Error(t, json.Unmarshal([]byte(`{"openapi": "3.1.0", "info": {"title": 1}}`), &o))
	assert.Error(t, json.Unmarshal([]byte(`{"components": {"schemas": {"A": {"type": ["string", "integer"]}}}}`), &o))
	assert.Error(t, json.Unmarshal([]byte(`[]`), &o))

	// Documents without schemas leave the registry unset, and unknown
	// non-extension keys are ignored.
	o = huma.OpenAPI{}
	require.NoError(t, json.Unmarshal([]byte(`{"openapi": "3.1.0", "components": {"unknown": 1}}`), &o))
	assert.Nil(t, o.Components.Schemas)
}
