package huma

import (
	"fmt"
	"reflect"

	"github.com/danielgtaylor/huma/schema"
)

// oaContact describes contact information for this API.
type oaContact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

// oaServer describes an OpenAPI 3 API server location
type oaServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

// paramLocation describes where in the HTTP request the parameter comes from.
type paramLocation string

// Parameter locations supported by OpenAPI 3
const (
	inPath   paramLocation = "path"
	inQuery  paramLocation = "query"
	inHeader paramLocation = "header"
)

// oaParam describes an OpenAPI 3 parameter
type oaParam struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	In          paramLocation  `json:"in"`
	Required    bool           `json:"required,omitempty"`
	Schema      *schema.Schema `json:"schema,omitempty"`
	Deprecated  bool           `json:"deprecated,omitempty"`
	Explode     *bool          `json:"explode,omitempty"`
	CLIName     string         `json:"x-cli-name,omitempty"`

	// Internal params are excluded from the OpenAPI document and can set up
	// params sent between a load balander / proxy and the service internally.
	Internal bool `json:"-"`
}

type oaComponents struct {
	Schemas         map[string]*schema.Schema   `json:"schemas,omitempty"`
	SecuritySchemes map[string]oaSecurityScheme `json:"securitySchemes,omitempty"`
}

// AddSchema creates and adds a new schema from a type.
func (c *oaComponents) AddSchema(t reflect.Type, mode schema.Mode, hint string) string {
	// Try to determine the type's name.
	name := t.Name()
	if name == "" && t.Kind() == reflect.Ptr {
		// Take the name of the pointed-to type.
		name = t.Elem().Name()
	}
	if name == "" && t.Kind() == reflect.Slice {
		// Take the name of the type in the array and append "List" to it.
		tmp := t.Elem()
		if tmp.Kind() == reflect.Ptr {
			tmp = tmp.Elem()
		}
		name = tmp.Name()
		if name != "" {
			name += "List"
		}
	}
	if name == "" {
		// No luck, fall back to the passed-in hint. Better than nothing.
		name = hint
	}

	var s *schema.Schema

	if t.Kind() == reflect.Slice {
		// We actually want to create two models: one for the container slice
		// and one for the items within it.
		ref := c.AddSchema(t.Elem(), mode, name+"Item")
		s = &schema.Schema{
			Type: schema.TypeArray,
			Items: &schema.Schema{
				Ref: ref,
			},
		}
	} else {
		var err error
		if s, err = schema.GenerateWithMode(t, mode, nil); err != nil {
			panic(err)
		}
	}

	return c.AddExistingSchema(s, name)
}

// AddExistingSchema adds an existing schema instance under the given name.
func (c *oaComponents) AddExistingSchema(s *schema.Schema, name string) string {
	orig := name
	num := 1
	for {
		if c.Schemas[name] == nil {
			// No existing schema, we are the first!
			break
		}

		if reflect.DeepEqual(c.Schemas[name], s) {
			// Existing schema matches!
			break
		}

		// If we are here, then an existing schema doesn't match and this is a new
		// type. So we will rename it in a deterministic fashion.
		num++
		name = fmt.Sprintf("%s%d", orig, num)
	}

	c.Schemas[name] = s

	return "#/components/schemas/" + name
}

type oaFlow struct {
	AuthorizationURL string            `json:"authorizationUrl,omitempty"`
	TokenURL         string            `json:"tokenUrl,omitempty"`
	Scopes           map[string]string `json:"scopes,omitempty"`
}

type oaFlows struct {
	ClientCredentials *oaFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode *oaFlow `json:"authorizationCode,omitempty"`
}

type oaSecurityScheme struct {
	Type   string  `json:"type"`
	Scheme string  `json:"scheme,omitempty"`
	Flows  oaFlows `json:"flows,omitempty"`
}
