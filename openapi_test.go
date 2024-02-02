package huma_test

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
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
