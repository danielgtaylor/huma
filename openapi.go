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

	// Internal params are excluded from the OpenAPI document and can set up
	// params sent between a load balander / proxy and the service internally.
	Internal bool `json:"-"`
}

type oaComponents struct {
	Schemas map[string]*schema.Schema `json:"schemas,omitempty"`
}

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

	s, err := schema.GenerateWithMode(t, mode, nil)
	if err != nil {
		panic(err)
	}

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
