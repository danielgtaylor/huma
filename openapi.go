package huma

import (
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
	Example     interface{}    `json:"example,omitempty"`
	Explode     *bool          `json:"explode,omitempty"`

	// Internal params are excluded from the OpenAPI document and can set up
	// params sent between a load balander / proxy and the service internally.
	Internal bool `json:"-"`
}
