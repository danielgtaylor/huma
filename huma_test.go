package huma_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/go-chi/chi/v5"
	"github.com/goccy/go-yaml"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var NewExampleAdapter = humatest.NewAdapter
var NewExampleAPI = humachi.New

// Recoverer is a really simple recovery middleware we can use during tests.
func Recoverer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func TestFeatures(t *testing.T) {
	for _, feature := range []struct {
		Name         string
		Transformers []huma.Transformer
		Register     func(t *testing.T, api huma.API)
		Method       string
		URL          string
		Headers      map[string]string
		Body         string
		Assert       func(t *testing.T, resp *httptest.ResponseRecorder)
	}{
		{
			Name: "middleware",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					// Just a do-nothing passthrough. Shows that chaining works.
					next(ctx)
				})
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					// Return an error response, never calling the next handler.
					ctx.SetStatus(299)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					// This should never be called because of the middleware.
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				// We should get the error response from the middleware.
				assert.Equal(t, 299, resp.Code)
			},
		},
		{
			Name: "params",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test-params/{string}/{int}",
				}, func(ctx context.Context, input *struct {
					PathString    string    `path:"string"`
					PathInt       int       `path:"int"`
					QueryString   string    `query:"string"`
					QueryInt      int       `query:"int"`
					QueryDefault  float32   `query:"def" default:"135" example:"5"`
					QueryBefore   time.Time `query:"before"`
					QueryDate     time.Time `query:"date" timeFormat:"2006-01-02"`
					QueryUint     uint32    `query:"uint"`
					QueryBool     bool      `query:"bool"`
					QueryStrings  []string  `query:"strings"`
					QueryInts     []int     `query:"ints"`
					QueryInts64   []int64   `query:"ints64"`
					QueryUints    []uint    `query:"uints"`
					QueryUints32  []uint32  `query:"uints32"`
					QueryFloats32 []float32 `query:"floats32"`
					QueryFloats64 []float64 `query:"floats64"`
					HeaderString  string    `header:"String"`
					HeaderInt     int       `header:"Int"`
					HeaderDate    time.Time `header:"Date"`
				}) (*struct{}, error) {
					assert.Equal(t, "foo", input.PathString)
					assert.Equal(t, 123, input.PathInt)
					assert.Equal(t, "bar", input.QueryString)
					assert.Equal(t, 456, input.QueryInt)
					assert.EqualValues(t, 135, input.QueryDefault)
					assert.True(t, input.QueryBefore.Equal(time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)))
					assert.True(t, input.QueryDate.Equal(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)))
					assert.EqualValues(t, 1, input.QueryUint)
					assert.True(t, input.QueryBool)
					assert.Equal(t, []string{"foo", "bar"}, input.QueryStrings)
					assert.Equal(t, "baz", input.HeaderString)
					assert.Equal(t, 789, input.HeaderInt)
					assert.Equal(t, []int{2, 3}, input.QueryInts)
					assert.Equal(t, []int64{4, 5}, input.QueryInts64)
					assert.Equal(t, []uint{1, 2}, input.QueryUints)
					assert.Equal(t, []uint32{10, 15}, input.QueryUints32)
					assert.Equal(t, []float32{2.2, 2.3}, input.QueryFloats32)
					assert.Equal(t, []float64{3.2, 3.3}, input.QueryFloats64)
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test-params/foo/123?string=bar&int=456&before=2023-01-01T12:00:00Z&date=2023-01-01&uint=1&bool=true&strings=foo,bar&ints=2,3&ints64=4,5&uints=1,2&uints32=10,15&floats32=2.2,2.3&floats64=3.2,3.3",
			Headers: map[string]string{
				"string": "baz",
				"int":    "789",
				"date":   "Mon, 01 Jan 2023 12:00:00 GMT",
			},
		},
		{
			Name: "params-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Name: "request-body-defaults",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body struct {
						// Test defaults for primitive types.
						Name  string `json:"name,omitempty" default:"Huma"`
						Count int    `json:"count,omitempty" default:"5"`
						// Test defaults for slices of primitives.
						Tags    []string `json:"tags,omitempty" default:"foo, bar"`
						Numbers []int    `json:"numbers,omitempty" default:"[1, 2, 3]"`
						// Test defaults for fields within slices of structs.
						Items []struct {
							ID       int  `json:"id"`
							Verified bool `json:"verified,omitempty" default:"true"`
						} `json:"items,omitempty"`
					}
				}) (*struct{}, error) {
					assert.Equal(t, "Huma", input.Body.Name)
					assert.Equal(t, 5, input.Body.Count)
					assert.Equal(t, []string{"foo", "bar"}, input.Body.Tags)
					assert.Equal(t, []int{1, 2, 3}, input.Body.Numbers)
					assert.Equal(t, 1, input.Body.Items[0].ID)
					assert.True(t, input.Body.Items[0].Verified)
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"items": [{"id": 1}]}`,
		},
		{
			Name: "request-body-required",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Name: "request-body-unsupported-media-type",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Method:  http.MethodPut,
			URL:     "/body",
			Headers: map[string]string{"Content-Type": "application/foo"},
			Body:    `abcd`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnsupportedMediaType, resp.Code)
			},
		},
		{
			Name: "request-body-file-upload",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
				assert.Equal(t, "binary", api.OpenAPI().Paths["/file"].Put.RequestBody.Content["application/foo"].Schema.Format)
			},
			Method:  http.MethodPut,
			URL:     "/file",
			Headers: map[string]string{"Content-Type": "application/foo"},
			Body:    `some-data`,
		},
		{
			Name: "handler-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/error",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, huma.Error403Forbidden("nope")
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
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Str   string    `header:"str"`
					Int   int       `header:"int"`
					Uint  uint      `header:"uint"`
					Float float64   `header:"float"`
					Bool  bool      `header:"bool"`
					Date  time.Time `header:"date"`
					Empty string    `header:"empty"`
				}

				huma.Register(api, huma.Operation{
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
				assert.Empty(t, resp.Header().Values("Empty"))
			},
		},
		{
			Name: "response-custom-content-type",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					ContentType string `header:"Content-Type"`
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-custom-ct",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.ContentType = "application/custom-type"
					return resp, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/response-custom-ct",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code)
				assert.Equal(t, "application/custom-type", resp.Header().Get("Content-Type"))
			},
		},
		{
			Name: "response",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Body struct {
						Greeting string `json:"greeting"`
					}
				}

				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Body []byte
				}

				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/stream",
				}, func(ctx context.Context, input *struct{}) (*huma.StreamResponse, error) {
					return &huma.StreamResponse{
						Body: func(ctx huma.Context) {
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
			Name: "response-transform-error",
			Transformers: []huma.Transformer{
				func(ctx huma.Context, status string, v any) (any, error) {
					return nil, fmt.Errorf("whoops")
				},
			},
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response",
				}, func(ctx context.Context, input *struct{}) (*struct{ Body string }, error) {
					return &struct{ Body string }{"foo"}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/response",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				// Since the handler completed, this returns a 204, however while
				// writing the body there is an error, so that is written as a message
				// into the body and dumped via a panic.
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.Equal(t, `error transforming response`, resp.Body.String())
			},
		},
		{
			Name: "response-marshal-error",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Body struct {
						Greeting any `json:"greeting"`
					}
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.Body.Greeting = func() {}
					return resp, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/response",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.Equal(t, `error marshaling response`, resp.Body.String())
			},
		},
		{
			Name: "dynamic-status",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Status int
				}

				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
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
			Register: func(t *testing.T, api huma.API) {
				// Step 1: create a custom schema
				customSchema := &huma.Schema{
					OneOf: []*huma.Schema{
						{
							Type: huma.TypeObject,
							Properties: map[string]*huma.Schema{
								"foo": {Type: huma.TypeString},
							},
						},
						{
							Type: huma.TypeArray,
							Items: &huma.Schema{
								Type: huma.TypeObject,
								Properties: map[string]*huma.Schema{
									"foo": {Type: huma.TypeString},
								},
							},
						},
					},
				}
				customSchema.PrecomputeMessages()

				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/one-of",
					// Step 2: register an operation with a custom schema
					RequestBody: &huma.RequestBody{
						Required: true,
						Content: map[string]*huma.MediaType{
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
					require.NoError(t, json.Unmarshal(input.RawBody, &parsed))
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
			r.Use(Recoverer)
			config := huma.DefaultConfig("Features Test API", "1.0.0")
			if feature.Transformers != nil {
				config.Transformers = append(config.Transformers, feature.Transformers...)
			}
			api := humatest.NewTestAPI(t, r, config)
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
				cn := w.Body.String()
				assert.Less(t, w.Code, 300, cn)
			}
		})
	}
}

func TestOpenAPI(t *testing.T) {
	r, api := humatest.New(t, huma.DefaultConfig("Features Test API", "1.0.0"))

	type Resp struct {
		Body struct {
			Greeting string `json:"greeting"`
		}
	}

	huma.Register(api, huma.Operation{
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

		assert.Equal(t, 200, w.Code, w.Body.String())
	}
}

type IntNot3 int

func (i IntNot3) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	if i != 0 && i%3 == 0 {
		return []error{&huma.ErrorDetail{
			Location: prefix.String(),
			Message:  "Value cannot be a multiple of three",
			Value:    i,
		}}
	}
	return nil
}

var _ huma.ResolverWithPath = (*IntNot3)(nil)

type ExhaustiveErrorsInputBody struct {
	Name  string  `json:"name" maxLength:"10"`
	Count IntNot3 `json:"count" minimum:"1"`

	// Having a pointer which is never loaded should not cause
	// the tests to fail when running resolvers.
	Ptr *IntNot3 `json:"ptr,omitempty" minimum:"1"`
}

func (b *ExhaustiveErrorsInputBody) Resolve(ctx huma.Context) []error {
	return []error{fmt.Errorf("body resolver error")}
}

type ExhaustiveErrorsInput struct {
	ID     IntNot3                   `path:"id" maximum:"10"`
	Query  IntNot3                   `query:"query"`
	Header IntNot3                   `header:"header"`
	Body   ExhaustiveErrorsInputBody `json:"body"`
}

func (i *ExhaustiveErrorsInput) Resolve(ctx huma.Context) []error {
	return []error{&huma.ErrorDetail{
		Location: "path.id",
		Message:  "input resolver error",
		Value:    i.ID,
	}}
}

var _ huma.Resolver = (*ExhaustiveErrorsInput)(nil)

func TestExhaustiveErrors(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPut,
		Path:        "/errors/{id}",
	}, func(ctx context.Context, input *ExhaustiveErrorsInput) (*struct{}, error) {
		return nil, nil
	})

	req, _ := http.NewRequest(http.MethodPut, "/errors/15?query=3", strings.NewReader(`{"name": "12345678901", "count": -6}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Header", "3")
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
				"message": "expected number <= 10",
				"location": "path.id",
				"value": 15
			}, {
				"message": "expected length <= 10",
				"location": "body.name",
				"value": "12345678901"
			}, {
				"message": "expected number >= 1",
				"location": "body.count",
				"value": -6
			}, {
				"message": "input resolver error",
				"location": "path.id",
				"value": 15
			}, {
				"message": "Value cannot be a multiple of three",
				"location": "path.id",
				"value": 15
			}, {
				"message": "Value cannot be a multiple of three",
				"location": "query.query",
				"value": 3
			}, {
				"message": "Value cannot be a multiple of three",
				"location": "header.header",
				"value": 3
			}, {
				"message": "body resolver error"
			}, {
				"message": "Value cannot be a multiple of three",
				"location": "body.count",
				"value": -6
			}
		]
	}`, w.Body.String())
}

type MyError struct {
	status  int
	Message string   `json:"message"`
	Details []string `json:"details"`
}

func (e *MyError) Error() string {
	return e.Message
}

func (e *MyError) GetStatus() int {
	return e.status
}

func TestCustomError(t *testing.T) {
	orig := huma.NewError
	defer func() {
		huma.NewError = orig
	}()
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		details := make([]string, len(errs))
		for i, err := range errs {
			details[i] = err.Error()
		}
		return &MyError{
			status:  status,
			Message: message,
			Details: details,
		}
	}

	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "get-error",
		Method:      http.MethodGet,
		Path:        "/error",
	}, func(ctx context.Context, i *struct{}) (*struct{}, error) {
		return nil, huma.Error404NotFound("not found", fmt.Errorf("some-other-error"))
	})

	resp := api.Get("/error", "Host: localhost")
	assert.Equal(t, `{"$schema":"http://localhost/schemas/MyError.json","message":"not found","details":["some-other-error"]}`+"\n", resp.Body.String())
}

type NestedResolversStruct struct {
	Field2 string `json:"field2"`
}

func (b *NestedResolversStruct) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	return []error{&huma.ErrorDetail{
		Location: prefix.With("field2"),
		Message:  "resolver error",
		Value:    b.Field2,
	}}
}

var _ huma.ResolverWithPath = (*NestedResolversStruct)(nil)

type NestedResolversBody struct {
	Field1 map[string][]NestedResolversStruct `json:"field1"`
}

type NestedResolverRequest struct {
	Body NestedResolversBody
}

func TestNestedResolverWithPath(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
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

func (r *ResolverCustomStatus) Resolve(ctx huma.Context) []error {
	return []error{huma.Error403Forbidden("nope")}
}

func TestResolverCustomStatus(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
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

func TestParamPointerPanics(t *testing.T) {
	// For now we don't support these, so we panic rather than have subtle
	// bugs that are hard to track down.
	_, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	assert.Panics(t, func() {
		huma.Register(app, huma.Operation{
			OperationID: "bug",
			Method:      http.MethodGet,
			Path:        "/bug",
		}, func(ctx context.Context, input *struct {
			Param *string `query:"param"`
		}) (*struct{}, error) {
			return nil, nil
		})
	})
}

func TestPointerDefaultPanics(t *testing.T) {
	// For now we don't support these, so we panic rather than have subtle
	// bugs that are hard to track down.
	_, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	assert.Panics(t, func() {
		huma.Register(app, huma.Operation{
			OperationID: "bug",
			Method:      http.MethodGet,
			Path:        "/bug",
		}, func(ctx context.Context, input *struct {
			Body struct {
				Value *string `json:"value,omitempty" default:"foo"`
			}
		}) (*struct{}, error) {
			return nil, nil
		})
	})
}

func BenchmarkSecondDecode(b *testing.B) {
	//nolint: musttag
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
		} `json:"owner"`
		Categories []struct {
			Name    string   `json:"name"`
			Order   int      `json:"order"`
			Visible bool     `json:"visible"`
			Aliases []string `json:"aliases"`
		} `json:"categories"`
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

	pb := huma.NewPathBuffer([]byte{}, 0)
	res := &huma.ValidateResult{}
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	fmt.Println("name", reflect.TypeOf(MediumSized{}).Name())
	schema := registry.Schema(reflect.TypeOf(MediumSized{}), false, "")

	b.Run("json.Unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var tmp any
			if err := json.Unmarshal(data, &tmp); err != nil {
				panic(err)
			}

			huma.Validate(registry, schema, pb, huma.ModeReadFromServer, tmp, res)

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

			huma.Validate(registry, schema, pb, huma.ModeReadFromServer, tmp, res)

			var out MediumSized
			if err := mapstructure.Decode(tmp, &out); err != nil {
				panic(err)
			}
		}
	})
}

func globalHandler(ctx context.Context, input *struct {
	Count int `query:"count"`
}) (*struct{ Body int }, error) {
	return &struct{ Body int }{Body: input.Count * 3 / 2}, nil
}

var BenchmarkHandlerResponse *httptest.ResponseRecorder

// BenchmarkHandlerFunc compares the performance of a global handler function
// defined via `func name(...) { ... }` versus an inline handler function
// which is defined as `func(...) { ... }` as an argument to `huma.Register`.
// Performance should not be impacted much (if any) between the two.
func BenchmarkHandlerFunc(b *testing.B) {
	_, api := humatest.New(b, huma.DefaultConfig("Test API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "global",
		Method:      http.MethodGet,
		Path:        "/global",
	}, globalHandler)

	huma.Register(api, huma.Operation{
		OperationID: "inline",
		Method:      http.MethodGet,
		Path:        "/inline",
	}, func(ctx context.Context, input *struct {
		Count int `query:"count"`
	}) (*struct{ Body int }, error) {
		return &struct{ Body int }{Body: input.Count * 3 / 2}, nil
	})

	b.Run("global", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BenchmarkHandlerResponse = api.Get("/global")
		}
	})

	b.Run("inline", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BenchmarkHandlerResponse = api.Get("/inline")
		}
	})
}
