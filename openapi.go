package huma

import (
	"fmt"
	"net/http"

	"github.com/goccy/go-yaml"
)

// Contact information to get support for the API.
type Contact struct {
	// Name of the contact person/organization.
	Name string `yaml:"name,omitempty"`

	// URL pointing to the contact information.
	URL string `yaml:"url,omitempty"`

	// Email address of the contact person/organization.
	Email string `yaml:"email,omitempty"`

	// Extensions (user-defined properties), if any. Values in this map will
	// be marshalled as siblings of the other properties above.
	Extensions map[string]any `yaml:",inline"`
}

// License name & link for using the API.
type License struct {
	// Name of the license.
	Name string `yaml:"name"`

	// Identifier SPDX license expression for the API. This field is mutually
	// exclusive with the URL field.
	Identifier string `yaml:"identifier,omitempty"`

	// URL pointing to the license. This field is mutually exclusive with the
	// Identifier field.
	URL string `yaml:"url,omitempty"`

	// Extensions (user-defined properties), if any. Values in this map will
	// be marshalled as siblings of the other properties above.
	Extensions map[string]any `yaml:",inline"`
}

// Info object that provides metadata about the API. The metadata MAY be used by the clients if needed, and MAY be presented in editing or documentation generation tools for convenience.
type Info struct {
	// Title of the API.
	Title string `yaml:"title"`

	// Description of the API. CommonMark syntax MAY be used for rich text representation.
	Description string `yaml:"description,omitempty"`

	// TermsOfService URL for the API.
	TermsOfService string `yaml:"termsOfService,omitempty"`

	// Contact information to get support for the API.
	Contact *Contact `yaml:"contact,omitempty"`

	// License name & link for using the API.
	License *License `yaml:"license,omitempty"`

	// Version of the OpenAPI document (which is distinct from the OpenAPI Specification version or the API implementation version).
	Version string `yaml:"version"`

	// Extensions (user-defined properties), if any. Values in this map will
	// be marshalled as siblings of the other properties above.
	Extensions map[string]any `yaml:",inline"`
}

// ServerVariable for server URL template substitution.
type ServerVariable struct {
	// Enumeration of string values to be used if the substitution options are from a limited set. The array MUST NOT be empty.
	Enum []string `yaml:"enum,omitempty"`

	// Default value to use for substitution, which SHALL be sent if an alternate value is not supplied.
	Default string `yaml:"default"`

	// Description for the server variable. CommonMark syntax MAY be used for rich text representation.
	Description string `yaml:"description,omitempty"`

	// Extensions (user-defined properties), if any. Values in this map will
	// be marshalled as siblings of the other properties above.
	Extensions map[string]any `yaml:",inline"`
}

// Server URL, optionally with variables.
type Server struct {
	// URL to the target host. This URL supports Server Variables and MAY be relative, to indicate that the host location is relative to the location where the OpenAPI document is being served. Variable substitutions will be made when a variable is named in {brackets}.
	URL string `yaml:"url"`

	// Description of the host designated by the URL. CommonMark syntax MAY be used for rich text representation.
	Description string `yaml:"description,omitempty"`

	// Variables map between a variable name and its value. The value is used for substitution in the server’s URL template.
	Variables map[string]*ServerVariable `yaml:"variables,omitempty"`

	// Extensions (user-defined properties), if any. Values in this map will
	// be marshalled as siblings of the other properties above.
	Extensions map[string]any `yaml:",inline"`
}

type Example struct {
	Ref           string         `yaml:"$ref,omitempty"`
	Summary       string         `yaml:"summary,omitempty"`
	Description   string         `yaml:"description,omitempty"`
	Value         any            `yaml:"value,omitempty"`
	ExternalValue string         `yaml:"externalValue,omitempty"`
	Extensions    map[string]any `yaml:",inline"`
}

type Encoding struct {
	ContentType   string             `yaml:"contentType,omitempty"`
	Headers       map[string]*Header `yaml:"headers,omitempty"`
	Style         string             `yaml:"style,omitempty"`
	Explode       *bool              `yaml:"explode,omitempty"`
	AllowReserved bool               `yaml:"allowReserved,omitempty"`
	Extensions    map[string]any     `yaml:",inline"`
}

type MediaType struct {
	Schema     *Schema              `yaml:"schema,omitempty"`
	Example    any                  `yaml:"example,omitempty"`
	Examples   map[string]*Example  `yaml:"examples,omitempty"`
	Encoding   map[string]*Encoding `yaml:"encoding,omitempty"`
	Extensions map[string]any       `yaml:",inline"`
}

type Param struct {
	Ref           string              `yaml:"$ref,omitempty"`
	Name          string              `yaml:"name,omitempty"`
	In            string              `yaml:"in,omitempty"`
	Description   string              `yaml:"description,omitempty"`
	Required      bool                `yaml:"required,omitempty"`
	Deprecated    bool                `yaml:"deprecated,omitempty"`
	Style         string              `yaml:"style,omitempty"`
	Explode       *bool               `yaml:"explode,omitempty"`
	AllowReserved bool                `yaml:"allowReserved,omitempty"`
	Schema        *Schema             `yaml:"schema,omitempty"`
	Example       any                 `yaml:"example,omitempty"`
	Examples      map[string]*Example `yaml:"examples,omitempty"`
	Extensions    map[string]any      `yaml:",inline"`
}

type Header = Param

type RequestBody struct {
	Ref         string                `yaml:"$ref,omitempty"`
	Description string                `yaml:"description,omitempty"`
	Content     map[string]*MediaType `yaml:"content"`
	Required    bool                  `yaml:"required,omitempty"`
	Extensions  map[string]any        `yaml:",inline"`
}

type Link struct {
	Ref          string         `yaml:"$ref,omitempty"`
	OperationRef string         `yaml:"operationRef,omitempty"`
	OperationID  string         `yaml:"operationId,omitempty"`
	Parameters   map[string]any `yaml:"parameters,omitempty"`
	RequestBody  any            `yaml:"requestBody,omitempty"`
	Description  string         `yaml:"description,omitempty"`
	Server       *Server        `yaml:"server,omitempty"`
	Extensions   map[string]any `yaml:",inline"`
}

type Response struct {
	Ref         string                `yaml:"$ref,omitempty"`
	Description string                `yaml:"description,omitempty"`
	Headers     map[string]*Param     `yaml:"headers,omitempty"`
	Content     map[string]*MediaType `yaml:"content,omitempty"`
	Links       map[string]*Link      `yaml:"links,omitempty"`
	Extensions  map[string]any        `yaml:",inline"`
}

type Operation struct {
	// Huma-specific fields

	// Method is the HTTP method for this operation
	Method string `yaml:"-"`

	// Path is the URL path for this operation
	Path string `yaml:"-"`

	// DefaultStatus is the default HTTP status code for this operation. It will
	// be set to 200 or 204 if not specified, depending on whether the handler
	// returns a response body.
	DefaultStatus int `yaml:"-"`

	// MaxBodyBytes is the maximum number of bytes to read from the request
	// body. If not specified, the default is 1MB. Use -1 for unlimited. If
	// the limit is reached, then an HTTP 413 error is returned.
	MaxBodyBytes int64 `yaml:"-"`

	// Errors is a list of HTTP status codes that the handler may return. If
	// not specified, then a default error response is added to the OpenAPI.
	Errors []int `yaml:"-"`

	// SkipValidateParams disables validation of path, query, and header
	// parameters. This can speed up request processing if you want to handle
	// your own validation. Use with caution!
	SkipValidateParams bool `yaml:"-"`

	// SkipValidateBody disables validation of the request body. This can speed
	// up request processing if you want to handle your own validation. Use with
	// caution!
	SkipValidateBody bool `yaml:"-"`

	// Hidden will skip documenting this operation in the OpenAPI. This is
	// useful for operations that are not intended to be used by clients but
	// you'd still like the benefits of using Huma. Generally not recommended.
	Hidden bool `yaml:"-"`

	// OpenAPI fields

	Tags         []string              `yaml:"tags,omitempty"`
	Summary      string                `yaml:"summary,omitempty"`
	Description  string                `yaml:"description,omitempty"`
	ExternalDocs *ExternalDocs         `yaml:"externalDocs,omitempty"`
	OperationID  string                `yaml:"operationId,omitempty"`
	Parameters   []*Param              `yaml:"parameters,omitempty"`
	RequestBody  *RequestBody          `yaml:"requestBody,omitempty"`
	Responses    map[string]*Response  `yaml:"responses,omitempty"`
	Callbacks    map[string]*PathItem  `yaml:"callbacks,omitempty"`
	Deprecated   bool                  `yaml:"deprecated,omitempty"`
	Security     []map[string][]string `yaml:"security,omitempty"`
	Servers      []*Server             `yaml:"servers,omitempty"`
	Extensions   map[string]any        `yaml:",inline"`
}

type PathItem struct {
	Ref         string         `yaml:"$ref,omitempty"`
	Summary     string         `yaml:"summary,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Get         *Operation     `yaml:"get,omitempty"`
	Put         *Operation     `yaml:"put,omitempty"`
	Post        *Operation     `yaml:"post,omitempty"`
	Delete      *Operation     `yaml:"delete,omitempty"`
	Options     *Operation     `yaml:"options,omitempty"`
	Head        *Operation     `yaml:"head,omitempty"`
	Patch       *Operation     `yaml:"patch,omitempty"`
	Trace       *Operation     `yaml:"trace,omitempty"`
	Servers     []*Server      `yaml:"servers,omitempty"`
	Parameters  []*Param       `yaml:"parameters,omitempty"`
	Extensions  map[string]any `yaml:",inline"`
}

type OAuthFlow struct {
	AuthorizationURL string            `yaml:"authorizationUrl"`
	TokenURL         string            `yaml:"tokenUrl"`
	RefreshURL       string            `yaml:"refreshUrl,omitempty"`
	Scopes           map[string]string `yaml:"scopes"`
	Extensions       map[string]any    `yaml:",inline"`
}

type OAuthFlows struct {
	Implicit          *OAuthFlow     `yaml:"implicit,omitempty"`
	Password          *OAuthFlow     `yaml:"password,omitempty"`
	ClientCredentials *OAuthFlow     `yaml:"clientCredentials,omitempty"`
	AuthorizationCode *OAuthFlow     `yaml:"authorizationCode,omitempty"`
	Extensions        map[string]any `yaml:",inline"`
}

type SecurityScheme struct {
	Type             string         `yaml:"type"`
	Description      string         `yaml:"description,omitempty"`
	Name             string         `yaml:"name,omitempty"`
	In               string         `yaml:"in,omitempty"`
	Scheme           string         `yaml:"scheme,omitempty"`
	BearerFormat     string         `yaml:"bearerFormat,omitempty"`
	Flows            *OAuthFlows    `yaml:"flows,omitempty"`
	OpenIDConnectURL string         `yaml:"openIdConnectUrl,omitempty"`
	Extensions       map[string]any `yaml:",inline"`
}

type Components struct {
	Schemas         Registry                   `yaml:"schemas,omitempty"`
	Responses       map[string]*Response       `yaml:"responses,omitempty"`
	Parameters      map[string]*Param          `yaml:"parameters,omitempty"`
	Examples        map[string]*Example        `yaml:"examples,omitempty"`
	RequestBodies   map[string]*RequestBody    `yaml:"requestBodies,omitempty"`
	Headers         map[string]*Header         `yaml:"headers,omitempty"`
	SecuritySchemes map[string]*SecurityScheme `yaml:"securitySchemes,omitempty"`
	Links           map[string]*Link           `yaml:"links,omitempty"`
	Callbacks       map[string]*PathItem       `yaml:"callbacks,omitempty"`
	PathItems       map[string]*PathItem       `yaml:"pathItems,omitempty"`
	Extensions      map[string]any             `yaml:",inline"`
}

type ExternalDocs struct {
	Description string         `yaml:"description,omitempty"`
	URL         string         `yaml:"url"`
	Extensions  map[string]any `yaml:",inline"`
}

type Tag struct {
	Name         string         `yaml:"name"`
	Description  string         `yaml:"description,omitempty"`
	ExternalDocs *ExternalDocs  `yaml:"externalDocs,omitempty"`
	Extensions   map[string]any `yaml:",inline"`
}

type AddOpFunc func(oapi *OpenAPI, op *Operation)

type OpenAPI struct {
	OpenAPI           string                `yaml:"openapi"`
	Info              *Info                 `yaml:"info"`
	Servers           []*Server             `yaml:"servers,omitempty"`
	JSONSchemaDialect string                `yaml:"jsonSchemaDialect,omitempty"`
	Paths             map[string]*PathItem  `yaml:"paths,omitempty"`
	Webhooks          map[string]*PathItem  `yaml:"webhooks,omitempty"`
	Components        *Components           `yaml:"components,omitempty"`
	Security          []map[string][]string `yaml:"security,omitempty"`
	Tags              []*Tag                `yaml:"tags,omitempty"`
	ExternalDocs      *ExternalDocs         `yaml:"externalDocs,omitempty"`
	Extensions        map[string]any        `yaml:",inline"`

	// OnAddOperation is called when an operation is added to the OpenAPI via
	// `AddOperation`. You may bypass this by directly writing to the `Paths`
	// map instead.
	OnAddOperation []AddOpFunc `yaml:"-"`
}

func (o *OpenAPI) AddOperation(op *Operation) {
	if o.Paths == nil {
		o.Paths = map[string]*PathItem{}
	}

	item := o.Paths[op.Path]
	if item == nil {
		item = &PathItem{}
		o.Paths[op.Path] = item
	}

	switch op.Method {
	case http.MethodGet:
		item.Get = op
	case http.MethodPost:
		item.Post = op
	case http.MethodPut:
		item.Put = op
	case http.MethodPatch:
		item.Patch = op
	case http.MethodDelete:
		item.Delete = op
	case http.MethodHead:
		item.Head = op
	case http.MethodOptions:
		item.Options = op
	case http.MethodTrace:
		item.Trace = op
	default:
		panic(fmt.Sprintf("unknown method %s", op.Method))
	}

	for _, f := range o.OnAddOperation {
		f(o, op)
	}
}

func (o *OpenAPI) MarshalJSON() ([]byte, error) {
	// JSON doesn't support the `,inline` field tag, so we go through the YAML
	// marshaller instead. It's not quite as fast, but this operation should
	// only happen once on server load.
	// Note: it does mean the individual structs above cannot be marshalled
	// directly to JSON - you must marshal the entire OpenAPI struct with the
	// exception of individual schemas.
	return yaml.MarshalWithOptions(o, yaml.JSON())
}
