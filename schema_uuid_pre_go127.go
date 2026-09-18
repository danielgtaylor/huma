//go:build !go1.27

package huma

import "reflect"

func stdlibUUIDSchema(reflect.Type, bool) *Schema { return nil }
