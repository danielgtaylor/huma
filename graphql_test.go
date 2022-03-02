package huma

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type CategoryParam struct {
	CategoryID string `path:"category-id"`
}

type CategorySummary struct {
	ID string `json:"id" graphParam:"category-id" doc:"Category ID"`
}

type Category struct {
	CategorySummary
	Featured bool   `json:"featured" doc:"Display as featured in the app"`
	Code     []byte `json:"code" doc:"Category code"`

	products map[string]*Product `json:"-"`
}

type ProductParam struct {
	ProductID string `path:"product-id"`
}

type ProductSummary struct {
	ID string `json:"id" graphParam:"product-id" doc:"Product ID"`
}

type Product struct {
	ProductSummary
	SuggestedPrice float32           `json:"suggested_price"`
	Created        *time.Time        `json:"created,omitempty" doc:"When this product was created"`
	Metadata       map[string]string `json:"metadata,omitempty" doc:"Additional information about the product"`
	Empty          *struct{}         `json:"empty"`

	stores map[string]*Store `json:"-"`
}

type StoreSummary struct {
	ID string `json:"id" graphParam:"store-id" doc:"Store ID"`
}

type Store struct {
	StoreSummary
	URL string `json:"url" doc:"Web link to buy product"`
}

func TestGraphQL(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2022-02-22T22:22:22Z")

	amazon := &Store{StoreSummary: StoreSummary{ID: "amazon"}, URL: "https://www.amazon.com/"}
	target := &Store{StoreSummary: StoreSummary{ID: "target"}, URL: "https://www.target.com/"}

	xsx := &Product{ProductSummary: ProductSummary{ID: "xbox_series_x"}, SuggestedPrice: 499.99, Created: &now, Metadata: map[string]string{"foo": "bar"}, stores: map[string]*Store{"amazon": amazon, "target": target}, Empty: &struct{}{}}
	ps5 := &Product{ProductSummary: ProductSummary{ID: "playstation_ps5"}, SuggestedPrice: 499.99, Created: &now, stores: map[string]*Store{"amazon": amazon}}
	ns := &Product{ProductSummary: ProductSummary{ID: "nintendo_switch"}, SuggestedPrice: 349.99, stores: map[string]*Store{"target": target}}

	videoGames := &Category{
		CategorySummary: CategorySummary{ID: "video_games"},
		Featured:        true,
		Code:            []byte{'h', 'i'},
		products: map[string]*Product{
			"xbox_series_x":   xsx,
			"playstation_ps5": ps5,
			"nintendo_switch": ns,
		},
	}

	categories := map[string]*Category{
		"video_games": videoGames,
	}

	app := newTestRouter()

	categoriesResource := app.Resource("/categories")
	categoriesResource.Get("get-categories", "doc",
		NewResponse(http.StatusOK, "").Model([]CategorySummary{}).Headers("link"),
	).Run(func(ctx Context, input struct {
		Limit int `query:"limit" default:"10"`
	}) {
		summaries := []CategorySummary{}
		for _, cat := range categories {
			summaries = append(summaries, cat.CategorySummary)
		}
		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].ID < summaries[j].ID
		})
		if input.Limit == 0 {
			input.Limit = 10
		}
		if len(summaries) < input.Limit {
			input.Limit = len(summaries)
		}
		ctx.Header().Set("Link", "</categories>; rel=\"first\"")
		ctx.WriteModel(http.StatusOK, summaries[:input.Limit])
	})

	categoriesResource.Delete("delete-category", "doc",
		NewResponse(http.StatusNoContent, ""),
	).Run(func(ctx Context) {
		ctx.WriteHeader(http.StatusNoContent)
	})

	categoriesResource.SubResource("/{category-id}").Get("get-category", "doc",
		NewResponse(http.StatusOK, "").Model(&Category{}),
		NewResponse(http.StatusNotFound, "").Model(&ErrorModel{}),
	).Run(func(ctx Context, input struct {
		CategoryParam
	}) {
		if categories[input.CategoryID] == nil {
			ctx.WriteError(http.StatusNotFound, "Not found")
			return
		}
		ctx.WriteModel(http.StatusOK, categories[input.CategoryID])
	})

	app.Resource("/categories/{category-id}/products").Get("get-items", "doc",
		NewResponse(http.StatusOK, "").Model([]ProductSummary{}),
		NewResponse(http.StatusNotFound, "").Model(&ErrorModel{}),
	).Run(func(ctx Context, input struct {
		CategoryParam
	}) {
		if categories[input.CategoryID] == nil {
			ctx.WriteError(http.StatusNotFound, "Not found")
			return
		}
		summaries := []ProductSummary{}
		for _, item := range categories[input.CategoryID].products {
			summaries = append(summaries, item.ProductSummary)
		}
		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].ID < summaries[j].ID
		})
		ctx.WriteModel(http.StatusOK, summaries)
	})

	app.Resource("/categories/{category-id}/products/{product-id}").Get("get-item", "doc",
		NewResponse(http.StatusOK, "").Model(&Product{}),
		NewResponse(http.StatusNotFound, "").Model(&ErrorModel{}),
	).Run(func(ctx Context, input struct {
		CategoryParam
		ProductParam
	}) {
		if categories[input.CategoryID] == nil || categories[input.CategoryID].products[input.ProductID] == nil {
			ctx.WriteError(http.StatusNotFound, "Not found")
			return
		}
		ctx.WriteModel(http.StatusOK, categories[input.CategoryID].products[input.ProductID])
	})

	app.Resource("/categories/{category-id}/products/{product-id}/stores").Get("get-stores", "doc",
		NewResponse(http.StatusOK, "").Model([]StoreSummary{}),
		NewResponse(http.StatusNotFound, "").Model(&ErrorModel{}),
	).Run(func(ctx Context, input struct {
		CategoryParam
		ProductParam
	}) {
		if categories[input.CategoryID] == nil || categories[input.CategoryID].products[input.ProductID] == nil {
			ctx.WriteError(http.StatusNotFound, "Not found")
			return
		}
		summaries := []StoreSummary{}
		for _, store := range categories[input.CategoryID].products[input.ProductID].stores {
			summaries = append(summaries, store.StoreSummary)
		}
		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].ID < summaries[j].ID
		})
		ctx.WriteModel(http.StatusOK, summaries)
	})

	app.Resource("/categories/{category-id}/products/{product-id}/stores/{store-id}").Get("get-store", "doc",
		NewResponse(http.StatusOK, "").Model(&Store{}),
		NewResponse(http.StatusNotFound, "").Model(&ErrorModel{}),
	).Run(func(ctx Context, input struct {
		CategoryParam
		ProductParam
		StoreID string `path:"store-id" doc:"Store ID"`
	}) {
		if categories[input.CategoryID] == nil || categories[input.CategoryID].products[input.ProductID] == nil {
			ctx.WriteError(http.StatusNotFound, "Not found")
			return
		}
		ctx.WriteModel(http.StatusOK, categories[input.CategoryID].products[input.ProductID].stores[input.StoreID])
	})

	app.EnableGraphQL(&GraphQLConfig{
		ComplexityLimit: 250,
	})

	query := strings.Replace(strings.Replace(`{
		categories(limit: 1) {
			headers {
				link
			}
			edges {
				categoriesItem {
					id
					featured
					code
					products {
						edges {
							productsItem {
								id
								suggested_price
								created
								metadata{
									key
									value
								}
								empty {
									_
								}
								stores {
									edges {
										storesItem {
											id
											url
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}`, "\n", " ", -1), "\t", "", -1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/graphql?query="+query, nil)
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.YAMLEq(t, strings.Replace(`
data:
	categories:
		headers:
			link: </categories>; rel="first"
		edges:
			- categoriesItem:
					id: video_games
					featured: true
					code: aGk=
					products:
						edges:
							- productsItem:
									id: nintendo_switch
									suggested_price: 349.99
									created: null
									metadata: null
									empty: null
									stores:
										edges:
											- storesItem:
													id: target
													url: https://www.target.com/
							- productsItem:
									id: playstation_ps5
									suggested_price: 499.99
									created: "2022-02-22T22:22:22Z"
									metadata: null
									empty: null
									stores:
										edges:
											- storesItem:
													id: amazon
													url: https://www.amazon.com/
							- productsItem:
									id: xbox_series_x
									suggested_price: 499.99
									created: "2022-02-22T22:22:22Z"
									metadata:
										- key: foo
											value: bar
									empty:
										_: null
									stores:
										edges:
											- storesItem:
													id: amazon
													url: https://www.amazon.com/
											- storesItem:
													id: target
													url: https://www.target.com/
`, "\t", "  ", -1), w.Body.String())
}
