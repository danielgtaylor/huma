package huma_test

import (
	"context"
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
												ExclusiveMinimum: Ptr(0.0),
												ExclusiveMaximum: Ptr(100.0),
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
