//go:build go1.27

package huma

import (
	"reflect"
	"uuid"
)

var uuidType = reflect.TypeFor[uuid.UUID]()

func stdlibUUIDSchema(t reflect.Type, isPointer bool) *Schema {
	if t == uuidType {
		return &Schema{Type: TypeString, Nullable: isPointer, Format: "uuid"}
	}
	return nil
}
