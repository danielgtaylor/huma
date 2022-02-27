package huma

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/casing"
	"github.com/graphql-go/graphql"
)

var (
	ipType  = reflect.TypeOf(net.IP{})
	uriType = reflect.TypeOf(url.URL{})
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
		fields = append(fields, getFields(newTyp)...)
	}

	return fields
}

// addHeaderFields will add a `headers` field which is an object with all
// defined headers as string fields.
func addHeaderFields(name string, fields graphql.Fields, headerNames []string) {
	if len(headerNames) > 0 {
		headerFields := graphql.Fields{}
		for _, name := range headerNames {
			headerFields[casing.LowerCamel(strings.ToLower(name))] = &graphql.Field{
				Type: graphql.String,
			}
		}
		fields["headers"] = &graphql.Field{
			Type: graphql.NewObject(graphql.ObjectConfig{
				Name:   casing.Camel(strings.Replace(name+" Headers", ".", " ", -1)),
				Fields: headerFields,
			}),
		}
	}
}

// generateGraphModel converts a Go type to GraphQL Schema.
func (r *Router) generateGraphModel(config *GraphQLConfig, t reflect.Type, urlTemplate string, headerNames []string, ignoreParams map[string]bool) (graphql.Output, error) {
	if t == ipType {
		return graphql.String, nil
	}

	switch t.Kind() {
	case reflect.Struct:
		// Handle special cases.
		switch t {
		case timeType:
			return graphql.DateTime, nil
		case uriType:
			return graphql.String, nil
		}

		if config.known[t.String()] != nil {
			return config.known[t.String()], nil
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

				out, err := r.generateGraphModel(config, f.Type, "", nil, ignoreParams)

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
							value := p.Source.(map[string]interface{})[name].(map[string]interface{})
							entries := []interface{}{}
							for k, v := range value {
								entries = append(entries, map[string]interface{}{
									"key":   k,
									"value": v,
								})
							}
							return entries, nil
						}
					}
				}
			}
		}

		config.paramMappings[urlTemplate] = paramMap
		for k := range paramMap {
			ignoreParams[k] = true
		}

		if urlTemplate != "" {
			for _, resource := range config.resources {
				if len(resource.path) > len(urlTemplate) {
					// This could be a child resource. Let's find the longest prefix match
					// among all the resources and if that value matches the current
					// resources's URL template then this is a direct child.
					var best *Resource
					for _, sub := range config.resources {
						if len(resource.path) > len(sub.path) && strings.HasPrefix(resource.path, sub.path) {
							if best == nil || len(best.path) < len(sub.path) {
								best = sub
							}
						}
					}
					if best != nil && best.path == urlTemplate {
						r.handleResource(config, fields, resource, ignoreParams)
					}
				}
			}
		}

		addHeaderFields(t.String(), fields, headerNames)

		if len(fields) == 0 {
			fields["_"] = &graphql.Field{
				Type: graphql.Boolean,
			}
		}

		out := graphql.NewObject(graphql.ObjectConfig{
			Name:   casing.Camel(strings.Replace(t.String(), ".", " ", -1)),
			Fields: fields,
		})
		config.known[t.String()] = out
		return out, nil
	case reflect.Map:
		// Ruh-roh... GraphQL doesn't support maps. So here we'll convert the map
		// into a list of objects with a key and value, then later use a resolver
		// function to convert from the map to this list of objects.
		if config.known[t.String()] != nil {
			return config.known[t.String()], nil
		}

		// map[string]MyObject -> StringMyObjectEntry
		name := casing.Camel(strings.Replace(t.Key().String()+" "+t.Elem().String()+" Entry", ".", " ", -1))

		keyModel, err := r.generateGraphModel(config, t.Key(), "", nil, ignoreParams)
		if err != nil {
			return nil, err
		}
		valueModel, err := r.generateGraphModel(config, t.Elem(), "", nil, ignoreParams)
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

		items, err := r.generateGraphModel(config, t.Elem(), urlTemplate, nil, ignoreParams)
		if err != nil {
			return nil, err
		}

		if headerNames != nil {
			// The presence of headerNames implies this is an HTTP resource and
			// not just any normal array within the response structure.
			name := items.Name() + "Collection"

			if config.known[name] != nil {
				return config.known[name], nil
			}

			fields := graphql.Fields{
				"edges": &graphql.Field{
					Type: graphql.NewList(items),
				},
			}

			addHeaderFields(name, fields, headerNames)

			wrapper := graphql.NewObject(graphql.ObjectConfig{
				Name:   name,
				Fields: fields,
			})

			config.known[name] = wrapper

			return wrapper, nil
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
		return r.generateGraphModel(config, t.Elem(), urlTemplate, headerNames, ignoreParams)
	}

	return nil, fmt.Errorf("unsupported type %s from %s", t.Kind(), t)
}
