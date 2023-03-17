package huma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/schema"
	"github.com/stretchr/testify/assert"
)

func newTestRouter() *Router {
	app := New("Test API", "1.0.0")
	return app
}

func TestRouterServiceLink(t *testing.T) {
	r := newTestRouter()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)
	assert.Contains(t, w.Header().Get("Link"), `</openapi.json>; rel="service-desc"`)
	assert.Contains(t, w.Header().Get("Link"), `</docs>; rel="service-doc"`)
}

func TestRouterHello(t *testing.T) {
	r := New("Test", "1.0.0")
	r.Resource("/test").Get("test", "Test",
		NewResponse(http.StatusNoContent, "test"),
	).Run(func(ctx Context) {
		ctx.WriteHeader(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	// Assert the response is as expected.
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestStreamingInput(t *testing.T) {
	r := New("Test", "1.0.0")
	r.Resource("/stream").Post("stream", "Stream test",
		NewResponse(http.StatusNoContent, "test"),
		NewResponse(http.StatusInternalServerError, "error"),
	).Run(func(ctx Context, input struct {
		Body io.Reader
	}) {
		_, err := ioutil.ReadAll(input.Body)
		if err != nil {
			ctx.WriteError(http.StatusInternalServerError, "Problem reading input", err)
		}

		ctx.WriteHeader(http.StatusNoContent)
	})

	stream := r.GetOperation("stream")
	assert.NotNil(t, stream)
	assert.Equal(t, *stream, OperationInfo{
		ID:          "stream",
		URITemplate: "/stream",
		Summary:     "Stream test",
		Tags:        []string{},
	})

	w := httptest.NewRecorder()
	body := bytes.NewReader(make([]byte, 1024))
	req, _ := http.NewRequest(http.MethodPost, "/stream", body)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestModelInputOutput(t *testing.T) {
	type Response struct {
		Category string `json:"category"`
		Hidden   bool   `json:"hidden"`
		Auth     string `json:"auth"`
		ID       string `json:"id"`
		Age      int    `json:"age"`
	}

	r := New("Test", "1.0.0")
	r.Resource("/players/{category}").Post("player", "Create player",
		NewResponse(http.StatusOK, "test").Model(Response{}),
	).Run(func(ctx Context, input struct {
		Category string `path:"category"`
		Hidden   bool   `query:"hidden"`
		Auth     string `header:"Authorization"`
		Body     struct {
			ID  string `json:"id"`
			Age int    `json:"age" minimum:"16"`
		}
	}) {
		ctx.WriteModel(http.StatusOK, Response{
			Category: input.Category,
			Hidden:   input.Hidden,
			Auth:     input.Auth,
			ID:       input.Body.ID,
			Age:      input.Body.Age,
		})
	})

	w := httptest.NewRecorder()
	body := bytes.NewReader([]byte(`{"id": "abc123", "age": 25}`))
	req, _ := http.NewRequest(http.MethodPost, "/players/fps?hidden=true", body)
	req.Header.Set("Authorization", "dummy")
	req.Host = "example.com"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{
			"$schema": "https://example.com/schemas/Response.json",
			"category": "fps",
			"hidden": true,
			"auth": "dummy",
			"id": "abc123",
			"age": 25
		}`, w.Body.String())

	// Should be able to get OpenAPI describing this API with its resource,
	// operation, schema, etc.
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/openapi.json", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Should be able to get model referenced by the response.
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/schemas/Response.json", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Missing schema should return 404
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/schemas/Missing.json", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRouterEmbeddedStructOutput(t *testing.T) {
	type CreatedField struct {
		Created time.Time `json:"created,omitempty"`
	}

	type Resp struct {
		CreatedField
		Another string `json:"another"`
		Ignored string `json:"-"`
	}

	now := time.Now()

	r := New("Test", "1.0.0")
	r.Resource("/test").Get("test", "Test",
		NewResponse(http.StatusOK, "test").Model(&Resp{}),
	).Run(func(ctx Context) {
		ctx.WriteModel(http.StatusOK, &Resp{
			CreatedField: CreatedField{
				Created: now,
			},
			Another: "foo",
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/test", nil)
	req.Host = "example.com"
	r.ServeHTTP(w, req)

	// Assert the response is as expected.
	assert.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, fmt.Sprintf(`{
		"$schema": "https://example.com/schemas/Resp.json",
		"created": "%s",
		"another": "foo"
	}`, now.Format(time.RFC3339Nano)), w.Body.String())
}

func TestTooBigBody(t *testing.T) {
	app := newTestRouter()

	type Input struct {
		Body struct {
			ID string `json:"id"`
		}
	}

	op := app.Resource("/test").Put("put", "desc",
		NewResponse(http.StatusNoContent, "desc"),
	)
	op.MaxBodyBytes(5)
	op.Run(func(ctx Context, input Input) {
		// Do nothing...
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{"id": "foo"}`))
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), "Request body too large")

	// With content length
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{"id": "foo"}`))
	req.Header.Set("Content-Length", "13")
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), "Request body too large")
}

type timeoutError struct{}

func (e *timeoutError) Error() string {
	return "timed out"
}

func (e *timeoutError) Timeout() bool {
	return true
}

func (e *timeoutError) Temporary() bool {
	return false
}

type slowReader struct{}

func (r *slowReader) Read(p []byte) (int, error) {
	return 0, &timeoutError{}
}

func TestBodySlow(t *testing.T) {
	app := newTestRouter()

	type Input struct {
		Body struct {
			ID string
		}
	}

	op := app.Resource("/test").Put("put", "desc",
		NewResponse(http.StatusNoContent, "desc"),
	)
	op.BodyReadTimeout(1 * time.Millisecond)
	op.Run(func(ctx Context, input Input) {
		// Do nothing...
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/test", &slowReader{})
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestTimeout, w.Code)
	assert.Contains(t, w.Body.String(), "timed out")
}

func TestErrorHandlers(t *testing.T) {
	app := newTestRouter()

	app.Resource("/").Get("root", "desc",
		NewResponse(http.StatusNoContent, "desc"),
	).Run(func(ctx Context) {
		// Do nothing
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/notfound", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "/notfound")

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/", nil)
	app.ServeHTTP(w, req)
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "PUT")
}

func TestInvalidPathParam(t *testing.T) {
	type Input struct {
		ThingID string `path:"thing-if"`
	}

	app := newTestRouter()

	// The router has no middleware, so no panic recovery will happen. This lets
	// us test via a simple assertion that it would panic, and the actual test
	// to ensure a 5xx error happens in the `middleware` package instead.
	assert.Panics(t, func() {
		app.Resource("/things/{thing-id}").Get("get", "Test",
			NewResponse(http.StatusNoContent, "desc"),
		).Run(func(ctx Context, input Input) {
			// Do nothing
		})
	})
}

func TestRouterSecurity(t *testing.T) {
	app := newTestRouter()

	// Document that the API gateway handles auth via OAuth2 Authorization Code.
	app.GatewayAuthCode("default", "https://example.com/authorize", "https://example.com/token", nil)
	app.GatewayClientCredentials("m2m", "https://example.com/token", nil)
	app.GatewayBasicAuth("basic")
	app.GatewayBearerFormat("bearer", "JWT bearer description", "JWT")
	app.GatewayAPIKey("apikey", "api key", "key", keyInHeader)
	app.GatewayOpenIDConnect("openid", "Open ID", "https://example.com/OpenID")

	// Every call must be authenticated using the default auth mechanism
	// registered above.
	app.SecurityRequirement("default")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/openapi.json", nil)
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var parsed map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	assert.Nil(t, err)

	assert.Equal(t, parsed["security"], []interface{}{
		map[string]interface{}{
			"default": []interface{}{},
		}})

	assert.Equal(t, parsed["components"].(map[string]interface{})["securitySchemes"], map[string]interface{}{
		"default": map[string]interface{}{
			"type": "oauth2",
			"flows": map[string]interface{}{
				"authorizationCode": map[string]interface{}{
					"authorizationUrl": "https://example.com/authorize",
					"tokenUrl":         "https://example.com/token",
				},
			},
		},
		"m2m": map[string]interface{}{
			"type": "oauth2",
			"flows": map[string]interface{}{
				"clientCredentials": map[string]interface{}{
					"tokenUrl": "https://example.com/token",
				},
			},
		},
		"basic": map[string]interface{}{
			"type":   "http",
			"scheme": "basic",
		},
		"bearer": map[string]interface{}{
			"type":         "http",
			"scheme":       "bearer",
			"description":  "JWT bearer description",
			"bearerFormat": "JWT",
		},
		"apikey": map[string]interface{}{
			"type":        "apiKey",
			"name":        "key",
			"in":          "header",
			"description": "api key",
		},
		"openid": map[string]interface{}{
			"type":             "openIdConnect",
			"description":      "Open ID",
			"openIdConnectUrl": "https://example.com/OpenID",
		},
	})
}

// TODO: test app.AutoConfig
func TestRouterAutoConfig(t *testing.T) {
	app := newTestRouter()

	app.GatewayAuthCode("authcode", "https://example.com/authorize", "https://example.com/token", nil)
	app.SecurityRequirement("authcode")

	app.AutoConfig(AutoConfig{
		Security: "authcode",
		Prompt: map[string]AutoConfigVar{
			"extra": {
				Description: "Some extra value",
				Example:     "abc123",
			},
		},
		Params: map[string]string{
			"another": "https://example.com/extras/{extra}",
		},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/openapi.json", nil)
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var parsed map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &parsed)
	assert.Nil(t, err)

	assert.Equal(t, parsed["x-cli-config"], map[string]interface{}{
		"security": "authcode",
		"prompt": map[string]interface{}{
			"extra": map[string]interface{}{
				"description": "Some extra value",
				"example":     "abc123",
			},
		},
		"params": map[string]interface{}{
			"another": "https://example.com/extras/{extra}",
		},
	})
}

func TestCustomRequestSchema(t *testing.T) {
	app := newTestRouter()

	op := app.Resource("/foo").Post("id", "doc", NewResponse(http.StatusOK, "ok"))
	op.RequestSchema(&schema.Schema{
		Type:    schema.TypeInteger,
		Minimum: schema.F(0),
		Maximum: schema.F(100),
	})
	op.Run(func(ctx Context, input struct {
		Body int
	}) {
		// Custom schema should be used so we never get here.
		ctx.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/foo", strings.NewReader("1234"))
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnprocessableEntity, w.Result().StatusCode)
}

func TestGetOperationName(t *testing.T) {
	app := newTestRouter()

	var opInfo *OperationInfo
	app.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)

			opInfo = GetOperationInfo(r.Context())
		})
	})

	app.Resource("/").Get("test-id", "doc",
		NewResponse(http.StatusOK, "ok"),
	).Run(func(ctx Context) {
		// Do nothing!
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Equal(t, "test-id", opInfo.ID)
	assert.Equal(t, "/", opInfo.URITemplate)
	assert.Equal(t, "doc", opInfo.Summary)
	assert.Equal(t, []string{}, opInfo.Tags)
}

func TestGetOperationDoesNotCrash(t *testing.T) {
	ctx := context.Background()
	assert.NotPanics(t, func() {
		info := GetOperationInfo(ctx)
		assert.NotNil(t, info)
	})
}

func TestSubResource(t *testing.T) {
	app := newTestRouter()

	// This should not crash.
	app.Resource("/").SubResource("/foo").SubResource("/bar").Get("get-bar", "docs", NewResponse(http.StatusOK, "ok")).Run(func(ctx Context) {
		// Do nothing
	})
}

func TestDefaultResponse(t *testing.T) {
	app := newTestRouter()

	// This should not crash.
	app.Resource("/").Get("get-root", "docs", NewResponse(0, "Default response")).Run(func(ctx Context) {
		ctx.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	app.ServeHTTP(w, req)

	// This should not panic and should return the 200 OK
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestRoundTrip(t *testing.T) {
	app := newTestRouter()

	// This should not crash.
	resource := app.Resource("/")

	type Thing struct {
		Name string `json:"name"`
	}

	resource.Get("get-root", "docs", NewResponse(0, "").Model(Thing{})).Run(func(ctx Context) {
		ctx.WriteModel(http.StatusOK, Thing{Name: "Test"})
	})

	resource.Put("put-root", "", NewResponse(200, "")).Run(func(ctx Context, input struct {
		Body Thing
	}) {
		// If we get here then all the validation passed okay!
		assert.Equal(t, "Test", input.Body.Name)
	})

	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodGet, "/openapi.json", nil)
	app.ServeHTTP(w1, req1)
	t.Log(w1.Body.String())

	w1 = httptest.NewRecorder()
	req1, _ = http.NewRequest(http.MethodGet, "/schemas/Thing.json", nil)
	app.ServeHTTP(w1, req1)
	t.Log(w1.Body.String())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/", nil)
	app.ServeHTTP(w, req)

	// This should not panic and should return the 200 OK
	assert.Equal(t, http.StatusOK, w.Result().StatusCode)

	thing := w.Body

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPut, "/", thing)
	app.ServeHTTP(w, req)

	t.Log(w.Body.String())

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
}

func TestRequestContentTypes(t *testing.T) {
	app := newTestRouter()
	app.DisableSchemaProperty()
	resource := app.Resource("/")

	type ThingV1 struct {
		First string `json:"first"`
		Last  string `json:"last"`
	}

	type ThingV2 struct {
		Name string `json:"name"`
	}

	resource.Post("post-root", "docs",
		NewResponse(http.StatusOK, "").Model(&ThingV2{}),
	).Run(func(ctx Context, input struct {
		BodyV2 *ThingV2 `body:"application/thingv2+json"`
		BodyV1 *ThingV1 `body:"application/thingv1+json"`
	}) {
		var thing *ThingV2
		if input.BodyV2 != nil {
			thing = input.BodyV2
		} else if input.BodyV1 != nil {
			thing = &ThingV2{
				Name: input.BodyV1.First + " " + input.BodyV1.Last,
			}
		}
		ctx.WriteModel(http.StatusOK, thing)
	})

	// Version 1 should work
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"first": "one", "last": "two"}`))
	req.Header.Set("Content-Type", "application/thingv1+json")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"name": "one two"}`, w.Body.String())

	// Version 2 (explicit) should work
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name": "one two"}`))
	req.Header.Set("Content-Type", "application/thingv2+json")
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"name": "one two"}`, w.Body.String())

	// Version 2 (implicit, missing content type) should work by selecting the
	// first defined body (ThingV2).
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name": "one two"}`))
	app.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"name": "one two"}`, w.Body.String())
}

func TestOpenAPIOrdering(t *testing.T) {
	app := newTestRouter()

	type Response struct {
		Description string `json:"description"`
		Category    string `json:"category"`
		ID          string `json:"id"`
		Name        string `json:"name"`
	}

	app.Resource("/menu/{menu-category}/item/{item-id}").Get("cafe menu", "ISP Cafe",
		NewResponse(http.StatusOK, "test").Model(Response{}),
	).Run(func(ctx Context, input struct {
		Category string `path:"menu-category"`
		ItemID   string `path:"item-id"`
	}) {
		ctx.WriteModel(http.StatusOK, Response{
			Category:    input.Category,
			ID:          input.ItemID,
			Name:        "Apple Tea",
			Description: "Green tea with apples",
		})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/menu/drinks/item/apple-tea", nil)
	req.Header.Set("Authorization", "dummy")
	req.Host = "example.com"
	app.ServeHTTP(w, req)

	// JSON response should be lexicographically sorted
	assert.Equal(t, http.StatusOK, w.Code)
	expectedResp := `{"$schema":"https://example.com/schemas/Response.json","category":"drinks","description":"Green tea with apples","id":"apple-tea","name":"Apple Tea"}`
	assert.Equal(t, expectedResp, w.Body.String())

	// Parameters should match insertion order
	openapi := app.OpenAPI().Search("paths", "/menu/{menu-category}/item/{item-id}", "get").Bytes()
	type parameters struct {
		Name string `json:"name"`
	}

	type opschema struct {
		Parameters []parameters `json:"parameters"`
	}

	var p opschema
	json.Unmarshal(openapi, &p)
	assert.Equal(t, p.Parameters, []parameters{{Name: "menu-category"}, {Name: "item-id"}})
}
