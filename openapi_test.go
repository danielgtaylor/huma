package huma_test

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
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

func TestFixWildcardPaths(t *testing.T) {
	input := map[string]*huma.PathItem{
		// ServeMux
		"/api/{path...}":       {},
		"/files/{filepath...}": {},
		// Gorilla Mux
		"/mux/{path:.*}":    {},
		"/mux/v1/{rest:.*}": {},
		// Gin, HttpRouter, BunRouter
		"/gin/*filepath":   {},
		"/router/v1/*rest": {},
		// Chi, Echo
		"/chi/*":         {},
		"/echo/static/*": {},
		// Fiber
		"/fiber/+":        {},
		"/fiber/assets/+": {},
		// No wildcard (unchanged)
		"/users/{id}":   {},
		"/api/v1/items": {},
	}

	expected := map[string]bool{
		// ServeMux
		"/api/{path}":       true,
		"/files/{filepath}": true,
		// Gorilla Mux
		"/mux/{path}":    true,
		"/mux/v1/{rest}": true,
		// Gin, HttpRouter, BunRouter
		"/gin/{filepath}":   true,
		"/router/v1/{rest}": true,
		// Chi, Echo
		"/chi/{path}":         true,
		"/echo/static/{path}": true,
		// Fiber
		"/fiber/{path}":        true,
		"/fiber/assets/{path}": true,
		// No wildcard (unchanged)
		"/users/{id}":   true,
		"/api/v1/items": true,
	}

	result := huma.FixWildcardPaths(input)

	require.Len(t, result, len(expected), "result should have same number of paths")

	for path := range result {
		assert.True(t, expected[path], "unexpected path in result: %q", path)
	}

	// Test nil input
	assert.Nil(t, huma.FixWildcardPaths(nil))
}
