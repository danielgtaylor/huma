package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"

	"github.com/danielgtaylor/casing"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
)

type graphContextKey string

var graphKeyHeaders graphContextKey = "headers"

type GraphQLConfig struct {
	// Path where the GraphQL endpoint is available. Defaults to `/graphql`.
	Path string

	// GraphiQL sets whether the UI is available at the path. Defaults to off.
	GraphiQL bool

	// known keeps track of known structs since they can only be defined once
	// per GraphQL endpoint. If used by multiple HTTP operations, they must
	// reference the same struct converted to GraphQL schema.
	known map[string]graphql.Output

	// resources is a list of all resources in the router.
	resources []*Resource

	// paramMappings are a mapping of URL template to a map of OpenAPI param name
	// to Go struct field JSON name. For example, `/items` could have a
	// mapping of `item-id` -> `id` if the structs returned for each item have
	// a field named `id` that should be used as input to e.g.
	// `/items/{item-id}/prices`. These mappings are configured by putting a
	// tag `graphParam` on your go struct fields.
	paramMappings map[string]map[string]string
}

// allResources recursively finds all resource and sub-resources and adds them
// to the `result` slice.
func allResources(result []*Resource, r *Resource) {
	for _, sub := range r.subResources {
		result = append(result, sub)
		allResources(result, sub)
	}
}

// fetch from a Huma router. Returns the parsed JSON.
func (r *Router) fetch(headers http.Header, path string, query map[string]interface{}) (interface{}, http.Header, error) {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	// Keep it simple & fast for these internal requests.
	headers.Set("Accept", "application/json")
	headers.Set("Accept-Encoding", "none")
	req.Header = headers
	q := req.URL.Query()
	for k, v := range query {
		q.Set(k, fmt.Sprintf("%v", v))
	}
	req.URL.RawQuery = q.Encode()
	r.ServeHTTP(w, req)
	if w.Result().StatusCode >= 400 {
		return nil, nil, fmt.Errorf("error response from server while fetching %s: %d\n%s", path, w.Result().StatusCode, w.Body.String())
	}
	var body interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	return body, w.Result().Header, err
}

// getModel returns the schema and model for the operation's first HTTP 2xx
// response that is found.
func getModel(op *Operation) (reflect.Type, []string, error) {
	for _, resp := range op.responses {
		if resp.status >= 200 && resp.status < 300 && resp.model != nil {
			return resp.model, resp.headers, nil
		}
	}
	return nil, nil, fmt.Errorf("no model found for %s", op.id)
}

func (r *Router) handleResource(config *GraphQLConfig, fields graphql.Fields, resource *Resource, ignoreParams map[string]bool) {
	for _, op := range resource.operations {
		if op.method != http.MethodGet {
			continue
		}

		model, headerNames, err := getModel(op)
		if err != nil || model == nil {
			// This is a GET but returns nothing???
			continue
		}

		// `/things` -> `things`
		// `/things/{thing-id}` -> `thingsItem(thingId)`
		// `/things/{thing-id}/sub` -> `sub(thingId)`
		parts := strings.Split(strings.Trim(resource.path, "/"), "/")
		last := parts[len(parts)-1]
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i][0] == '{' {
				if i > 0 {
					last = parts[i-1] + "Item"
				}
				continue
			}
			break
		}

		// Setup input arguments (i.e. OpenAPI operation params).
		args := graphql.FieldConfigArgument{}
		argsNameMap := map[string]string{}
		for name, param := range op.params {
			if ignoreParams[name] || param.Internal {
				// This will be handled automatically.
				continue
			}
			jsName := casing.LowerCamel(name)
			typ, err := r.generateGraphModel(config, param.typ, "", nil, nil)
			if err != nil {
				panic(err)
			}
			argsNameMap[jsName] = name
			args[jsName] = &graphql.ArgumentConfig{
				Type:        typ,
				Description: param.Description,
			}
		}

		// Convert the Go model to GraphQL Schema.
		out, err := r.generateGraphModel(config, model, resource.path, headerNames, ignoreParams)
		if err != nil {
			panic(err)
		}

		fields[last] = &graphql.Field{
			Type:        out,
			Description: op.description,
			Args:        args,
			Resolve: func(p graphql.ResolveParams) (interface{}, error) {
				// Fetch and populate this resource from the underlying REST API.
				headers := p.Context.Value(graphKeyHeaders).(http.Header).Clone()
				path := resource.path
				queryParams := map[string]interface{}{}

				// Handle pre-filled args, then passed args
				params := map[string]interface{}{}
				if p.Source != nil {
					if m, ok := p.Source.(map[string]interface{}); ok {
						if m["__params"] != nil {
							params = m["__params"].(map[string]interface{})
							for k, v := range params {
								path = strings.Replace(path, "{"+k+"}", fmt.Sprintf("%v", v), 1)
							}
						}
					}
				}

				for arg := range p.Args {
					// Passed args get saved for later use.
					params[argsNameMap[arg]] = p.Args[arg]

					// Apply the arg to the request.
					param := op.params[argsNameMap[arg]]
					if param.In == inPath {
						path = strings.Replace(path, "{"+argsNameMap[arg]+"}", fmt.Sprintf("%v", p.Args[arg]), 1)
					} else if param.In == inQuery {
						queryParams[argsNameMap[arg]] = p.Args[arg]
					} else if param.In == inHeader {
						headers.Set(argsNameMap[arg], fmt.Sprintf("%v", p.Args[arg]))
					}
				}

				result, respHeader, err := r.fetch(headers, path, queryParams)
				if err != nil {
					return nil, err
				}

				paramMap := config.paramMappings[resource.path]

				if m, ok := result.(map[string]interface{}); ok {
					// Save params for child requests to use.
					newParams := map[string]interface{}{}
					for k, v := range params {
						newParams[k] = v
					}
					for paramName, fieldName := range paramMap {
						newParams[paramName] = m[fieldName]
					}
					m["__params"] = newParams

					// Set headers so they can be queried.
					headerMap := map[string]string{}
					for headerName := range respHeader {
						headerMap[casing.LowerCamel(strings.ToLower(headerName))] = respHeader.Get(headerName)
					}
					m["headers"] = headerMap
				} else if s, ok := result.([]interface{}); ok {
					// Since this is a list, we set params on each item.
					for _, item := range s {
						if m, ok := item.(map[string]interface{}); ok {
							newParams := map[string]interface{}{}
							for k, v := range params {
								newParams[k] = v
							}
							for paramName, fieldName := range paramMap {
								newParams[paramName] = m[fieldName]
							}
							m["__params"] = newParams
						}
					}
				}
				return result, nil
			},
		}
	}
}

// EnableGraphQL turns on a read-only GraphQL endpoint.
func (r *Router) EnableGraphQL(config *GraphQLConfig) {
	fields := graphql.Fields{}

	if config == nil {
		config = &GraphQLConfig{}
	}

	// Collect all resources for the top-level operations.
	resources := []*Resource{}
	for _, resource := range r.resources {
		resources = append(resources, resource)
		allResources(resources, resource)
	}
	sort.Slice(resources, func(i, j int) bool {
		return len(resources[i].path) < len(resources[j].path)
	})

	if config.Path == "" {
		config.Path = "/graphql"
	}
	config.known = map[string]graphql.Output{}
	config.resources = resources
	config.paramMappings = map[string]map[string]string{}

	for _, resource := range resources {
		r.handleResource(config, fields, resource, map[string]bool{})
	}

	root := graphql.ObjectConfig{Name: "Query", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(root)}
	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		panic(err)
	}

	h := handler.New(&handler.Config{
		Schema:   &schema,
		Pretty:   true,
		GraphiQL: config.GraphiQL,
	})
	r.mux.HandleFunc(config.Path, func(w http.ResponseWriter, r *http.Request) {
		// Save the headers for future requests as they can contain important
		// information.
		r = r.WithContext(context.WithValue(r.Context(), graphKeyHeaders, r.Header))
		h.ServeHTTP(w, r)
	})
}
