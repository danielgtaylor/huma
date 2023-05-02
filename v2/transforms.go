package huma

import (
	"path"
	"reflect"
	"sync"
)

type schemaField struct {
	Schema string `json:"$schema"`
}

// TransformAddSchemaField adds a $schema field to the top level of the response
// body that points to the schema for the response.
func TransformAddSchemaField(prefix string) func(op *Operation, status string, v any) (any, error) {
	// Keep a cache of types we've seen to make this much faster after the first
	// one comes through.
	mu := sync.RWMutex{}
	types := map[any]struct {
		t   reflect.Type
		ref reflect.Value
	}{}
	return func(op *Operation, status string, v any) (any, error) {
		if v == nil {
			return v, nil
		}

		if deref(reflect.TypeOf(v)).Kind() != reflect.Struct {
			return v, nil
		}

		mu.RLock()
		info, ok := types[v]
		mu.RUnlock()
		if !ok {
			// TODO: ignore if it already has a $schema field...

			resp := op.Responses[status]
			if resp == nil {
				resp = op.Responses["default"]
				if resp == nil {
					return v, nil
				}
			}
			mt := resp.Content["application/json"]
			if mt.Schema == nil || mt.Schema.Ref == "" {
				return v, nil
			}

			extra := schemaField{
				Schema: prefix + "/" + path.Base(mt.Schema.Ref) + ".json",
			}

			t := deref(reflect.TypeOf(v))
			fields := []reflect.StructField{
				reflect.TypeOf(extra).Field(0),
			}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if !f.IsExported() {
					continue
				}
				fields = append(fields, f)
			}

			typ := reflect.StructOf(fields)
			info.t = typ
			info.ref = reflect.ValueOf(extra).Field(0)
			mu.Lock()
			types[v] = info
			mu.Unlock()
		}

		vv := reflect.Indirect(reflect.ValueOf(v))
		tmp := reflect.New(info.t).Elem()
		for i := 0; i < tmp.NumField(); i++ {
			f := tmp.Field(i)
			if !f.CanSet() {
				continue
			}
			if i == 0 {
				tmp.Field(i).Set(info.ref)
			} else {
				tmp.Field(i).Set(vv.Field(i - 1))
			}
		}

		return tmp.Addr().Interface(), nil
	}
}
