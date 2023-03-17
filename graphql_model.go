package huma

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/casing"
	"github.com/graphql-go/graphql"
)

// getFields performs a breadth-first search for all fields including embedded
// ones. It may return multiple fields with the same name, the first of which
// represents the outer-most declaration.
func getFields(typ reflect.Type) []reflect.StructField {
	fields := make([]reflect.StructField, 0, typ.NumField())
	embedded := []reflect.StructField{}

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Anonymous {
			embedded = append(embedded, f)
			continue
		}

		fields = append(fields, f)
	}

	for _, f := range embedded {
		newTyp := f.Type
		if newTyp.Kind() == reflect.Ptr {
			newTyp = newTyp.Elem()
		}
		if newTyp.Kind() == reflect.Struct {
			fields = append(fields, getFields(newTyp)...)
		}
	}

	return fields
}

// addHeaderFields will add a `headers` field which is an object with all
// defined headers as string fields.
func addHeaderFields(name string, fields graphql.Fields, headerNames []string) {
	if len(headerNames) > 0 && fields["headers"] == nil {
		headerFields := graphql.Fields{}
		for _, name := range headerNames {
			headerFields[casing.LowerCamel(strings.ToLower(name))] = &graphql.Field{
				Type: graphql.String,
			}
		}
		fields["headers"] = &graphql.Field{
			Type: graphql.NewObject(graphql.ObjectConfig{
				Name:   casing.Camel(strings.ReplaceAll(name+" Headers", ".", " ")),
				Fields: headerFields,
			}),
		}
	}
}

// generateGraphModel converts a Go type to GraphQL Schema. It uses reflection
// to recursively crawl structures and can also handle sub-resources if the
// input type is a struct representing a resource.
func (r *Router) generateGraphModel(config *GraphQLConfig, t reflect.Type, urlTemplate string, headerNames []string, ignoreParams map[string]bool, listItems graphql.Output) (graphql.Output, error) {
	switch t.Kind() {
	case reflect.Struct:
		// Handle special cases.
		switch t {
		case timeType:
			return graphql.DateTime, nil
		}

		objectName := casing.Camel(strings.ReplaceAll(t.String(), ".", " "))
		if _, ok := reflect.New(t).Interface().(GraphQLPaginator); ok {
			// Special case: this is a paginator implementation, and we need to
			// generate a paginator specific to the item types it contains. This
			// sets the name to the item type + a suffix, e.g. `MyItemCollection`.
			objectName = listItems.Name() + "Collection"
		}

		if config.known[objectName] != nil {
			return config.known[objectName], nil
		}

		fields := graphql.Fields{}

		paramMap := map[string]string{}
		for _, f := range getFields(t) {
			jsonTags := strings.Split(f.Tag.Get("json"), ",")
			name := strings.ToLower(f.Name)
			if len(jsonTags) > 0 && jsonTags[0] != "" {
				name = jsonTags[0]
			}

			// JSON "-" means to ignore the field.
			if name != "-" {
				if mapping := f.Tag.Get("graphParam"); mapping != "" {
					paramMap[mapping] = name
				}

				if f.Type == reflect.TypeOf(GraphQLHeaders{}) {
					// Special case: generate an object for the known headers
					if len(headerNames) > 0 {
						headerFields := graphql.Fields{}
						for _, name := range headerNames {
							headerFields[casing.LowerCamel(strings.ToLower(name))] = &graphql.Field{
								Type: graphql.String,
							}
						}
						fields[name] = &graphql.Field{
							Name:        name,
							Description: "HTTP response headers",
							Type: graphql.NewObject(graphql.ObjectConfig{
								Name:   casing.Camel(strings.ReplaceAll(objectName+" "+name, ".", " ")),
								Fields: headerFields,
							}),
						}
						headerNames = []string{}
					}
					continue
				}

				if f.Type == reflect.TypeOf(GraphQLItems{}) {
					// Special case: items placeholder for list responses. This should
					// be replaced with the generated specific item schema.
					fields[name] = &graphql.Field{
						Name:        name,
						Description: "List items",
						Type:        graphql.NewList(listItems),
					}
					continue
				}

				out, err := r.generateGraphModel(config, f.Type, "", nil, ignoreParams, listItems)
				if err != nil {
					return nil, err
				}

				if name != "" {
					fields[name] = &graphql.Field{
						Name:        name,
						Type:        out,
						Description: f.Tag.Get("doc"),
					}

					if out == graphql.DateTime {
						// Since graphql expects a `time.Time` we have to parse it here.
						// TODO: figure out some way to pass-through the string?
						fields[name].Resolve = func(p graphql.ResolveParams) (interface{}, error) {
							if p.Source == nil || p.Source.(map[string]interface{})[name] == nil {
								return nil, nil
							}
							return time.Parse(time.RFC3339Nano, p.Source.(map[string]interface{})[name].(string))
						}
					}

					if f.Type.Kind() == reflect.Map {
						// Use a resolver to convert between the Go map and the GraphQL
						// list of {key, value} objects.
						fields[name].Resolve = func(p graphql.ResolveParams) (interface{}, error) {
							if p.Source == nil || p.Source.(map[string]interface{})[name] == nil {
								return nil, nil
							}
							entries := []interface{}{}
							m := reflect.ValueOf(p.Source.(map[string]interface{})[name])
							for _, k := range m.MapKeys() {
								entries = append(entries, map[string]interface{}{
									"key":   k.Interface(),
									"value": m.MapIndex(k).Interface(),
								})
							}
							return entries, nil
						}
					}
				}
			}
		}

		// Store the parameter mappings for later use in resolver functions.
		config.paramMappings[urlTemplate] = paramMap
		for k := range paramMap {
			ignoreParams[k] = true
		}

		if urlTemplate != "" {
			// The presence of a template means this is a resource. Try and find
			// all child resources.
			for _, resource := range config.resources {
				if len(resource.path) > len(urlTemplate) {
					// This could be a child resource. Let's find the longest prefix match
					// among all the resources and if that value matches the current
					// resources's URL template then this is a direct child, even if
					// it spans multiple URL path components or arguments.
					var best *Resource
					for _, sub := range config.resources {
						if len(resource.path) > len(sub.path) && strings.HasPrefix(resource.path, sub.path) {
							if best == nil || len(best.path) < len(sub.path) {
								best = sub
							}
						}
					}
					if best != nil && best.path == urlTemplate {
						r.handleResource(config, objectName, fields, resource, ignoreParams)
					}
				}
			}
		}

		addHeaderFields(objectName, fields, headerNames)

		if len(fields) == 0 {
			// JSON supports empty object (e.g. for future expansion) but GraphQL
			// does not, so here we add a dummy value that can be used in the query
			// and will always return `null`. The presence of this field being
			// null vs the containing object being `null` lets you know if the JSON
			// empty object was present or not.
			fields["_"] = &graphql.Field{
				Type: graphql.Boolean,
			}
		}

		out := graphql.NewObject(graphql.ObjectConfig{
			Name:   objectName,
			Fields: fields,
		})
		config.known[objectName] = out
		return out, nil
	case reflect.Map:
		// Ruh-roh... GraphQL doesn't support maps. So here we'll convert the map
		// into a list of objects with a key and value, then later use a resolver
		// function to convert from the map to this list of objects.
		if config.known[t.String()] != nil {
			return config.known[t.String()], nil
		}

		// map[string]MyObject -> StringMyObjectEntry
		name := casing.Camel(strings.ReplaceAll(t.Key().String()+" "+t.Elem().String()+" Entry", ".", " "))

		keyModel, err := r.generateGraphModel(config, t.Key(), "", nil, ignoreParams, listItems)
		if err != nil {
			return nil, err
		}
		valueModel, err := r.generateGraphModel(config, t.Elem(), "", nil, ignoreParams, listItems)
		if err != nil {
			return nil, err
		}

		fields := graphql.Fields{
			"key": &graphql.Field{
				Type: keyModel,
			},
			"value": &graphql.Field{
				Type: valueModel,
			},
		}

		out := graphql.NewList(graphql.NewObject(graphql.ObjectConfig{
			Name:   name,
			Fields: fields,
		}))

		config.known[t.String()] = out
		return out, nil
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// Special case: `[]byte` should be a Base-64 string.
			return graphql.String, nil
		}

		items, err := r.generateGraphModel(config, t.Elem(), urlTemplate, headerNames, ignoreParams, nil)
		if err != nil {
			return nil, err
		}

		if headerNames != nil {
			// The presence of headerNames implies this is an HTTP resource and
			// not just any normal array within the response structure.
			paginator, err := r.generateGraphModel(config, reflect.TypeOf(config.Paginator), "", headerNames, ignoreParams, items)
			if err != nil {
				return nil, err
			}

			if config.known[paginator.Name()] != nil {
				return config.known[paginator.Name()], nil
			}

			config.known[paginator.Name()] = paginator
			return paginator, nil
		}

		return graphql.NewList(items), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return graphql.Int, nil
	case reflect.Float32, reflect.Float64:
		return graphql.Float, nil
	case reflect.Bool:
		return graphql.Boolean, nil
	case reflect.String:
		return graphql.String, nil
	case reflect.Ptr:
		return r.generateGraphModel(config, t.Elem(), urlTemplate, headerNames, ignoreParams, listItems)
	}

	return nil, fmt.Errorf("unsupported type %s from %s", t.Kind(), t)
}
