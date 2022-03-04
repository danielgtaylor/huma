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
	"github.com/fatih/structs"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/handler"
	"github.com/koron-go/gqlcost"
)

type graphContextKey string

var graphKeyHeaders graphContextKey = "headers"

type GraphQLConfig struct {
	// Path where the GraphQL endpoint is available. Defaults to `/graphql`.
	Path string

	// GraphiQL sets whether the UI is available at the path. Defaults to off.
	GraphiQL bool

	// ComplexityLimit sets the maximum allowed complexity, which is calculated
	// as 1 for each field and 2 + (n * child) for each array with n children
	// created from sub-resource requests.
	ComplexityLimit int

	// Paginator defines the struct to be used for paginated responses. This
	// can be used to conform to different pagination styles if the underlying
	// API supports them, such as Relay. If not set, then
	// `GraphQLDefaultPaginator` is used.
	Paginator GraphQLPaginator

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

	// costMap tracks the type name -> field cost for any fields that aren't
	// the default cost of 1 (i.e. arrays of subresources).
	costMap gqlcost.CostMap

	// paginatorType stores the type for fast calls to `reflect.New`.
	paginatorType reflect.Type
}

// allResources recursively finds all resource and sub-resources and adds them
// to the `result` slice.
func allResources(r *Resource) []*Resource {
	result := []*Resource{}
	for _, sub := range r.subResources {
		result = append(result, sub)
		result = append(result, allResources(sub)...)
	}
	return result
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

// caluclateComplexity will populate the cost map whenever a resource request
// is made for a field. If the request returns a list and has a count-limiting
// argument, then that is used as a multiplier for downstream values.
func calculateComplexity(config *GraphQLConfig, parentName string, model reflect.Type, out graphql.Output, fieldName string) {
	if config.costMap[parentName].Fields == nil {
		config.costMap[parentName] = gqlcost.TypeCost{
			Fields: gqlcost.FieldsCost{},
		}
	}

	// All resources have a cost associated with fetching them. Always set
	// `useMultipliers` as that controls whether or not to apply parent
	// multipliers to the current field complexity value.
	cost := gqlcost.Cost{
		Complexity:     1,
		UseMultipliers: true,
	}
	if model.Kind() == reflect.Slice && strings.HasSuffix(out.Name(), "Collection") {
		// This is an array and we need to multiply by the number of items requested.
		cost.MultiplierFunc = func(m map[string]interface{}) int {
			// Try to get the max number of items requested from various well-known
			// argument names.
			result := 0
			found := false
			for _, arg := range []string{"first", "last", "limit", "count", "pageSize", "records"} {
				if _, ok := m[arg]; ok {
					v := reflect.ValueOf(m[arg])
					switch v.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						result += int(v.Int())
						found = true
					case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
						result += int(v.Uint())
						found = true
					}
				}
			}

			if found {
				return result
			}

			// No idea how many items will get returned, so we default to 10.
			return 10
		}
	}
	config.costMap[parentName].Fields[fieldName] = cost
}

func (r *Router) handleOperation(config *GraphQLConfig, parentName string, fields graphql.Fields, resource *Resource, op *Operation, ignoreParams map[string]bool) {
	model, headerNames, err := getModel(op)
	if err != nil || model == nil {
		// This is a GET but returns nothing???
		return
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
		typ, err := r.generateGraphModel(config, param.typ, "", nil, nil, nil)
		if err != nil {
			panic(err)
		}
		if param.In == inPath {
			typ = graphql.NewNonNull(typ)
		}
		var def interface{}
		if param.Schema != nil {
			def = param.Schema.Default
		}
		argsNameMap[jsName] = name
		args[jsName] = &graphql.ArgumentConfig{
			Type:         typ,
			Description:  param.Description,
			DefaultValue: def,
		}
	}

	// Convert the Go model to GraphQL Schema.
	out, err := r.generateGraphModel(config, model, resource.path, headerNames, ignoreParams, nil)
	if err != nil {
		panic(err)
	}

	calculateComplexity(config, parentName, model, out, last)

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

			// Fire off the request but don't wait for the response. Instead, we
			// return a "thunk" which is a function to be resolved later (like a js
			// Promise) which GraphQL resolves *after* visiting all fields in
			// breadth-first order. This ensures we kick off all the requests in
			// parallel but then wait for all the results until processing deeper
			// into the query.
			// See also https://github.com/graphql-go/graphql/pull/388.
			done := make(chan bool)
			var result interface{}
			var respHeader http.Header
			go func() {
				result, respHeader, err = r.fetch(headers, path, queryParams)
				done <- true
			}()

			return func() (interface{}, error) {
				// Wait for request goroutine to complete. Since it's done async we
				// have to handle the errors here, not in the goroutine above.
				<-done
				if err != nil {
					return nil, err
				}

				// Create a simple map of header name to header value.
				headerMap := map[string]string{}
				for headerName := range respHeader {
					headerMap[casing.LowerCamel(strings.ToLower(headerName))] = respHeader.Get(headerName)
				}

				paramMap := config.paramMappings[resource.path]

				if m, ok := result.(map[string]interface{}); ok {
					// Save params for child requests to use. By putting this into the
					// response object but not into the GraphQL schema it ensures that
					// downstream resolvers can access it but it never gets sent to the
					// client as part of a response.
					newParams := map[string]interface{}{}
					for k, v := range params {
						newParams[k] = v
					}
					for paramName, fieldName := range paramMap {
						newParams[paramName] = m[fieldName]
					}
					m["__params"] = newParams
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
					paginator := reflect.New(config.paginatorType).Interface().(GraphQLPaginator)
					paginator.Load(headerMap, s)

					// Other code expects map[string]interface{} not structs, so here we
					// convert to a map in case there is further processing to do.
					converter := structs.New(paginator)
					converter.TagName = "json"
					result = converter.Map()
				}
				return result, nil
			}, nil
		},
	}
}

func (r *Router) handleResource(config *GraphQLConfig, parentName string, fields graphql.Fields, resource *Resource, ignoreParams map[string]bool) {
	for _, op := range resource.operations {
		if op.method != http.MethodGet {
			continue
		}

		r.handleOperation(config, parentName, fields, resource, op, ignoreParams)
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
		resources = append(resources, allResources(resource)...)
	}
	sort.Slice(resources, func(i, j int) bool {
		return len(resources[i].path) < len(resources[j].path)
	})

	if config.Path == "" {
		config.Path = "/graphql"
	}
	if config.Paginator == nil {
		config.Paginator = &GraphQLDefaultPaginator{}
	}
	config.known = map[string]graphql.Output{}
	config.resources = resources
	config.paramMappings = map[string]map[string]string{}
	config.costMap = gqlcost.CostMap{}
	config.paginatorType = reflect.TypeOf(config.Paginator).Elem()

	for _, resource := range resources {
		r.handleResource(config, "Query", fields, resource, map[string]bool{})
	}

	root := graphql.ObjectConfig{Name: "Query", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(root)}
	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		panic(err)
	}

	if config.ComplexityLimit > 0 {
		gqlcost.AddCostAnalysisRule(gqlcost.AnalysisOptions{
			MaximumCost: config.ComplexityLimit,
			CostMap:     config.costMap,
		})
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
