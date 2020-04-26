package main

import (
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/schema"
)

// Item stores some value.
type Item struct {
	ID    string `json:"id"`
	Value int32  `json:"value"`
}

func main() {
	r := huma.NewRouter("Unsafe Test", "1.0.0")

	// Generate response schema for docs.
	s, _ := schema.Generate(reflect.TypeOf(Item{}))

	r.Resource("/unsafe",
		huma.PathParam("id", "desc"),
		huma.ResponseJSON(http.StatusOK, "doc", huma.Schema(*s)),
	).Get("doc", huma.UnsafeHandler(func(inputs ...interface{}) []interface{} {
		// Get the ID, which is the first input and will be a string since it's
		// a path parameter.
		id := inputs[0].(string)

		// Return an item with the passed in ID.
		return []interface{}{&Item{
			ID:    id,
			Value: 123,
		}}
	}))

	r.Run()
}
