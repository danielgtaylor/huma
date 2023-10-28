package huma_test

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

// Item represents a single item with a unique ID.
type Item struct {
	ID string `json:"id"`
}

// ItemsResponse is a response containing a list of items.
type ItemsResponse struct {
	Body []Item `json:"body"`
}

// ItemsHandler handles item-related CRUD operations.
type ItemsHandler struct{}

// RegisterListItems registers the `list-items` operation with the given API.
// Because the method starts with `Register` it will be automatically called
// by `huma.AutoRegister` down below.
func (s *ItemsHandler) RegisterListItems(api huma.API) {
	// Register a list operation to get all the items.
	huma.Register(api, huma.Operation{
		OperationID: "list-items",
		Method:      http.MethodGet,
		Path:        "/items",
	}, func(ctx context.Context, input *struct{}) (*ItemsResponse, error) {
		resp := &ItemsResponse{}
		resp.Body = []Item{{ID: "123"}}
		return resp, nil
	})
}

func ExampleAutoRegister() {
	// Create the router and API.
	router := chi.NewMux()
	api := NewExampleAPI(router, huma.DefaultConfig("My Service", "1.0.0"))

	// Create the item handler and register all of its operations.
	itemsHandler := &ItemsHandler{}
	huma.AutoRegister(api, itemsHandler)

	// Confirm the list operation was registered.
	fmt.Println(api.OpenAPI().Paths["/items"].Get.OperationID)
	// Output: list-items
}
