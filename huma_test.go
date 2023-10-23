package huma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/queryparam"
	"github.com/go-chi/chi/v5"
	"github.com/goccy/go-yaml"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
)

type testContext struct {
	op *Operation
	r  *http.Request
	w  http.ResponseWriter
}

func (c *testContext) Operation() *Operation {
	return c.op
}

func (c *testContext) Matched() string {
	return chi.RouteContext(c.r.Context()).RoutePattern()
}

func (c *testContext) Context() context.Context {
	return c.r.Context()
}

func (c *testContext) Method() string {
	return c.r.Method
}

func (c *testContext) Host() string {
	return c.r.Host
}

func (c *testContext) URL() url.URL {
	return *c.r.URL
}

func (c *testContext) Param(name string) string {
	return chi.URLParam(c.r, name)
}

func (c *testContext) Query(name string) string {
	return queryparam.Get(c.r.URL.RawQuery, name)
}

func (c *testContext) Header(name string) string {
	return c.r.Header.Get(name)
}

func (c *testContext) EachHeader(cb func(name, value string)) {
	for name, values := range c.r.Header {
		for _, value := range values {
			cb(name, value)
		}
	}
}

func (c *testContext) Body() ([]byte, error) {
	return io.ReadAll(c.r.Body)
}

func (c *testContext) BodyReader() io.Reader {
	return c.r.Body
}

func (c *testContext) GetMultipartForm() (*multipart.Form, error) {
	err := c.r.ParseMultipartForm(8 * 1024)
	return c.r.MultipartForm, err
}

func (c *testContext) SetReadDeadline(deadline time.Time) error {
	return http.NewResponseController(c.w).SetReadDeadline(deadline)
}

func (c *testContext) SetStatus(code int) {
	c.w.WriteHeader(code)
}

func (c *testContext) AppendHeader(name string, value string) {
	c.w.Header().Add(name, value)
}

func (c *testContext) SetHeader(name string, value string) {
	c.w.Header().Set(name, value)
}

func (c *testContext) BodyWriter() io.Writer {
	return c.w
}

type testAdapter struct {
	router chi.Router
}

func (a *testAdapter) Handle(op *Operation, handler func(Context)) {
	a.router.MethodFunc(op.Method, op.Path, func(w http.ResponseWriter, r *http.Request) {
		handler(&testContext{op: op, r: r, w: w})
	})
}

func (a *testAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.router.ServeHTTP(w, r)
}

func NewTestAdapter(r chi.Router, config Config) API {
	return NewAPI(config, &testAdapter{router: r})
}

func TestFeatures(t *testing.T) {
	for _, feature := range []struct {
		Name     string
		Register func(t *testing.T, api API)
		Method   string
		URL      string
		Headers  map[string]string
		Body     string
		Assert   func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			Name: "params",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/test-params/{string}/{int}",
				}, func(ctx context.Context, input *struct {
					PathString   string    `path:"string"`
					PathInt      int       `path:"int"`
					QueryString  string    `query:"string"`
					QueryInt     int       `query:"int"`
					QueryDefault float32   `query:"def" default:"135" example:"5"`
					QueryBefore  time.Time `query:"before"`
					QueryDate    time.Time `query:"date" timeFormat:"2006-01-02"`
					QueryUint    uint32    `query:"uint"`
					QueryBool    bool      `query:"bool"`
					QueryStrings []string  `query:"strings"`
					HeaderString string    `header:"String"`
					HeaderInt    int       `header:"Int"`
					HeaderDate   time.Time `header:"Date"`
				}) (*struct{}, error) {
					assert.Equal(t, "foo", input.PathString)
					assert.Equal(t, 123, input.PathInt)
					assert.Equal(t, "bar", input.QueryString)
					assert.Equal(t, 456, input.QueryInt)
					assert.EqualValues(t, 135, input.QueryDefault)
					assert.True(t, input.QueryBefore.Equal(time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)))
					assert.True(t, input.QueryDate.Equal(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)))
					assert.EqualValues(t, 1, input.QueryUint)
					assert.Equal(t, true, input.QueryBool)
					assert.Equal(t, []string{"foo", "bar"}, input.QueryStrings)
					assert.Equal(t, "baz", input.HeaderString)
					assert.Equal(t, 789, input.HeaderInt)
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test-params/foo/123?string=bar&int=456&before=2023-01-01T12:00:00Z&date=2023-01-01&uint=1&bool=true&strings=foo,bar",
			Headers: map[string]string{
				"string": "baz",
				"int":    "789",
				"date":   "Mon, 01 Jan 2023 12:00:00 GMT",
			},
		},
		{
			Name: "params-error",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/test-params/{int}",
				}, func(ctx context.Context, input *struct {
					PathInt     string    `path:"int"`
					QueryInt    int       `query:"int"`
					QueryFloat  float32   `query:"float"`
					QueryBefore time.Time `query:"before"`
					QueryDate   time.Time `query:"date" timeFormat:"2006-01-02"`
					QueryUint   uint32    `query:"uint"`
					QueryBool   bool      `query:"bool"`
					QueryReq    bool      `query:"req" required:"true"`
					HeaderReq   string    `header:"req" required:"true"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test-params/bad?int=bad&float=bad&before=bad&date=bad&uint=bad&bool=bad",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "invalid integer")
				assert.Contains(t, resp.Body.String(), "invalid float")
				assert.Contains(t, resp.Body.String(), "invalid date/time")
				assert.Contains(t, resp.Body.String(), "invalid bool")
				assert.Contains(t, resp.Body.String(), "required query parameter is missing")
				assert.Contains(t, resp.Body.String(), "required header parameter is missing")
			},
		},
		{
			Name: "request-body",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					RawBody []byte
					Body    struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					assert.Equal(t, `{"name":"foo"}`, string(input.RawBody))
					assert.Equal(t, "foo", input.Body.Name)
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			// Headers: map[string]string{"Content-Type": "application/json"},
			Body: `{"name":"foo"}`,
		},
		{
			Name: "request-body-required",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadRequest, resp.Code)
			},
		},
		{
			Name: "request-ptr-body-required",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body *struct {
						Name string `json:"name"`
					} `required:"true"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadRequest, resp.Code)
			},
		},
		{
			Name: "request-body-too-large",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method:       http.MethodPut,
					Path:         "/body",
					MaxBodyBytes: 1,
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   "foobarbaz",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusRequestEntityTooLarge, resp.Code)
			},
		},
		{
			Name: "request-body-bad-json",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   "{{{",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusBadRequest, resp.Code)
			},
		},
		{
			Name: "request-body-file-upload",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodPut,
					Path:   "/file",
				}, func(ctx context.Context, input *struct {
					RawBody []byte `contentType:"application/foo"`
				}) (*struct{}, error) {
					assert.Equal(t, `some-data`, string(input.RawBody))
					return nil, nil
				})

				// Ensure OpenAPI spec is listed as a binary upload. This enables
				// generated documentation to show a file upload button.
				assert.Equal(t, api.OpenAPI().Paths["/file"].Put.RequestBody.Content["application/foo"].Schema.Format, "binary")
			},
			Method:  http.MethodPut,
			URL:     "/file",
			Headers: map[string]string{"Content-Type": "application/foo"},
			Body:    `some-data`,
		},
		{
			Name: "handler-error",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/error",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, Error403Forbidden("nope")
				})
			},
			Method: http.MethodGet,
			URL:    "/error",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusForbidden, resp.Code)
			},
		},
		{
			Name: "response-headers",
			Register: func(t *testing.T, api API) {
				type Resp struct {
					Str   string    `header:"str"`
					Int   int       `header:"int"`
					Uint  uint      `header:"uint"`
					Float float64   `header:"float"`
					Bool  bool      `header:"bool"`
					Date  time.Time `header:"date"`
				}

				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/response-headers",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.Str = "str"
					resp.Int = 1
					resp.Uint = 2
					resp.Float = 3.45
					resp.Bool = true
					resp.Date = time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
					return resp, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/response-headers",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code)
				assert.Equal(t, "str", resp.Header().Get("Str"))
				assert.Equal(t, "1", resp.Header().Get("Int"))
				assert.Equal(t, "2", resp.Header().Get("Uint"))
				assert.Equal(t, "3.45", resp.Header().Get("Float"))
				assert.Equal(t, "true", resp.Header().Get("Bool"))
				assert.Equal(t, "Sun, 01 Jan 2023 12:00:00 GMT", resp.Header().Get("Date"))
			},
		},
		{
			Name: "response",
			Register: func(t *testing.T, api API) {
				type Resp struct {
					Body struct {
						Greeting string `json:"greeting"`
					}
				}

				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/response",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.Body.Greeting = "Hello, world!"
					return resp, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/response",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.JSONEq(t, `{"$schema": "https:///schemas/RespBody.json", "greeting":"Hello, world!"}`, resp.Body.String())
			},
		},
		{
			Name: "response-raw",
			Register: func(t *testing.T, api API) {
				type Resp struct {
					Body []byte
				}

				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/response-raw",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					return &Resp{Body: []byte("hello")}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/response-raw",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.Equal(t, `hello`, resp.Body.String())
			},
		},
		{
			Name: "response-stream",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/stream",
				}, func(ctx context.Context, input *struct{}) (*StreamResponse, error) {
					return &StreamResponse{
						Body: func(ctx Context) {
							writer := ctx.BodyWriter()
							writer.Write([]byte("hel"))
							writer.Write([]byte("lo"))
						},
					}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/stream",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.Equal(t, `hello`, resp.Body.String())
			},
		},
		{
			Name: "dynamic-status",
			Register: func(t *testing.T, api API) {
				type Resp struct {
					Status int
				}

				Register(api, Operation{
					Method: http.MethodGet,
					Path:   "/status",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.Status = 256
					return resp, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/status",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, 256, resp.Code)
			},
		},
		{
			// Simulate a request with a body that came from another call, which
			// includes the `$schema` field. It should be allowed to be passed
			// to the new operation as input without modification.
			Name: "round-trip-schema-field",
			Register: func(t *testing.T, api API) {
				Register(api, Operation{
					Method: http.MethodPut,
					Path:   "/round-trip",
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/round-trip",
			Body:   `{"$schema": "...", "name": "foo"}`,
		},
		{
			Name: "one-of input",
			Register: func(t *testing.T, api API) {
				// Step 1: create a custom schema
				customSchema := &Schema{
					OneOf: []*Schema{
						{
							Type: TypeObject,
							Properties: map[string]*Schema{
								"foo": {Type: TypeString},
							},
						},
						{
							Type: TypeArray,
							Items: &Schema{
								Type: TypeObject,
								Properties: map[string]*Schema{
									"foo": {Type: TypeString},
								},
							},
						},
					},
				}
				customSchema.PrecomputeMessages()

				Register(api, Operation{
					Method: http.MethodPut,
					Path:   "/one-of",
					// Step 2: register an operation with a custom schema
					RequestBody: &RequestBody{
						Required: true,
						Content: map[string]*MediaType{
							"application/json": {
								Schema: customSchema,
							},
						},
					},
				}, func(ctx context.Context, input *struct {
					// Step 3: only take in raw bytes
					RawBody []byte
				}) (*struct{}, error) {
					// Step 4: determine which it is and parse it into the right type.
					// We will check the first byte but there are other ways to do this.
					assert.EqualValues(t, '[', input.RawBody[0])
					var parsed []struct {
						Foo string `json:"foo"`
					}
					assert.NoError(t, json.Unmarshal(input.RawBody, &parsed))
					assert.Len(t, parsed, 2)
					assert.Equal(t, "first", parsed[0].Foo)
					assert.Equal(t, "second", parsed[1].Foo)
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/one-of",
			Body:   `[{"foo": "first"}, {"foo": "second"}]`,
		},
	} {
		t.Run(feature.Name, func(t *testing.T) {
			r := chi.NewRouter()
			api := NewTestAdapter(r, DefaultConfig("Features Test API", "1.0.0"))
			feature.Register(t, api)

			var body io.Reader = nil
			if feature.Body != "" {
				body = strings.NewReader(feature.Body)
			}
			req, _ := http.NewRequest(feature.Method, feature.URL, body)
			for k, v := range feature.Headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			b, _ := yaml.Marshal(api.OpenAPI())
			t.Log(string(b))
			if feature.Assert != nil {
				feature.Assert(t, w)
			} else {
				assert.Less(t, w.Code, 300, w.Body.String())
			}
		})
	}
}

func TestOpenAPI(t *testing.T) {
	r := chi.NewRouter()
	api := NewTestAdapter(r, DefaultConfig("Features Test API", "1.0.0"))

	type Resp struct {
		Body struct {
			Greeting string `json:"greeting"`
		}
	}

	Register(api, Operation{
		Method: http.MethodGet,
		Path:   "/test",
	}, func(ctx context.Context, input *struct{}) (*Resp, error) {
		resp := &Resp{}
		resp.Body.Greeting = "Hello, world"
		return resp, nil
	})

	for _, url := range []string{"/openapi.json", "/openapi.yaml", "/docs", "/schemas/Resp.json"} {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, w.Code, 200, w.Body.String())
	}
}

type ExhaustiveErrorsInputBody struct {
	Name  string `json:"name" maxLength:"10"`
	Count int    `json:"count" minimum:"1"`
}

func (b *ExhaustiveErrorsInputBody) Resolve(ctx Context) []error {
	return []error{fmt.Errorf("body resolver error")}
}

type ExhaustiveErrorsInput struct {
	ID   string                    `path:"id" maxLength:"5"`
	Body ExhaustiveErrorsInputBody `json:"body"`
}

func (i *ExhaustiveErrorsInput) Resolve(ctx Context) []error {
	return []error{&ErrorDetail{
		Location: "path.id",
		Message:  "input resolver error",
		Value:    i.ID,
	}}
}

type ExhaustiveErrorsOutput struct {
}

func TestExhaustiveErrors(t *testing.T) {
	r := chi.NewRouter()
	app := NewTestAdapter(r, DefaultConfig("Test API", "1.0.0"))
	Register(app, Operation{
		OperationID: "test",
		Method:      http.MethodPut,
		Path:        "/errors/{id}",
	}, func(ctx context.Context, input *ExhaustiveErrorsInput) (*ExhaustiveErrorsOutput, error) {
		return &ExhaustiveErrorsOutput{}, nil
	})

	req, _ := http.NewRequest(http.MethodPut, "/errors/123456", strings.NewReader(`{"name": "12345678901", "count": 0}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.JSONEq(t, `{
		"$schema": "https:///schemas/ErrorModel.json",
		"title": "Unprocessable Entity",
		"status": 422,
		"detail": "validation failed",
		"errors": [
			{
				"message": "expected length <= 5",
				"location": "path.id",
				"value": "123456"
			}, {
				"message": "expected length <= 10",
				"location": "body.name",
				"value": "12345678901"
			}, {
				"message": "expected number >= 1",
				"location": "body.count",
				"value": 0
			}, {
				"message": "input resolver error",
				"location": "path.id",
				"value": "123456"
			}, {
				"message": "body resolver error"
			}
		]
	}`, w.Body.String())
}

type NestedResolversStruct struct {
	Field2 string `json:"field2"`
}

func (b *NestedResolversStruct) Resolve(ctx Context, prefix *PathBuffer) []error {
	return []error{&ErrorDetail{
		Location: prefix.With("field2"),
		Message:  "resolver error",
		Value:    b.Field2,
	}}
}

var _ ResolverWithPath = (*NestedResolversStruct)(nil)

type NestedResolversBody struct {
	Field1 map[string][]NestedResolversStruct `json:"field1"`
}

type NestedResolverRequest struct {
	Body NestedResolversBody
}

func TestNestedResolverWithPath(t *testing.T) {
	r := chi.NewRouter()
	app := NewTestAdapter(r, DefaultConfig("Test API", "1.0.0"))
	Register(app, Operation{
		OperationID: "test",
		Method:      http.MethodPut,
		Path:        "/test",
	}, func(ctx context.Context, input *NestedResolverRequest) (*struct{}, error) {
		return nil, nil
	})

	req, _ := http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{"field1": {"foo": [{"field2": "bar"}]}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"location":"body.field1.foo[0].field2"`)
}

type ResolverCustomStatus struct{}

func (r *ResolverCustomStatus) Resolve(ctx Context) []error {
	return []error{Error403Forbidden("nope")}
}

func TestResolverCustomStatus(t *testing.T) {
	r := chi.NewRouter()
	app := NewTestAdapter(r, DefaultConfig("Test API", "1.0.0"))
	Register(app, Operation{
		OperationID: "test",
		Method:      http.MethodPut,
		Path:        "/test",
	}, func(ctx context.Context, input *ResolverCustomStatus) (*struct{}, error) {
		return nil, nil
	})

	req, _ := http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "nope")
}

func BenchmarkSecondDecode(b *testing.B) {
	type MediumSized struct {
		ID   int      `json:"id"`
		Name string   `json:"name"`
		Tags []string `json:"tags"`
		// Created time.Time `json:"created"`
		// Updated time.Time `json:"updated"`
		Rating float64 `json:"rating"`
		Owner  struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
		}
		Categories []struct {
			Name    string   `json:"name"`
			Order   int      `json:"order"`
			Visible bool     `json:"visible"`
			Aliases []string `json:"aliases"`
		}
	}

	data := []byte(`{
		"id": 123,
		"name": "Test",
		"tags": ["one", "two", "three"],
		"created": "2021-01-01T12:00:00Z",
		"updated": "2021-01-01T12:00:00Z",
		"rating": 5.0,
		"owner": {
			"id": 4,
			"name": "Alice",
			"email": "alice@example.com"
		},
		"categories": [
			{
				"name": "First",
				"order": 1,
				"visible": true
			},
			{
				"name": "Second",
				"order": 2,
				"visible": false,
				"aliases": ["foo", "bar"]
			}
		]
	}`)

	pb := NewPathBuffer([]byte{}, 0)
	res := &ValidateResult{}
	registry := NewMapRegistry("#/components/schemas/", DefaultSchemaNamer)
	fmt.Println("name", reflect.TypeOf(MediumSized{}).Name())
	schema := registry.Schema(reflect.TypeOf(MediumSized{}), false, "")

	b.Run("json.Unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var tmp any
			if err := json.Unmarshal(data, &tmp); err != nil {
				panic(err)
			}

			Validate(registry, schema, pb, ModeReadFromServer, tmp, res)

			var out MediumSized
			if err := json.Unmarshal(data, &out); err != nil {
				panic(err)
			}
		}
	})

	b.Run("mapstructure.Decode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var tmp any
			if err := json.Unmarshal(data, &tmp); err != nil {
				panic(err)
			}

			Validate(registry, schema, pb, ModeReadFromServer, tmp, res)

			var out MediumSized
			if err := mapstructure.Decode(tmp, &out); err != nil {
				panic(err)
			}
		}
	})
}

// var jsonData = []byte(`[
//   {
//     "desired_state": "ON",
//     "etag": "203f7a94",
//     "id": "bvt3",
//     "name": "BVT channel - CNN Plus 2",
//     "org": "t2dev",
//     "self": "https://api.istreamplanet.com/v2/t2dev/channels/bvt3",
// 		"created": "2021-01-01T12:00:00Z",
// 		"count": 18273,
// 		"rating": 5.0,
// 		"tags": ["one", "three"],
//     "source": {
//       "id": "stn-dd4j42ytxmajz6xz",
//       "self": "https://api.istreamplanet.com/v2/t2dev/sources/stn-dd4j42ytxmajz6xz"
//     }
//   },
//   {
//     "desired_state": "ON",
//     "etag": "WgY5zNTPn3ECf_TSPAgL9Y-E9doUaRxAdjukGsCt_sQ",
//     "id": "bvt2",
//     "name": "BVT channel - Hulu",
//     "org": "t2dev",
//     "self": "https://api.istreamplanet.com/v2/t2dev/channels/bvt2",
// 		"created": "2023-01-01T12:01:00Z",
// 		"count": 1,
// 		"rating": 4.5,
// 		"tags": ["two"],
//     "source": {
//       "id": "stn-yuqvm3hzowrv6rph",
//       "self": "https://api.istreamplanet.com/v2/t2dev/sources/stn-yuqvm3hzowrv6rph"
//     }
//   },
//   {
//     "desired_state": "ON",
//     "etag": "1GaleyULVhpmHJXCJPUGSeBM2YYAZGBYKVcR5sZu5U8",
//     "id": "bvt1",
//     "name": "BVT channel - Hulu",
//     "org": "t2dev",
//     "self": "https://api.istreamplanet.com/v2/t2dev/channels/bvt1",
// 		"created": "2023-01-01T12:00:00Z",
// 		"count": 57,
// 		"rating": 3.5,
// 		"tags": ["one", "two"],
//     "source": {
//       "id": "stn-fc6sqodptbz5keuy",
//       "self": "https://api.istreamplanet.com/v2/t2dev/sources/stn-fc6sqodptbz5keuy"
//     }
//   }
// ]`)

// type Summary struct {
// 	DesiredState string    `json:"desired_state"`
// 	ETag         string    `json:"etag"`
// 	ID           string    `json:"id"`
// 	Name         string    `json:"name"`
// 	Org          string    `json:"org"`
// 	Self         string    `json:"self"`
// 	Created      time.Time `json:"created"`
// 	Count        int       `json:"count"`
// 	Rating       float64   `json:"rating"`
// 	Tags         []string  `json:"tags"`
// 	Source       struct {
// 		ID   string `json:"id"`
// 		Self string `json:"self"`
// 	} `json:"source"`
// }

// func BenchmarkMarshalStructJSON(b *testing.B) {
// 	var summaries []Summary
// 	if err := stdjson.Unmarshal(jsonData, &summaries); err != nil {
// 		panic(err)
// 	}

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := stdjson.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkMarshalAnyJSON(b *testing.B) {
// 	var summaries any
// 	stdjson.Unmarshal(jsonData, &summaries)

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := stdjson.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkUnmarshalStructJSON(b *testing.B) {
// 	var summaries []Summary

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		stdjson.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }

// func BenchmarkUnmarshalAnyJSON(b *testing.B) {
// 	var summaries any

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		stdjson.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }

// func BenchmarkMarshalStructJSONiter(b *testing.B) {
// 	var summaries []Summary
// 	json.Unmarshal(jsonData, &summaries)

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := json.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkMarshalAnyJSONiter(b *testing.B) {
// 	var summaries any
// 	json.Unmarshal(jsonData, &summaries)

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		b, _ := json.Marshal(summaries)
// 		_ = b
// 	}
// }

// func BenchmarkUnmarshalStructJSONiter(b *testing.B) {
// 	var summaries []Summary

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		json.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }

// func BenchmarkUnmarshalAnyJSONiter(b *testing.B) {
// 	var summaries any

// 	b.ResetTimer()
// 	b.ReportAllocs()
// 	for i := 0; i < b.N; i++ {
// 		summaries = nil
// 		json.Unmarshal(jsonData, &summaries)
// 		_ = summaries
// 	}
// }
