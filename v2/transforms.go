package huma

import (
	"bytes"
	"path"
	"reflect"
	"sync"
)

type schemaField struct {
	Schema string `json:"$schema"`
}

type SchemaLinkTransformer struct {
	prefix      string
	schemasPath string
	types       map[any]struct {
		t      reflect.Type
		ref    string
		header string
	}
	bufPool sync.Pool
}

func NewSchemaLinkTransformer(prefix, schemasPath string) *SchemaLinkTransformer {
	return &SchemaLinkTransformer{
		prefix:      prefix,
		schemasPath: schemasPath,
		types: map[any]struct {
			t      reflect.Type
			ref    string
			header string
		}{},
		bufPool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, 128))
			},
		},
	}
}

func (t *SchemaLinkTransformer) OnAddOperation(oapi *OpenAPI, op *Operation) {
	// Update registry to be able to get the type from a schema ref.
	// Register the type in t.types with the generated ref
	registry := oapi.Components.Schemas
	for _, resp := range op.Responses {
		for _, content := range resp.Content {
			if content.Schema.Ref == "" {
				continue
			}

			schema := registry.SchemaFromRef(content.Schema.Ref)
			if schema.Type != TypeObject || (schema.Properties != nil && schema.Properties["$schema"] != nil) {
				continue
			}

			// First, modify the schema to have the $schema field.
			schema.Properties["$schema"] = &Schema{
				Type:        TypeString,
				Format:      "uri",
				Description: "A URL to the JSON Schema for this object.",
				ReadOnly:    true,
			}

			// Then, create the wrapper Go type that has the $schema field.
			typ := deref(registry.TypeFromRef(content.Schema.Ref))

			extra := schemaField{
				Schema: t.schemasPath + "/" + path.Base(content.Schema.Ref) + ".json",
			}

			fields := []reflect.StructField{
				reflect.TypeOf(extra).Field(0),
			}
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if !f.IsExported() {
					continue
				}
				fields = append(fields, f)
			}

			newType := reflect.StructOf(fields)
			info := t.types[typ]
			info.t = newType
			info.ref = extra.Schema
			info.header = "<" + extra.Schema + ">; rel=\"describedBy\""
			t.types[typ] = info
		}
	}
}

func (t *SchemaLinkTransformer) Transform(ctx Context, op *Operation, status string, v any) (any, error) {
	if v == nil {
		return v, nil
	}

	typ := deref(reflect.TypeOf(v))

	if typ.Kind() != reflect.Struct {
		return v, nil
	}

	info := t.types[typ]
	if info.t == nil {
		return v, nil
	}

	host := ctx.GetHeader("Host")
	ctx.AppendHeader("Link", info.header)

	vv := reflect.Indirect(reflect.ValueOf(v))
	tmp := reflect.New(info.t).Elem()
	for i := 0; i < tmp.NumField(); i++ {
		f := tmp.Field(i)
		if !f.CanSet() {
			continue
		}
		if i == 0 {
			buf := t.bufPool.Get().(*bytes.Buffer)
			if len(host) >= 9 && host[:9] == "localhost" {
				buf.WriteString("http://")
			} else {
				buf.WriteString("https://")
			}
			buf.WriteString(host)
			buf.WriteString(info.ref)
			tmp.Field(i).SetString(buf.String())
			buf.Reset()
			t.bufPool.Put(buf)
		} else {
			tmp.Field(i).Set(vv.Field(i - 1))
		}
	}

	return tmp.Addr().Interface(), nil
}
