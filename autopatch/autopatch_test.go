package autopatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/gin-gonic/gin"
	"github.com/go-chi/chi/v5"
	fiberV2 "github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/gorilla/mux"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/danielgtaylor/huma/v2/adapters/humafiber"
	"github.com/danielgtaylor/huma/v2/adapters/humaflow"
	"github.com/danielgtaylor/huma/v2/adapters/humaflow/flow"
	"github.com/danielgtaylor/huma/v2/adapters/humagin"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/adapters/humamux"
	"github.com/danielgtaylor/huma/v2/casing"
	"github.com/danielgtaylor/huma/v2/humatest"
)

type SaleModel struct {
	Location string `json:"location"`
	Count    int    `json:"count"`
}

func (m SaleModel) String() string {
	return fmt.Sprintf("%s%d", m.Location, m.Count)
}

type ThingModel struct {
	ID      string      `json:"id"`
	Price   float32     `json:"price,omitempty"`
	Sales   []SaleModel `json:"sales,omitempty"`
	Tags    []string    `json:"tags,omitempty"`
	Comment *string     `json:"comment"`
}

func (m ThingModel) ETag() string {
	return fmt.Sprintf("%s%v%v%v", m.ID, m.Price, m.Sales, m.Tags)
}

type ThingIDParam struct {
	ThingID string `path:"thing-id"`
}

func TestPatch(t *testing.T) {
	db := map[string]*ThingModel{
		"test": {
			ID:    "test",
			Price: 1.00,
			Sales: []SaleModel{
				{Location: "US", Count: 123},
				{Location: "EU", Count: 456},
			},
		},
	}

	_, api := humatest.New(t)

	type GetThingResponse struct {
		ETag string `header:"ETag"`
		Body *ThingModel
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*GetThingResponse, error) {
		thing := db[input.ThingID]
		if thing == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		resp := &GetThingResponse{
			ETag: thing.ETag(),
			Body: thing,
		}
		return resp, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Errors:      []int{404, 412},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body    ThingModel
		IfMatch []string `header:"If-Match" doc:"Succeeds if the server's resource matches one of the passed values."`
	}) (*GetThingResponse, error) {
		if len(input.IfMatch) > 0 {
			found := false
			if existing := db[input.ThingID]; existing != nil {
				if slices.Contains(input.IfMatch, existing.ETag()) {
					found = true
				}
			}
			if !found {
				return nil, huma.Error412PreconditionFailed("ETag '" + strings.Join(input.IfMatch, ", ") + "' does not match")
			}
		} else {
			// Since the GET returns an ETag, and the auto-patch feature should always
			// use it when available, we can fail the test if we ever get here.
			t.Fatal("No If-Match header set during PUT")
		}
		db[input.ThingID] = &input.Body
		resp := &GetThingResponse{
			ETag: db[input.ThingID].ETag(),
			Body: db[input.ThingID],
		}
		return resp, nil
	})

	AutoPatch(api)

	w := api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"price": 1.23}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test1.23[US123 EU456][]", w.Result().Header.Get("ETag"))

	// Same change results in a 304 (patches are idempotent)
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"price": 1.23}`),
	)
	assert.Equal(t, http.StatusNotModified, w.Code, w.Body.String())

	// Extra headers should not be a problem, including `Accept`.
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		"Accept: application/json",
		"X-Some-Other: value",
		strings.NewReader(`{"price": 1.23}`),
	)
	assert.Equal(t, http.StatusNotModified, w.Code, w.Body.String())

	// New change but with wrong manual ETag, should fail!
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		"If-Match: abc123",
		strings.NewReader(`{"price": 4.56}`),
	)
	assert.Equal(t, http.StatusPreconditionFailed, w.Code, w.Body.String())

	// Correct manual ETag should pass!
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		"If-Match: test1.23[US123 EU456][]",
		strings.NewReader(`{"price": 4.56}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][]", w.Result().Header.Get("ETag"))

	// Merge Patch: invalid
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// JSON Patch Test
	w = api.Patch("/things/test",
		"Content-Type: application/json-patch+json",
		strings.NewReader(`[
			{"op": "add", "path": "/tags", "value": ["b"]},
			{"op": "add", "path": "/tags/0", "value": "a"}
		]`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][a b]", w.Result().Header.Get("ETag"))

	// JSON Patch: bad JSON
	w = api.Patch("/things/test",
		"Content-Type: application/json-patch+json",
		strings.NewReader(`[`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// JSON Patch: invalid patch
	w = api.Patch("/things/test",
		"Content-Type: application/json-patch+json",
		strings.NewReader(`[{"op": "unsupported"}]`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Shorthand Patch Test
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+shorthand",
		strings.NewReader(`{tags[]: c}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "test4.56[US123 EU456][a b c]", w.Result().Header.Get("ETag"))

	// Shorthand Patch: bad input
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+shorthand",
		strings.NewReader(`[`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// Bad content type
	w = api.Patch("/things/test",
		"Content-Type: application/unsupported-content-type",
		strings.NewReader(`{}`),
	)
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code, w.Body.String())

	// PATCH body read error
	w = api.Patch("/things/notfound",
		"Content-Type: application/merge-patch+json",
		iotest.ErrReader(errors.New("test error")),
	)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// GET error
	w = api.Patch("/things/notfound",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{}`),
	)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestPatchKeepsMultiValueHeaders(t *testing.T) {
	db := map[string]*ThingModel{
		"test": {ID: "test", Price: 1.00},
	}

	_, api := humatest.New(t)

	type ThingResponse struct {
		SetCookie []http.Cookie `header:"Set-Cookie"`
		Body      *ThingModel
	}

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*ThingResponse, error) {
		thing := db[input.ThingID]
		if thing == nil {
			return nil, huma.ErrorWithHeaders(huma.Error404NotFound("Not found"), http.Header{
				"Set-Cookie": {"c=3", "d=4"},
			})
		}
		return &ThingResponse{Body: thing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body ThingModel
	}) (*ThingResponse, error) {
		db[input.ThingID] = &input.Body
		return &ThingResponse{
			SetCookie: []http.Cookie{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}},
			Body:      db[input.ThingID],
		}, nil
	})

	AutoPatch(api)

	w := api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"price": 1.23}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"a=1", "b=2"}, w.Header().Values("Set-Cookie"))

	// GET error
	w = api.Patch("/things/notfound",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{}`),
	)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Equal(t, []string{"c=3", "d=4"}, w.Header().Values("Set-Cookie"))
}

func TestPatchPutNoBody(t *testing.T) {
	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		// Note: no body!
	}) (*struct{}, error) {
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		// Note: no body!
	}) (*struct{}, error) {
		return nil, nil
	})

	AutoPatch(api)

	// There should be no generated PATCH since there is nothing to
	// write in the PUT!
	assert.Nil(t, api.OpenAPI().Paths["/things/{thing-id}"].Patch)
	w := api.Patch("/things/test")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code, w.Body.String())
}

// TestPatchPrefix checks the internal GET/PUT reach their routes when the API
// is mounted under a prefix. Most adapters register the prefix on a top-level
// router that needs the full path, while chi and Fiber can also mount a
// sub-router or sub-app that only matches the path below its mount point.
func TestPatchPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		prefix string
		serve  func(config huma.Config, register func(huma.API)) http.Handler
	}{
		{
			name:   "chi mount with param",
			prefix: "/tenants/t1",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				r := chi.NewRouter()
				sub := chi.NewRouter()
				sub.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Before this router matches, the mount's param and only its
						// pattern are set, as on the original request.
						rctx := chi.RouteContext(r.Context())
						if chi.URLParam(r, "tenant") != "t1" || len(rctx.RoutePatterns) != 1 {
							w.WriteHeader(http.StatusForbidden)
							return
						}
						next.ServeHTTP(w, r)
					})
				})
				register(humachi.New(sub, config))
				r.Mount("/tenants/{tenant}", sub)
				return r
			},
		},
		{
			name:   "echo group",
			prefix: "/api",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				e := echo.New()
				register(humaecho.NewWithGroup(e, e.Group("/api"), config))
				return e
			},
		},
		{
			name:   "fiber group",
			prefix: "/api",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				app := fiber.New()
				api := humafiber.NewWithGroup(app, app.Group("/api"), config)
				register(api)
				// The adapter serves through the whole app, not just the group.
				return api.Adapter()
			},
		},
		{
			name:   "fiber group in mount",
			prefix: "/api/v1",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				app := fiber.New()
				sub := fiber.New()
				register(humafiber.NewWithGroup(sub, sub.Group("/v1/"), config))
				app.Use("/api", sub)
				return humafiber.New(app, config).Adapter()
			},
		},
		{
			name:   "fiber v2 group",
			prefix: "/api",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				app := fiberV2.New()
				api := humafiber.NewV2WithGroup(app, app.Group("/api"), config)
				register(api)
				return api.Adapter()
			},
		},
		{
			name:   "fiber v2 mount",
			prefix: "/api",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				app := fiberV2.New()
				sub := fiberV2.New()
				register(humafiber.NewV2(sub, config))
				app.Mount("/api", sub)
				return humafiber.NewV2(app, config).Adapter()
			},
		},
		{
			name:   "flow prefix",
			prefix: "/api",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				m := flow.New()
				register(humaflow.NewWithPrefix(m, "/api", config))
				return m
			},
		},
		{
			name:   "gin group",
			prefix: "/api",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				e := gin.New()
				register(humagin.NewWithGroup(e, e.Group("/api"), config))
				return e
			},
		},
		{
			name:   "gin extra slash",
			prefix: "/",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				e := gin.New()
				e.RemoveExtraSlash = true
				register(humagin.New(e, config))
				return e
			},
		},
		{
			name:   "go prefix",
			prefix: "/api/v1",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				m := http.NewServeMux()
				register(humago.NewWithPrefix(m, "/api/v1", config))
				return m
			},
		},
		{
			name:   "gorilla subrouter",
			prefix: "/api",
			serve: func(config huma.Config, register func(huma.API)) http.Handler {
				r := mux.NewRouter()
				register(humamux.New(r.PathPrefix("/api").Subrouter(), config))
				return r
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := map[string]*ThingModel{"test": {ID: "test", Price: 1.00}}

			type ThingResponse struct {
				Body *ThingModel
			}

			handler := test.serve(huma.DefaultConfig("Test", "1.0.0"), func(api huma.API) {
				// Fiber ends a route param name at "-", so {thing-id} can't be used here.
				huma.Get(api, "/things/{id}", func(ctx context.Context, input *struct {
					ID string `path:"id"`
				}) (*ThingResponse, error) {
					return &ThingResponse{Body: db[input.ID]}, nil
				})
				huma.Put(api, "/things/{id}", func(ctx context.Context, input *struct {
					ID   string `path:"id"`
					Body ThingModel
				}) (*ThingResponse, error) {
					db[input.ID] = &input.Body
					return &ThingResponse{Body: db[input.ID]}, nil
				})
				AutoPatch(api)
			})

			req := httptest.NewRequest(http.MethodPatch, test.prefix+"/things/test", strings.NewReader(`{"price": 1.23}`))
			req.Header.Set("Content-Type", "application/merge-patch+json")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.InDelta(t, 1.23, db["test"].Price, 0.001)
		})
	}
}

// TestPatchEscapedPathAndQuery checks the internal GET/PUT reach the same
// resource as the PATCH when its ID is escaped, and both get its query params
// except a GET-only one that would trim the body written back.
func TestPatchEscapedPathAndQuery(t *testing.T) {
	_, api := humatest.New(t)
	db := map[string]*ThingModel{"a": {ID: "a"}, "a?b/c": {ID: "a?b/c", Tags: []string{"keep"}}}

	type ThingResponse struct {
		Body *ThingModel
	}

	var getVersion int

	huma.Get(api, "/things/{id}", func(ctx context.Context, input *struct {
		ID      string `path:"id"`
		Version int    `query:"version"`
		Fields  string `query:"fields"`
	}) (*ThingResponse, error) {
		getVersion = input.Version
		if input.Fields == "id" {
			return &ThingResponse{Body: &ThingModel{ID: input.ID}}, nil
		}
		return &ThingResponse{Body: db[input.ID]}, nil
	})
	huma.Put(api, "/things/{id}", func(ctx context.Context, input *struct {
		ID      string `path:"id"`
		Version int    `query:"version" required:"true"`
		Org     string `query:"org" hidden:"true" required:"true"`
		Body    ThingModel
	}) (*ThingResponse, error) {
		db[input.ID] = &input.Body
		return &ThingResponse{Body: db[input.ID]}, nil
	})
	AutoPatch(api)

	w := api.Patch("/things/a%3Fb%2Fc?version=2&fields=id&org=o", "Content-Type: application/merge-patch+json", strings.NewReader(`{"price": 1.23}`))
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "a?b/c", db["a?b/c"].ID, "the GET read a different resource")
	assert.InDelta(t, 1.23, db["a?b/c"].Price, 0.001)
	assert.Zero(t, db["a"].Price)
	assert.Equal(t, 2, getVersion)
	assert.Equal(t, []string{"keep"}, db["a?b/c"].Tags, "a GET-only query param trimmed the body")
}

func TestExplicitDisable(t *testing.T) {
	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
		Metadata: map[string]any{
			"autopatch": false, //           <-- Disabled here!
		},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*struct{ Body struct{} }, error) {
		return nil, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Errors:      []int{404, 412},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body    ThingModel
		IfMatch []string `header:"If-Match" doc:"Succeeds if the server's resource matches one of the passed values."`
	}) (*struct{ Body struct{} }, error) {
		return nil, nil
	})

	AutoPatch(api)

	// There should be no generated PATCH since there is nothing to
	// write in the PUT!
	assert.Nil(t, api.OpenAPI().Paths["/things/{thing-id}"].Patch)
	w := api.Patch("/things/test")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code, w.Body.String())
}

func TestPatchRecursiveNode(t *testing.T) {
	type Node struct {
		Name     string  `json:"name"`
		Children []*Node `json:"children,omitempty"`
	}

	db := map[string]*Node{
		"root": {Name: "root", Children: []*Node{{Name: "child"}}},
	}

	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-node",
		Method:      http.MethodGet,
		Path:        "/nodes/{id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct {
		Body *Node
	}, error) {
		node := db[input.ID]
		if node == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		return &struct{ Body *Node }{Body: node}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-node",
		Method:      http.MethodPut,
		Path:        "/nodes/{id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body Node
	}) (*struct {
		Body *Node
	}, error) {
		node := input.Body
		db[input.ID] = &node
		return &struct{ Body *Node }{Body: db[input.ID]}, nil
	})

	AutoPatch(api)

	path := api.OpenAPI().Paths["/nodes/{id}"]
	require.NotNil(t, path)
	require.NotNil(t, path.Patch)

	schema := path.Patch.RequestBody.Content["application/merge-patch+json"].Schema
	assert.Empty(t, schema.Required)
	assert.Equal(t, "string", schema.Properties["name"].Type)
	require.NotNil(t, schema.Properties["children"].Items)
	assert.Equal(t, "#/components/schemas/Node", schema.Properties["children"].Items.Ref)

	w := api.Patch("/nodes/root",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"name": "updated"}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"name":"updated"`)
}

func TestPatchSchemaNestedRef(t *testing.T) {
	type Inner struct {
		Val    *string `json:"val" nullable:"true"`
		Secret string  `json:"secret" hidden:"true"`
	}
	type Outer struct {
		Billing Inner `json:"billing" doc:"Billing info" readOnly:"true"`
		Private Inner `json:"private" hidden:"true"`
	}

	_, api := humatest.New(t)
	huma.Get(api, "/outer", func(ctx context.Context, input *struct{}) (*struct{ Body Outer }, error) {
		return &struct{ Body Outer }{}, nil
	})
	huma.Put(api, "/outer", func(ctx context.Context, input *struct{ Body Outer }) (*struct{ Body Outer }, error) {
		return &struct{ Body Outer }{}, nil
	})
	AutoPatch(api)

	schema := api.OpenAPI().Paths["/outer"].Patch.RequestBody.Content["application/merge-patch+json"].Schema
	billing := schema.Properties["billing"]
	require.NotNil(t, billing)
	assert.Empty(t, billing.Ref)
	assert.Equal(t, "Billing info", billing.Description)
	assert.True(t, billing.ReadOnly)
	assert.True(t, billing.Properties["val"].Nullable)

	b, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "secret")
	assert.NotContains(t, string(b), "private")
}

func TestDeprecatedPatch(t *testing.T) {
	_, api := humatest.New(t)

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*struct {
		Body *ThingModel
	}, error) {
		return &struct{ Body *ThingModel }{&ThingModel{}}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Deprecated:  true,
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body ThingModel
	}) (*struct {
		Body *ThingModel
	}, error) {
		return &struct{ Body *ThingModel }{&ThingModel{}}, nil
	})

	AutoPatch(api)

	assert.True(t, api.OpenAPI().Paths["/things/{thing-id}"].Patch.Deprecated)
}

func TestPatchOperationID(t *testing.T) {
	type ThingOutput struct {
		Body *ThingModel
	}
	type PutThingInput struct {
		ThingIDParam
		Body ThingModel
	}

	getThing := func(ctx context.Context, input *ThingIDParam) (*ThingOutput, error) {
		return &ThingOutput{Body: &ThingModel{}}, nil
	}
	putThing := func(ctx context.Context, input *PutThingInput) (*ThingOutput, error) {
		return &ThingOutput{Body: &ThingModel{}}, nil
	}

	t.Run("explicit", func(t *testing.T) {
		_, api := humatest.New(t)
		huma.Register(api, huma.Operation{
			OperationID: "fetch-thing",
			Method:      http.MethodGet,
			Path:        "/v1/things/{thing-id}",
		}, getThing)
		huma.Register(api, huma.Operation{
			OperationID: "put-thing",
			Method:      http.MethodPut,
			Path:        "/v1/things/{thing-id}",
		}, putThing)

		AutoPatch(api)

		assert.Equal(t, "patch-thing", api.OpenAPI().Paths["/v1/things/{thing-id}"].Patch.OperationID)
	})

	t.Run("convenience", func(t *testing.T) {
		_, api := humatest.New(t)
		huma.Get(api, "/v1/things/{thing-id}", getThing)
		huma.Put(api, "/v1/things/{thing-id}", putThing)

		AutoPatch(api)

		path := api.OpenAPI().Paths["/v1/things/{thing-id}"]
		assert.Equal(t, "get-v1-things-by-thing-id", path.Get.OperationID)
		assert.Equal(t, "patch-v1-things-by-thing-id", path.Patch.OperationID)
	})

	t.Run("convenience with overridden ID", func(t *testing.T) {
		_, api := humatest.New(t)
		huma.Get(api, "/things/{thing-id}", getThing, func(o *huma.Operation) {
			o.OperationID = "fetch-thing"
		})
		huma.Put(api, "/things/{thing-id}", putThing)

		AutoPatch(api)

		assert.Equal(t, "patch-thing", api.OpenAPI().Paths["/things/{thing-id}"].Patch.OperationID)
	})

	t.Run("custom generator", func(t *testing.T) {
		orig := huma.GenerateOperationID
		t.Cleanup(func() { huma.GenerateOperationID = orig })
		huma.GenerateOperationID = func(method, path string, response any) string {
			return casing.LowerCamel(orig(method, path, response))
		}

		_, api := humatest.New(t)
		grp := huma.NewGroup(api, "/v1")
		huma.Get(grp, "/things/{thing-id}", getThing)
		huma.Put(grp, "/things/{thing-id}", putThing)

		AutoPatch(api)

		path := api.OpenAPI().Paths["/v1/things/{thing-id}"]
		assert.Equal(t, "getV1ThingsByThingId", path.Get.OperationID)
		assert.Equal(t, "patchV1ThingsByThingId", path.Patch.OperationID)
	})
}

func TestNullabilityExtension(t *testing.T) {
	comment := "comment1"
	db := map[string]*ThingModel{
		"test": {
			ID:    "test",
			Price: 1.00,
			Sales: []SaleModel{
				{Location: "US", Count: 123},
			},
			Tags:    []string{"tag1", "tag2"},
			Comment: &comment,
		},
	}

	_, api := humatest.New(t)

	// Register the nullability extension
	RegisterNullabilityExtension(api, "__NULL__")

	huma.Register(api, huma.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
	}) (*struct {
		Body *ThingModel
	}, error) {
		thing := db[input.ThingID]
		if thing == nil {
			return nil, huma.Error404NotFound("Not found")
		}
		return &struct{ Body *ThingModel }{thing}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "put-thing",
		Method:      http.MethodPut,
		Path:        "/things/{thing-id}",
		Errors:      []int{404},
	}, func(ctx context.Context, input *struct {
		ThingIDParam
		Body ThingModel
	}) (*struct {
		Body *ThingModel
	}, error) {
		db[input.ThingID] = &input.Body
		return &struct{ Body *ThingModel }{&input.Body}, nil
	})

	AutoPatch(api)

	// Test setting an array field to null using the string representation
	w := api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"tags": "__NULL__"}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Verify the field was actually set to null; expect it to be omitted
	w = api.Get("/things/test")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), `"tags"`)

	// Test setting a nullable string field to null
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{"comment": null}`),
	)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Verify the nested field was set to null
	w = api.Get("/things/test")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"comment":null`)

	// Test error handling for invalid JSON
	w = api.Patch("/things/test",
		"Content-Type: application/merge-patch+json",
		strings.NewReader(`{invalid`),
	)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func testRegistry() huma.Registry {
	return huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
}

func testMakeOptionalSchema(registry huma.Registry, s *huma.Schema) *huma.Schema {
	return makeOptionalSchema(registry, s, map[string]struct{}{})
}

func TestMakeOptionalSchemaBasicProperties(t *testing.T) {
	originalSchema := &huma.Schema{
		Type: "object",
		Properties: map[string]*huma.Schema{
			"id":      {Type: "string"},
			"name":    {Type: "string"},
			"version": {Type: "string", Const: "v1"},
		},
		Required: []string{"id", "name"},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Equal(t, "object", optionalSchema.Type)
	assert.Contains(t, optionalSchema.Properties, "id")
	assert.Contains(t, optionalSchema.Properties, "name")
	assert.Equal(t, "v1", optionalSchema.Properties["version"].Const)
	assert.Empty(t, optionalSchema.Required)
}

func TestMakeOptionalSchemaOneOf(t *testing.T) {
	originalSchema := &huma.Schema{
		OneOf: []*huma.Schema{
			{Type: "string"},
			{Type: "number"},
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Len(t, optionalSchema.OneOf, 2)
	assert.Equal(t, "string", optionalSchema.OneOf[0].Type)
	assert.Equal(t, "number", optionalSchema.OneOf[1].Type)
}

func TestMakeOptionalSchemaAnyOf(t *testing.T) {
	originalSchema := &huma.Schema{
		AnyOf: []*huma.Schema{
			{Type: "string"},
			{Type: "number"},
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Len(t, optionalSchema.AnyOf, 2)
	assert.Equal(t, "string", optionalSchema.AnyOf[0].Type)
	assert.Equal(t, "number", optionalSchema.AnyOf[1].Type)
}

func TestMakeOptionalSchemaAllOf(t *testing.T) {
	minLength := 1
	maxLength := 100
	originalSchema := &huma.Schema{
		AllOf: []*huma.Schema{
			{MinLength: &minLength},
			{MaxLength: &maxLength},
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.Len(t, optionalSchema.AllOf, 2)
	assert.Equal(t, 1, *optionalSchema.AllOf[0].MinLength)
	assert.Equal(t, 100, *optionalSchema.AllOf[1].MaxLength)
}

func TestMakeOptionalSchemaNot(t *testing.T) {
	originalSchema := &huma.Schema{
		Not: &huma.Schema{
			Type: "null",
		},
	}

	optionalSchema := testMakeOptionalSchema(testRegistry(), originalSchema)

	assert.NotNil(t, optionalSchema.Not)
	assert.Equal(t, "null", optionalSchema.Not.Type)
}

func TestMakeOptionalSchemaNilInput(t *testing.T) {
	assert.Nil(t, testMakeOptionalSchema(testRegistry(), nil))
}

func TestReplaceNulls(t *testing.T) {
	settings := MergePatchNullabilitySettings{
		Enabled:                    true,
		StringRepresentationOfNull: "__NULL__",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple null",
			input:    `{"name": null}`,
			expected: `{"name":"__NULL__"}`,
		},
		{
			name:     "nested null",
			input:    `{"user": {"name": null, "active": true}}`,
			expected: `{"user":{"active":true,"name":"__NULL__"}}`,
		},
		{
			name:     "array with null",
			input:    `{"tags": ["a", null, "c"]}`,
			expected: `{"tags":["a","__NULL__","c"]}`,
		},
		{
			name:     "no nulls",
			input:    `{"name": "test", "count": 42}`,
			expected: `{"count":42,"name":"test"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := replaceNulls([]byte(tc.input), settings)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(result))
		})
	}

	// Test error case
	_, err := replaceNulls([]byte(`{invalid json`), settings)
	assert.Error(t, err)
}

func TestRestoreNulls(t *testing.T) {
	settings := MergePatchNullabilitySettings{
		Enabled:                    true,
		StringRepresentationOfNull: "__NULL__",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple null string",
			input:    `{"name": "__NULL__"}`,
			expected: `{"name":null}`,
		},
		{
			name:     "nested null string",
			input:    `{"user": {"name": "__NULL__", "active": true}}`,
			expected: `{"user":{"name":null,"active":true}}`,
		},
		{
			name:     "array with null string",
			input:    `{"tags": ["a", "__NULL__", "c"]}`,
			expected: `{"tags":["a",null,"c"]}`,
		},
		{
			name:     "no null strings",
			input:    `{"name": "test", "count": 42}`,
			expected: `{"name":"test","count":42}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := restoreNulls([]byte(tc.input), settings)
			require.NoError(t, err)
			assert.JSONEq(t, tc.expected, string(result))
		})
	}

	// Test error case
	_, err := restoreNulls([]byte(`{invalid json`), settings)
	assert.Error(t, err)
}

func TestMakeOptionalSchemaNestedSchemas(t *testing.T) {
	nestedSchema := &huma.Schema{
		Type: "object",
		Properties: map[string]*huma.Schema{
			"nested": {
				Type: "object",
				Properties: map[string]*huma.Schema{
					"deeplyNested": {Type: "string"},
				},
				Required: []string{"deeplyNested"},
			},
		},
		Required: []string{"nested"},
	}

	optionalNestedSchema := testMakeOptionalSchema(testRegistry(), nestedSchema)

	assert.Empty(t, optionalNestedSchema.Required)
	assert.Empty(t, optionalNestedSchema.Properties["nested"].Required)
}

func TestMakeOptionalSchemaRefs(t *testing.T) {
	const addressRef = "#/components/schemas/Address"
	const tagRef = "#/components/schemas/Tag"

	registry := testRegistry()
	registry.Map()["Address"] = &huma.Schema{
		Type: "object",
		Properties: map[string]*huma.Schema{
			"street": {Type: "string"},
			"city":   {Type: "string"},
		},
		Required: []string{"street", "city"},
	}
	registry.Map()["Tag"] = &huma.Schema{Type: "string"}

	t.Run("top-level ref", func(t *testing.T) {
		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: addressRef})

		assert.Empty(t, optional.Ref)
		assert.Equal(t, "object", optional.Type)
		assert.Empty(t, optional.Required)
		assert.Len(t, optional.Properties, 2)
		assert.Equal(t, "string", optional.Properties["street"].Type)
		assert.Equal(t, "string", optional.Properties["city"].Type)
	})

	t.Run("property ref", func(t *testing.T) {
		parent := &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"address": {Ref: addressRef},
			},
			Required: []string{"address"},
		}

		optional := testMakeOptionalSchema(registry, parent)

		assert.Empty(t, optional.Required)
		address := optional.Properties["address"]
		require.NotNil(t, address)
		assert.Empty(t, address.Ref)
		assert.Equal(t, "object", address.Type)
		assert.Empty(t, address.Required)
	})

	t.Run("items ref", func(t *testing.T) {
		parent := &huma.Schema{
			Type:  "array",
			Items: &huma.Schema{Ref: tagRef},
		}

		optional := testMakeOptionalSchema(registry, parent)

		require.NotNil(t, optional.Items)
		assert.Empty(t, optional.Items.Ref)
		assert.Equal(t, "string", optional.Items.Type)
	})

	t.Run("ref to a ref alias", func(t *testing.T) {
		registry := testRegistry()
		registry.Map()["Address"] = &huma.Schema{
			Type:       "object",
			Properties: map[string]*huma.Schema{"street": {Type: "string"}},
			Required:   []string{"street"},
		}
		registry.Map()["Alias"] = &huma.Schema{Ref: addressRef}

		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: "#/components/schemas/Alias", Description: "Home"})

		assert.Empty(t, optional.Ref)
		assert.Equal(t, "object", optional.Type)
		assert.Equal(t, "Home", optional.Description)
		assert.Empty(t, optional.Required)
	})

	t.Run("unresolvable external ref", func(t *testing.T) {
		const externalRef = "https://example.com/schemas/Thing.json"

		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: externalRef})

		require.NotNil(t, optional)
		assert.Equal(t, externalRef, optional.Ref)
		assert.Empty(t, optional.Type)
	})

	t.Run("recursive self reference", func(t *testing.T) {
		const nodeRef = "#/components/schemas/Node"

		registry := testRegistry()
		registry.Map()["Node"] = &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"name": {Type: "string"},
				"children": {
					Type:  "array",
					Items: &huma.Schema{Ref: nodeRef, Description: "Child", ReadOnly: true},
				},
			},
			Required: []string{"name"},
		}

		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: nodeRef})

		assert.Empty(t, optional.Ref)
		assert.Equal(t, "object", optional.Type)
		assert.Empty(t, optional.Required)
		assert.Equal(t, "string", optional.Properties["name"].Type)
		assert.Equal(t, "array", optional.Properties["children"].Type)
		require.NotNil(t, optional.Properties["children"].Items)
		assert.Equal(t, nodeRef, optional.Properties["children"].Items.Ref)
		assert.Equal(t, "Child", optional.Properties["children"].Items.Description)
		assert.True(t, optional.Properties["children"].Items.ReadOnly)
	})
}

func TestMakeOptionalSchemaAdditionalProperties(t *testing.T) {
	t.Run("boolean additionalProperties", func(t *testing.T) {
		for _, val := range []bool{true, false} {
			original := &huma.Schema{
				Type: "object",
				Properties: map[string]*huma.Schema{
					"name": {Type: "string"},
				},
				Required:             []string{"name"},
				AdditionalProperties: val,
			}

			optional := testMakeOptionalSchema(testRegistry(), original)

			assert.Equal(t, val, optional.AdditionalProperties)
			assert.Empty(t, optional.Required)
		}
	})

	t.Run("inline schema additionalProperties", func(t *testing.T) {
		original := &huma.Schema{
			Type: "object",
			AdditionalProperties: &huma.Schema{
				Type: "object",
				Properties: map[string]*huma.Schema{
					"value": {Type: "string"},
				},
				Required: []string{"value"},
			},
		}

		optional := testMakeOptionalSchema(testRegistry(), original)

		addl, ok := optional.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Equal(t, "object", addl.Type)
		assert.Empty(t, addl.Required)
		assert.Equal(t, "string", addl.Properties["value"].Type)
	})

	t.Run("ref additionalProperties", func(t *testing.T) {
		const tagRef = "#/components/schemas/Tag"

		registry := testRegistry()
		registry.Map()["Tag"] = &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"label": {Type: "string"},
			},
			Required: []string{"label"},
		}

		original := &huma.Schema{
			Type:                 "object",
			AdditionalProperties: &huma.Schema{Ref: tagRef},
		}

		optional := testMakeOptionalSchema(registry, original)

		addl, ok := optional.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Empty(t, addl.Ref)
		assert.Equal(t, "object", addl.Type)
		assert.Empty(t, addl.Required)
		assert.Equal(t, "string", addl.Properties["label"].Type)
	})

	t.Run("recursive ref additionalProperties", func(t *testing.T) {
		const nodeRef = "#/components/schemas/Node"

		registry := testRegistry()
		registry.Map()["Node"] = &huma.Schema{
			Type: "object",
			Properties: map[string]*huma.Schema{
				"name": {Type: "string"},
				"named": {
					Type:                 "object",
					AdditionalProperties: &huma.Schema{Ref: nodeRef},
				},
			},
			Required: []string{"name"},
		}

		optional := testMakeOptionalSchema(registry, &huma.Schema{Ref: nodeRef})

		assert.Empty(t, optional.Required)
		named := optional.Properties["named"]
		require.NotNil(t, named)
		addl, ok := named.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Equal(t, nodeRef, addl.Ref)
	})

	t.Run("unresolvable ref additionalProperties", func(t *testing.T) {
		const externalRef = "https://example.com/schemas/Thing.json"

		original := &huma.Schema{
			Type:                 "object",
			AdditionalProperties: &huma.Schema{Ref: externalRef},
		}

		optional := testMakeOptionalSchema(testRegistry(), original)

		addl, ok := optional.AdditionalProperties.(*huma.Schema)
		require.True(t, ok)
		assert.Equal(t, externalRef, addl.Ref)
	})
}

// routedPathAdapter gives every request a context reporting path as its routed
// path, as an adapter mounted under a prefix would.
type routedPathAdapter struct {
	huma.Adapter
	path string
}

func (a routedPathAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.Adapter.Handle(op, func(ctx huma.Context) {
		handler(routedPathContext{ctx, a.path})
	})
}

// routedPathContext embeds the context through an alias, so the field doesn't
// shadow the Context method.
type (
	humaContext       = huma.Context
	routedPathContext struct {
		humaContext
		path string
	}
)

func (c routedPathContext) RoutedPath() string { return c.path }

// TestPatchBadRoutedPath checks a routed path that can't be parsed is rejected
// rather than sent on.
func TestPatchBadRoutedPath(t *testing.T) {
	api := humatest.Wrap(t, huma.NewAPI(huma.DefaultConfig("Test", "1.0.0"), routedPathAdapter{humatest.NewAdapter(), "/things/%zz"}))

	type ThingResponse struct {
		Body *ThingModel
	}

	huma.Get(api, "/things/{id}", func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*ThingResponse, error) {
		return &ThingResponse{Body: &ThingModel{ID: input.ID}}, nil
	})
	huma.Put(api, "/things/{id}", func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body ThingModel
	}) (*ThingResponse, error) {
		return &ThingResponse{Body: &input.Body}, nil
	})
	AutoPatch(api)

	w := api.Patch("/things/test", "Content-Type: application/merge-patch+json", strings.NewReader(`{"price": 1.23}`))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestPatchFiberWildcardInMount checks routes under a mounted Fiber sub-app
// that a path's segment count can't map back to the sub-app's path: a wildcard
// matching several segments, and the sub-app's root at the mount path itself.
func TestPatchFiberWildcardInMount(t *testing.T) {
	app := fiber.New()
	sub := fiber.New()
	api := humafiber.New(sub, huma.DefaultConfig("Test", "1.0.0"))
	thing := &ThingModel{ID: "test", Price: 1.00}

	type ThingResponse struct {
		Body *ThingModel
	}

	huma.Get(api, "/files/*", func(ctx context.Context, input *struct{}) (*ThingResponse, error) {
		return &ThingResponse{Body: thing}, nil
	})
	huma.Put(api, "/files/*", func(ctx context.Context, input *struct{ Body ThingModel }) (*ThingResponse, error) {
		thing = &input.Body
		return &ThingResponse{Body: thing}, nil
	})
	huma.Get(api, "/", func(ctx context.Context, input *struct{}) (*ThingResponse, error) {
		return &ThingResponse{Body: thing}, nil
	})
	huma.Put(api, "/", func(ctx context.Context, input *struct{ Body ThingModel }) (*ThingResponse, error) {
		thing = &input.Body
		return &ThingResponse{Body: thing}, nil
	})
	AutoPatch(api)
	app.Use("/api", sub)
	handler := humafiber.New(app, huma.DefaultConfig("Test", "1.0.0")).Adapter()

	for i, target := range []string{"/api/files/a/b", "/api"} {
		req := httptest.NewRequest(http.MethodPatch, target, strings.NewReader(fmt.Sprintf(`{"price": %d}`, i+2)))
		req.Header.Set("Content-Type", "application/merge-patch+json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code, target, w.Body.String())
		assert.InDelta(t, float32(i+2), thing.Price, 0.001, target)
	}
}

func TestResourceQuery(t *testing.T) {
	get := &huma.Operation{Parameters: []*huma.Param{
		{Name: "version", In: "query"},
		{Name: "fields", In: "query"},
		{Name: "filter", In: "query", Style: "deepObject"},
	}}
	put := &huma.Operation{Parameters: []*huma.Param{
		{Name: "version", In: "query"},
	}}

	for _, test := range []struct {
		name  string
		query string
		want  string
	}{
		{"get only deep object", "filter%5Ba%5D=1&filter[b]=2&version=2", "version=2"},
		{"name prefix", "fieldset=a", "fieldset=a"},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, resourceQuery(test.query, get, put))
		})
	}
}
