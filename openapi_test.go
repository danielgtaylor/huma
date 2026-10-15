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
	"gopkg.in/yaml.v3"
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

func TestDowngradeWrapsRefSiblingsInAllOf(t *testing.T) {
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
										Type: huma.TypeObject,
										Properties: map[string]*huma.Schema{
											"location": {
												Ref:         "#/components/schemas/Location",
												Description: "User home address location",
												Extensions: map[string]any{
													"x-test": true,
												},
											},
										},
									},
								},
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
										"properties": {
											"location": {
												"allOf": [
													{
														"$ref": "#/components/schemas/Location"
													}
												],
												"description": "User home address location",
												"x-test": true
											}
										},
										"type": "object"
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	assert.JSONEq(t, expected, string(v30))
}

func TestDowngradePreservesExistingAllOfRefSiblings(t *testing.T) {
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
										Ref:         "#/components/schemas/Test",
										Description: "A schema description",
										AllOf: []*huma.Schema{
											{
												Type: huma.TypeObject,
												Properties: map[string]*huma.Schema{
													"name": {
														Type: huma.TypeString,
													},
												},
											},
										},
									},
								},
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
										"allOf": [
											{
												"$ref": "#/components/schemas/Test"
											},
											{
												"properties": {
													"name": {
														"type": "string"
													}
												},
												"type": "object"
											}
										],
										"description": "A schema description"
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	assert.JSONEq(t, expected, string(v30))
}

func TestDowngradeYAMLWrapsRefSiblingsInAllOf(t *testing.T) {
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
										Ref:         "#/components/schemas/Test",
										Description: "A schema description",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	v30, err := v31.DowngradeYAML()
	require.NoError(t, err)

	var spec map[string]any
	require.NoError(t, yaml.Unmarshal(v30, &spec))

	paths, ok := spec["paths"].(map[string]any)
	require.True(t, ok)
	path, ok := paths["/test"].(map[string]any)
	require.True(t, ok)
	get, ok := path["get"].(map[string]any)
	require.True(t, ok)
	responses, ok := get["responses"].(map[string]any)
	require.True(t, ok)
	response, ok := responses["200"].(map[string]any)
	require.True(t, ok)
	content, ok := response["content"].(map[string]any)
	require.True(t, ok)
	mediaType, ok := content["application/json"].(map[string]any)
	require.True(t, ok)
	schema, ok := mediaType["schema"].(map[string]any)
	require.True(t, ok)
	allOf, ok := schema["allOf"].([]any)
	require.True(t, ok)
	require.Len(t, allOf, 1)
	refSchema, ok := allOf[0].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "#/components/schemas/Test", refSchema["$ref"])
	assert.Equal(t, "A schema description", schema["description"])
	assert.NotContains(t, schema, "$ref")
}

func TestDowngradeWrapsRefSiblingsInParameterContent(t *testing.T) {
	v31 := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: map[string]*huma.PathItem{
			"/test": {
				Get: &huma.Operation{
					Parameters: []*huma.Param{
						{
							Name: "filter",
							In:   "query",
							Extensions: map[string]any{
								"content": map[string]any{
									"application/json": map[string]any{
										"schema": map[string]any{
											"$ref":        "#/components/schemas/Filter",
											"description": "Filter expression",
										},
									},
								},
							},
						},
					},
					Responses: map[string]*huma.Response{
						"204": {
							Description: "No content",
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
					"parameters": [
						{
							"name": "filter",
							"in": "query",
							"content": {
								"application/json": {
									"schema": {
										"allOf": [
											{
												"$ref": "#/components/schemas/Filter"
											}
										],
										"description": "Filter expression"
									}
								}
							}
						}
					],
					"responses": {
						"204": {
							"description": "No content"
						}
					}
				}
			}
		}
	}`

	assert.JSONEq(t, expected, string(v30))
}

func TestDowngradeWrapsRefSiblingsInComponentSchemas(t *testing.T) {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	registry.Map()["Wrapped"] = &huma.Schema{
		Type: huma.TypeObject,
		Properties: map[string]*huma.Schema{
			"property": {
				Ref:         "#/components/schemas/Property",
				Description: "Property ref",
			},
		},
		Items: &huma.Schema{
			Ref:         "#/components/schemas/Item",
			Description: "Item ref",
		},
		AdditionalProperties: &huma.Schema{
			Ref:         "#/components/schemas/Additional",
			Description: "Additional property ref",
		},
		Not: &huma.Schema{
			Ref:         "#/components/schemas/Not",
			Description: "Not ref",
		},
		OneOf: []*huma.Schema{
			{
				Ref:         "#/components/schemas/OneOf",
				Description: "OneOf ref",
			},
		},
		AnyOf: []*huma.Schema{
			{
				Ref:         "#/components/schemas/AnyOf",
				Description: "AnyOf ref",
			},
		},
		AllOf: []*huma.Schema{
			{
				Ref:         "#/components/schemas/AllOf",
				Description: "AllOf ref",
			},
		},
	}

	v31 := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Components: &huma.Components{
			Schemas: registry,
		},
		Paths: map[string]*huma.PathItem{
			"/wrapped": {
				Get: &huma.Operation{
					Responses: map[string]*huma.Response{
						"200": {
							Description: "OK",
							Content: map[string]*huma.MediaType{
								"application/json": {
									Schema: &huma.Schema{
										Ref: "#/components/schemas/Wrapped",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	v30, err := v31.Downgrade()
	require.NoError(t, err)

	spec := decodeOpenAPIJSON(t, v30)
	schema := specMap(t, spec, "components", "schemas", "Wrapped")

	assertWrappedRefSibling(t, specMap(t, schema, "properties", "property"), "#/components/schemas/Property", "Property ref")
	assertWrappedRefSibling(t, specMap(t, schema, "items"), "#/components/schemas/Item", "Item ref")
	assertWrappedRefSibling(t, specMap(t, schema, "additionalProperties"), "#/components/schemas/Additional", "Additional property ref")
	assertWrappedRefSibling(t, specMap(t, schema, "not"), "#/components/schemas/Not", "Not ref")
	assertWrappedRefSibling(t, specMapAt(t, schema, "oneOf", 0), "#/components/schemas/OneOf", "OneOf ref")
	assertWrappedRefSibling(t, specMapAt(t, schema, "anyOf", 0), "#/components/schemas/AnyOf", "AnyOf ref")
	assertWrappedRefSibling(t, specMapAt(t, schema, "allOf", 0), "#/components/schemas/AllOf", "AllOf ref")
}

func TestDowngradeWrapsRefSiblingsInComponentObjects(t *testing.T) {
	v31 := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Components: &huma.Components{
			Responses: map[string]*huma.Response{
				"TestResponse": {
					Description: "OK",
					Headers: map[string]*huma.Param{
						"X-Test": {
							Schema: &huma.Schema{
								Ref:         "#/components/schemas/ResponseHeader",
								Description: "Response header ref",
							},
						},
					},
					Content: map[string]*huma.MediaType{
						"application/json": {
							Schema: &huma.Schema{
								Ref:         "#/components/schemas/ResponseBody",
								Description: "Response body ref",
							},
						},
					},
				},
			},
			Parameters: map[string]*huma.Param{
				"TestParameter": {
					Name: "filter",
					In:   "query",
					Schema: &huma.Schema{
						Ref:         "#/components/schemas/Parameter",
						Description: "Parameter ref",
					},
					Extensions: map[string]any{
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"$ref":        "#/components/schemas/ParameterContent",
									"description": "Parameter content ref",
								},
							},
						},
					},
				},
			},
			RequestBodies: map[string]*huma.RequestBody{
				"TestRequest": {
					Content: map[string]*huma.MediaType{
						"application/json": {
							Schema: &huma.Schema{
								Ref:         "#/components/schemas/RequestBody",
								Description: "Request body ref",
							},
						},
					},
				},
			},
			Headers: map[string]*huma.Param{
				"TestHeader": {
					Schema: &huma.Schema{
						Ref:         "#/components/schemas/Header",
						Description: "Header ref",
					},
				},
			},
		},
	}

	v30, err := v31.Downgrade()
	require.NoError(t, err)

	spec := decodeOpenAPIJSON(t, v30)

	assertWrappedRefSibling(t, specMap(t, spec, "components", "responses", "TestResponse", "headers", "X-Test", "schema"), "#/components/schemas/ResponseHeader", "Response header ref")
	assertWrappedRefSibling(t, specMap(t, spec, "components", "responses", "TestResponse", "content", "application/json", "schema"), "#/components/schemas/ResponseBody", "Response body ref")
	assertWrappedRefSibling(t, specMap(t, spec, "components", "parameters", "TestParameter", "schema"), "#/components/schemas/Parameter", "Parameter ref")
	assertWrappedRefSibling(t, specMap(t, spec, "components", "parameters", "TestParameter", "content", "application/json", "schema"), "#/components/schemas/ParameterContent", "Parameter content ref")
	assertWrappedRefSibling(t, specMap(t, spec, "components", "requestBodies", "TestRequest", "content", "application/json", "schema"), "#/components/schemas/RequestBody", "Request body ref")
	assertWrappedRefSibling(t, specMap(t, spec, "components", "headers", "TestHeader", "schema"), "#/components/schemas/Header", "Header ref")
}

func TestDowngradeWrapsRefSiblingsInPathItemsAndOperations(t *testing.T) {
	v31 := &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: map[string]*huma.PathItem{
			"/test": {
				Parameters: []*huma.Param{
					{
						Name: "path-filter",
						In:   "query",
						Schema: &huma.Schema{
							Ref:         "#/components/schemas/PathParameter",
							Description: "Path parameter ref",
						},
					},
				},
				Post: operationWithSchemaLocations(),
			},
		},
		Webhooks: map[string]*huma.PathItem{
			"test-hook": {
				Post: operationWithSchemaLocations(),
			},
		},
		Components: &huma.Components{
			Extensions: map[string]any{
				"callbacks": map[string]any{
					"TestCallback": map[string]any{
						"{$request.body#/url}": map[string]any{
							"post": operationWithSchemaLocations(),
						},
					},
				},
			},
			PathItems: map[string]*huma.PathItem{
				"TestPathItem": {
					Parameters: []*huma.Param{
						{
							Name: "component-path-filter",
							In:   "query",
							Schema: &huma.Schema{
								Ref:         "#/components/schemas/ComponentPathParameter",
								Description: "Component path parameter ref",
							},
						},
					},
					Post: operationWithSchemaLocations(),
				},
			},
		},
	}

	v30, err := v31.Downgrade()
	require.NoError(t, err)

	spec := decodeOpenAPIJSON(t, v30)

	assertWrappedRefSibling(t, specMapAt(t, specMap(t, spec, "paths", "/test"), "parameters", 0, "schema"), "#/components/schemas/PathParameter", "Path parameter ref")
	assertOperationSchemaLocationsWrapped(t, specMap(t, spec, "paths", "/test", "post"))
	assertOperationSchemaLocationsWrapped(t, specMap(t, spec, "webhooks", "test-hook", "post"))
	assertOperationSchemaLocationsWrapped(t, specMap(t, spec, "components", "callbacks", "TestCallback", "{$request.body#/url}", "post"))
	assertWrappedRefSibling(t, specMapAt(t, specMap(t, spec, "components", "pathItems", "TestPathItem"), "parameters", 0, "schema"), "#/components/schemas/ComponentPathParameter", "Component path parameter ref")
	assertOperationSchemaLocationsWrapped(t, specMap(t, spec, "components", "pathItems", "TestPathItem", "post"))
}

func TestDowngradeRecursiveSchemaRefDoesNotExpandRef(t *testing.T) {
	type Node struct {
		Value string `json:"value"`
		Child *Node  `json:"child,omitempty" doc:"Child node"`
	}

	_, api := humatest.New(t)
	huma.Register(api, huma.Operation{
		Method: http.MethodGet,
		Path:   "/node",
	}, func(ctx context.Context, input *struct{}) (*struct {
		Body Node
	}, error) {
		return nil, nil
	})

	v30, err := api.OpenAPI().Downgrade()
	require.NoError(t, err)

	assert.Contains(t, string(v30), `"$ref":"#/components/schemas/Node"`)
	assert.Contains(t, string(v30), `"description":"Child node"`)
	assert.NotContains(t, string(v30), `"$ref":"#/components/schemas/Node","description"`)
}

func TestDowngradeDoesNotWrapSchemaKeysOutsideOpenAPIFields(t *testing.T) {
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
									Example: map[string]any{
										"schema": map[string]any{
											"$ref":        "#/components/schemas/Test",
											"description": "Not an OpenAPI schema field",
										},
									},
									Schema: &huma.Schema{
										Type: huma.TypeObject,
									},
								},
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
									"example": {
										"schema": {
											"$ref": "#/components/schemas/Test",
											"description": "Not an OpenAPI schema field"
										}
									},
									"schema": {
										"type": "object"
									}
								}
							}
						}
					}
				}
			}
		}
	}`

	assert.JSONEq(t, expected, string(v30))
}

func operationWithSchemaLocations() *huma.Operation {
	return &huma.Operation{
		Parameters: []*huma.Param{
			{
				Name: "filter",
				In:   "query",
				Schema: &huma.Schema{
					Ref:         "#/components/schemas/OperationParameter",
					Description: "Operation parameter ref",
				},
			},
		},
		RequestBody: &huma.RequestBody{
			Content: map[string]*huma.MediaType{
				"application/json": {
					Schema: &huma.Schema{
						Ref:         "#/components/schemas/OperationRequestBody",
						Description: "Operation request body ref",
					},
					Encoding: map[string]*huma.Encoding{
						"file": {
							Headers: map[string]*huma.Param{
								"X-Encoding": {
									Schema: &huma.Schema{
										Ref:         "#/components/schemas/EncodingHeader",
										Description: "Encoding header ref",
									},
								},
							},
						},
					},
				},
			},
		},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "OK",
				Headers: map[string]*huma.Param{
					"X-Response": {
						Schema: &huma.Schema{
							Ref:         "#/components/schemas/OperationResponseHeader",
							Description: "Operation response header ref",
						},
					},
				},
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: &huma.Schema{
							Ref:         "#/components/schemas/OperationResponseBody",
							Description: "Operation response body ref",
						},
					},
				},
			},
		},
	}
}

func assertOperationSchemaLocationsWrapped(t *testing.T, operation map[string]any) {
	t.Helper()

	assertWrappedRefSibling(t, specMapAt(t, operation, "parameters", 0, "schema"), "#/components/schemas/OperationParameter", "Operation parameter ref")
	assertWrappedRefSibling(t, specMap(t, operation, "requestBody", "content", "application/json", "schema"), "#/components/schemas/OperationRequestBody", "Operation request body ref")
	assertWrappedRefSibling(t, specMap(t, operation, "requestBody", "content", "application/json", "encoding", "file", "headers", "X-Encoding", "schema"), "#/components/schemas/EncodingHeader", "Encoding header ref")
	assertWrappedRefSibling(t, specMap(t, operation, "responses", "200", "headers", "X-Response", "schema"), "#/components/schemas/OperationResponseHeader", "Operation response header ref")
	assertWrappedRefSibling(t, specMap(t, operation, "responses", "200", "content", "application/json", "schema"), "#/components/schemas/OperationResponseBody", "Operation response body ref")
}

func decodeOpenAPIJSON(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var spec map[string]any
	require.NoError(t, json.Unmarshal(data, &spec))
	return spec
}

func specMap(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()

	current := any(root)
	for _, key := range path {
		m, ok := current.(map[string]any)
		require.Truef(t, ok, "expected object before key %q", key)
		current = m[key]
	}

	m, ok := current.(map[string]any)
	require.Truef(t, ok, "expected object at %v", path)
	return m
}

func specMapAt(t *testing.T, root map[string]any, arrayKey string, index int, path ...string) map[string]any {
	t.Helper()

	array, ok := root[arrayKey].([]any)
	require.Truef(t, ok, "expected array at key %q", arrayKey)
	require.Greater(t, len(array), index)

	m, ok := array[index].(map[string]any)
	require.Truef(t, ok, "expected object at %s[%d]", arrayKey, index)
	if len(path) == 0 {
		return m
	}
	return specMap(t, m, path...)
}

func assertWrappedRefSibling(t *testing.T, schema map[string]any, ref string, description string) {
	t.Helper()

	allOf, ok := schema["allOf"].([]any)
	require.True(t, ok)
	require.Len(t, allOf, 1)

	refSchema, ok := allOf[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, ref, refSchema["$ref"])
	assert.Equal(t, description, schema["description"])
	assert.NotContains(t, schema, "$ref")
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
