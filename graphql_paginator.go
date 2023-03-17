package huma

import (
	"net/url"
	"strings"

	link "github.com/tent/http-link-go"
)

// GraphQLPaginator defines how to turn list responses from the HTTP API to
// GraphQL response objects.
type GraphQLPaginator interface {
	// Load the paginated response from the given headers and body. After this
	// call completes, your struct instance should be ready to send back to
	// the client.
	Load(headers map[string]string, body []interface{}) error
}

// GraphQLHeaders is a placeholder to be used in `GraphQLPaginator` struct
// implementations which gets replaced with a struct of response headers.
type GraphQLHeaders map[string]string

// GraphQLItems is a placeholder to be used in `GraphQLPaginator` struct
// implementations which gets replaced with a list of the response items model.
type GraphQLItems []interface{}

// GraphQLPaginationParams provides params for link relationships so that
// new GraphQL queries to get e.g. the next page of items are easy to construct.
type GraphQLPaginationParams struct {
	First map[string]string `json:"first" doc:"First page link relationship"`
	Next  map[string]string `json:"next" doc:"Next page link relationship"`
	Prev  map[string]string `json:"prev" doc:"Previous page link relationship"`
	Last  map[string]string `json:"last" doc:"Last page link relationship"`
}

// GraphQLDefaultPaginator provides a default generic paginator implementation
// that makes no assumptions about pagination parameter names, headers, etc.
// It enables clients to access the response items (edges) as well as any
// response headers. If a link relation header is found in the response, then
// link relationships are parsed and turned into easy-to-use parameters for
// subsequent requests.
type GraphQLDefaultPaginator struct {
	Headers GraphQLHeaders          `json:"headers"`
	Links   GraphQLPaginationParams `json:"links" doc:"Pagination link parameters"`
	Edges   GraphQLItems            `json:"edges"`
}

// Load the paginated response and parse link relationships if available.
func (g *GraphQLDefaultPaginator) Load(headers map[string]string, body []interface{}) error {
	g.Headers = headers
	if parsed, err := link.Parse(headers["link"]); err == nil && len(parsed) > 0 {
		for _, item := range parsed {
			parsed, err := url.Parse(item.URI)
			if err != nil {
				continue
			}
			params := map[string]string{}
			query := parsed.Query()
			for k := range query {
				params[k] = query.Get(k)
			}

			switch strings.ToLower(item.Rel) {
			case "first":
				g.Links.First = params
			case "next":
				g.Links.Next = params
			case "prev":
				g.Links.Prev = params
			case "last":
				g.Links.Last = params
			}
		}
	}
	g.Edges = body
	return nil
}
