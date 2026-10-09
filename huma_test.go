package huma_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	_ "github.com/danielgtaylor/huma/v2/formats/cbor"
	"github.com/danielgtaylor/huma/v2/humatest"
)

var NewExampleAdapter = humatest.NewAdapter
var NewExampleAPI = humago.New

// Recoverer is a really simple recovery middleware we can use during tests.
func Recoverer(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				fmt.Println(rvr)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

// UUID is a custom type for testing SchemaProvider
type UUID struct {
	uuid.UUID
}

// Node is a custom type for testing recursive definition for huma.Register
type Node struct {
	Name  string          `json:"name"`
	Nodes []Node          `json:"nodes,omitempty"`
	Left  *Node           `json:"left,omitempty"`
	Named map[string]Node `json:"named,omitempty"`
}

func (UUID) Schema(r huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeString, Format: "uuid"}
}

// BodyContainer is an embed request body struct to test request body unmarshaling
type BodyContainer struct {
	Body struct {
		Name string `json:"name"`
	}
}

type CustomStringParam string

type StructWithDefaultField struct {
	Field string `json:"field" default:"default"`
}

// MyTextUnmarshaler is a custom type that implements the
// `encoding.TextUnmarshaler` interface
type MyTextUnmarshaler struct {
	value string
}

// MyCustomUnmarshaler is a custom type that imitates performing
// operations on integers
type MyCustomUnmarshaler struct {
	value int
	raw   string
}

func (m *MyTextUnmarshaler) UnmarshalText(text []byte) error {
	m.value = "Hello, World!"
	return nil
}

func (m *MyCustomUnmarshaler) UnmarshalText(text []byte) error {
	var err error
	m.value, err = strconv.Atoi(string(text)[1:])
	if err != nil {
		return err
	}
	m.raw = string(text)
	return nil
}

type OptionalParam[T any] struct {
	Value T
	IsSet bool
}

func (o OptionalParam[T]) Schema(r huma.Registry) *huma.Schema {
	return huma.SchemaFromType(r, reflect.TypeOf(o.Value))
}

func (o *OptionalParam[T]) Receiver() reflect.Value {
	return reflect.ValueOf(o).Elem().Field(0)
}

func (o *OptionalParam[T]) OnParamSet(isSet bool, parsed any) {
	o.IsSet = isSet
}

// CountingInner is used to verify resolver traversal skips nil optional fields.
type CountingInner struct{}

var resolverCalls int

func (b *CountingInner) Resolve(_ huma.Context, _ *huma.PathBuffer) []error {
	resolverCalls++
	return nil
}

func TestFeatures(t *testing.T) {
	for _, feature := range []struct {
		Name         string
		Transformers []huma.Transformer
		Config       huma.Config
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
			Name: "middleware-cookie",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "foo")
					require.NoError(t, err)
					assert.Equal(t, "bar", cookie.Value)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": "foo=bar",
			},
		},
		{
			Name: "middleware-empty-cookie",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "foo")
					assert.Nil(t, cookie)
					require.ErrorIs(t, err, http.ErrNoCookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": "",
			},
		},
		{
			Name: "middleware-cookie-only-semicolon",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "foo")
					assert.Nil(t, cookie)
					require.ErrorIs(t, err, http.ErrNoCookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": ";",
			},
		},
		{
			Name: "middleware-read-no-cookie-in-header",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "foo")
					assert.Nil(t, cookie)
					require.ErrorIs(t, err, http.ErrNoCookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
		},
		{
			Name: "openapi-relative-server",
			Config: func() huma.Config {
				c := huma.DefaultConfig("Test API", "1.0.0")
				c.Servers = []*huma.Server{
					{URL: "/v1"},
				}
				return c
			}(),
			Register: func(t *testing.T, api huma.API) {},
			Method:   http.MethodGet,
			URL:      "/openapi.json",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, 200, resp.Code)
				assert.Contains(t, resp.Body.String(), `"url":"/v1"`)
			},
		},
		{
			Name: "openapi-relative-server-dot",
			Config: func() huma.Config {
				c := huma.DefaultConfig("Test API", "1.0.0")
				c.Servers = []*huma.Server{
					{URL: "./v1"},
				}
				return c
			}(),
			Register: func(t *testing.T, api huma.API) {},
			Method:   http.MethodGet,
			URL:      "/openapi.json",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, 200, resp.Code)
				assert.Contains(t, resp.Body.String(), `"url":"./v1"`)
			},
		},
		{
			Name: "middleware-cookie-invalid-name",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "foo")
					assert.Nil(t, cookie)
					require.ErrorIs(t, err, http.ErrNoCookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": "=bar;",
			},
		},
		{
			Name: "middleware-cookie-filter-skip",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "foo")
					assert.Nil(t, cookie)
					require.ErrorIs(t, err, http.ErrNoCookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": "bar=foo;",
			},
		},
		{
			Name: "middleware-cookie-parse-double-quote",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "bar")
					require.NoError(t, err)
					assert.NotNil(t, cookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": `bar="foo"`,
			},
		},
		{
			Name: "middleware-cookie-invalid-value-byte-with-semicolon",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "bar")
					assert.Nil(t, cookie)
					require.ErrorIs(t, err, http.ErrNoCookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": `bar="fo;o"`,
			},
		},
		{
			Name: "middleware-cookie-invalid-value-byte-with-double-backslash",
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					cookie, err := huma.ReadCookie(ctx, "bar")
					assert.Nil(t, cookie)
					require.ErrorIs(t, err, http.ErrNoCookie)

					next(ctx)
				})
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/middleware",
			Headers: map[string]string{
				"Cookie": `bar="fo\\o"`,
			},
		},
		{
			Name: "middleware-operation",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/middleware",
					Middlewares: huma.Middlewares{
						func(ctx huma.Context, next func(huma.Context)) {
							// Just a do-nothing passthrough. Shows that chaining works.
							next(ctx)
						},
						func(ctx huma.Context, next func(huma.Context)) {
							// Return an error response, never calling the next handler.
							ctx.SetStatus(299)
						},
					},
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
					Path:   "/test-params/{string}/{int}/{uuid}",
				}, func(ctx context.Context, input *struct {
					PathString         string              `path:"string" doc:"Some docs"`
					PathInt            int                 `path:"int"`
					PathUUID           UUID                `path:"uuid"`
					QueryString        string              `query:"string"`
					QueryCustomString  CustomStringParam   `query:"customString"`
					QueryInt           int                 `query:"int"`
					QueryDefault       float32             `query:"def" default:"135" example:"5"`
					QueryBefore        time.Time           `query:"before"`
					QueryDate          time.Time           `query:"date" timeFormat:"2006-01-02"`
					QueryURL           url.URL             `query:"url"`
					QueryUint          uint32              `query:"uint"`
					QueryBool          bool                `query:"bool"`
					QueryStrings       []string            `query:"strings"`
					QueryCustomStrings []CustomStringParam `query:"customStrings"`
					QueryInts          []int               `query:"ints"`
					QueryInts8         []int8              `query:"ints8"`
					QueryInts16        []int16             `query:"ints16"`
					QueryInts32        []int32             `query:"ints32"`
					QueryInts64        []int64             `query:"ints64"`
					QueryUints         []uint              `query:"uints"`
					// QueryUints8   []uint8   `query:"uints8"`
					QueryUints16  []uint16    `query:"uints16"`
					QueryUints32  []uint32    `query:"uints32"`
					QueryUints64  []uint64    `query:"uints64"`
					QueryFloats32 []float32   `query:"floats32"`
					QueryFloats64 []float64   `query:"floats64"`
					QueryExploded []string    `query:"exploded,explode"`
					HeaderString  string      `header:"String"`
					HeaderInt     int         `header:"Int"`
					HeaderDate    time.Time   `header:"Date"`
					CookieValue   string      `cookie:"one"`
					CookieInt     int         `cookie:"two"`
					CookieFull    http.Cookie `cookie:"three"`
				}) (*struct{}, error) {
					assert.Equal(t, "foo", input.PathString)
					assert.Equal(t, 123, input.PathInt)
					assert.Equal(t, UUID{UUID: uuid.MustParse("fba4f46b-4539-4d19-8e3f-a0e629a243b5")}, input.PathUUID)
					assert.Equal(t, "bar", input.QueryString)
					assert.Equal(t, CustomStringParam("bar"), input.QueryCustomString)
					assert.Equal(t, 456, input.QueryInt)
					assert.InDelta(t, 135, input.QueryDefault, 0)
					assert.True(t, input.QueryBefore.Equal(time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)))
					assert.True(t, input.QueryDate.Equal(time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)))
					assert.Equal(t, url.URL{Scheme: "http", Host: "foo.com", Path: "/bar"}, input.QueryURL)
					assert.EqualValues(t, 1, input.QueryUint)
					assert.True(t, input.QueryBool)
					assert.Equal(t, []string{"foo", "bar"}, input.QueryStrings)
					assert.Equal(t, []CustomStringParam{"foo", "bar"}, input.QueryCustomStrings)
					assert.Equal(t, "baz", input.HeaderString)
					assert.Equal(t, 789, input.HeaderInt)
					assert.Equal(t, []int{2, 3}, input.QueryInts)
					assert.Equal(t, []int8{4, 5}, input.QueryInts8)
					assert.Equal(t, []int16{4, 5}, input.QueryInts16)
					assert.Equal(t, []int32{4, 5}, input.QueryInts32)
					assert.Equal(t, []int64{4, 5}, input.QueryInts64)
					assert.Equal(t, []uint{1, 2}, input.QueryUints)
					assert.Equal(t, []uint16{10, 15}, input.QueryUints16)
					assert.Equal(t, []uint32{10, 15}, input.QueryUints32)
					assert.Equal(t, []uint64{10, 15}, input.QueryUints64)
					assert.Equal(t, []float32{2.2, 2.3}, input.QueryFloats32)
					assert.Equal(t, []float64{3.2, 3.3}, input.QueryFloats64)
					assert.Equal(t, "foo", input.CookieValue)
					assert.Equal(t, 123, input.CookieInt)
					assert.Equal(t, "bar", input.CookieFull.Value)
					assert.Equal(t, []string{"foo", "bar"}, input.QueryExploded)
					return nil, nil
				})

				// Docs should be available on the param object, not just the schema.
				assert.Equal(t, "Some docs", api.OpenAPI().Paths["/test-params/{string}/{int}/{uuid}"].Get.Parameters[0].Description)

				// `http.Cookie` should be treated as a string.
				assert.Equal(t, "string", api.OpenAPI().Paths["/test-params/{string}/{int}/{uuid}"].Get.Parameters[29].Schema.Type)
			},
			Method: http.MethodGet,
			URL:    "/test-params/foo/123/fba4f46b-4539-4d19-8e3f-a0e629a243b5?string=bar&customString=bar&int=456&before=2023-01-01T12:00:00Z&date=2023-01-01&url=http%3A%2F%2Ffoo.com%2Fbar&uint=1&bool=true&strings=foo,bar&customStrings=foo,bar&ints=2,3&ints8=4,5&ints16=4,5&ints32=4,5&ints64=4,5&uints=1,2&uints16=10,15&uints32=10,15&uints64=10,15&floats32=2.2,2.3&floats64=3.2,3.3&exploded=foo&exploded=bar",
			Headers: map[string]string{
				"string": "baz",
				"int":    "789",
				"date":   "Mon, 01 Jan 2023 12:00:00 GMT",
				"cookie": "one=foo; two=123; three=bar",
			},
		},
		{
			Name: "params-named-numeric-slices",
			Register: func(t *testing.T, api huma.API) {
				type ID int
				type IDs []ID
				type Numbers []int
				type Count uint16
				type Counts []Count
				type Ratio float32
				type Ratios []Ratio

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/named-numeric-slices",
				}, func(ctx context.Context, input *struct {
					IDs     IDs     `query:"ids"`
					Numbers Numbers `query:"numbers"`
					Counts  Counts  `query:"counts"`
					Ratios  Ratios  `query:"ratios"`
				}) (*struct{}, error) {
					assert.Equal(t, IDs{1, 2}, input.IDs)
					assert.Equal(t, Numbers{5, 6}, input.Numbers)
					assert.Equal(t, Counts{3, 4}, input.Counts)
					assert.Equal(t, Ratios{1.5, 2.5}, input.Ratios)
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/named-numeric-slices?ids=1,2&numbers=5,6&counts=3,4&ratios=1.5,2.5",
		},
		{
			Name: "params-named-numeric-slices-validation",
			Register: func(t *testing.T, api huma.API) {
				type ID int
				type IDs []ID

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/named-numeric-slices",
				}, func(ctx context.Context, input *struct {
					IDs IDs `query:"ids" minimum:"1" uniqueItems:"true"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/named-numeric-slices?ids=0,0",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "expected number >= 1")
				assert.Contains(t, resp.Body.String(), "expected array items to be unique")
			},
		},
		{
			Name: "params-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test-params/{int}/{uuid}",
				}, func(ctx context.Context, input *struct {
					PathInt       int       `path:"int"`
					PathUUID      UUID      `path:"uuid"`
					QueryInt      int       `query:"int"`
					QueryFloat    float32   `query:"float"`
					QueryBefore   time.Time `query:"before"`
					QueryDate     time.Time `query:"date" timeFormat:"2006-01-02"`
					QueryURL      url.URL   `query:"url"`
					QueryUint     uint      `query:"uint"`
					QueryBool     bool      `query:"bool"`
					QueryInts     []int     `query:"ints"`
					QueryInts8    []int8    `query:"ints8"`
					QueryInts16   []int16   `query:"ints16"`
					QueryInts32   []int32   `query:"ints32"`
					QueryInts64   []int64   `query:"ints64"`
					QueryUints    []uint    `query:"uints"`
					QueryUints8   []uint8   `query:"uints8"`
					QueryUints16  []uint16  `query:"uints16"`
					QueryUints32  []uint32  `query:"uints32"`
					QueryUints64  []uint64  `query:"uints64"`
					QueryFloats32 []float32 `query:"floats32"`
					QueryFloats64 []float64 `query:"floats64"`
					QueryReq      bool      `query:"req" required:"true"`
					HeaderReq     string    `header:"req" required:"true"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test-params/bad/not-a-uuid?int=bad&float=bad&before=bad&date=bad&url=:&uint=bad&bool=bad&ints=bad&ints8=bad&ints16=bad&ints32=bad&ints64=bad&uints=bad&uints8=bad&uints16=bad&uints32=bad&uints64=bad&floats32=bad&floats64=bad",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)

				var body struct {
					Errors []huma.ErrorDetail `json:"errors"`
				}
				err := json.Unmarshal(resp.Body.Bytes(), &body)
				require.NoError(t, err)

				assert.Len(t, body.Errors, 23)
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "path.int", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid value: invalid UUID length: 10", Location: "path.uuid", Value: "not-a-uuid"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.int", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid float", Location: "query.float", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid date/time for format 2006-01-02T15:04:05.999999999Z07:00", Location: "query.before", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid date/time for format 2006-01-02", Location: "query.date", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid url.URL value", Location: "query.url", Value: ":"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.uint", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid boolean", Location: "query.bool", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.ints", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.ints8", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.ints16", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.ints32", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.ints64", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.uints", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.uints8", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.uints16", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.uints32", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid integer", Location: "query.uints64", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid floating value", Location: "query.floats32", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "invalid floating value", Location: "query.floats64", Value: "bad"})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "required query parameter is missing", Location: "query.req", Value: ""})
				assert.Contains(t, body.Errors, huma.ErrorDetail{Message: "required header parameter is missing", Location: "header.req", Value: ""})
			},
		},
		{
			Name: "param-unsupported-type",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test-params/{ipnet}",
				}, func(ctx context.Context, input *struct {
					PathIPNet net.IPNet `path:"ipnet"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test-params/255.255.0.0",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "unsupported param type: net.IPNet")
			},
		},
		{
			Name: "path-param-required-overrides-tag",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/items/{id}",
				}, func(ctx context.Context, input *struct {
					ID string `path:"id" required:"false"`
				}) (*struct{}, error) {
					return nil, nil
				})

				// Per OpenAPI 3.x spec, path parameters must always be required,
				// even if the struct tag says otherwise. Regression test for #1009.
				param := api.OpenAPI().Paths["/items/{id}"].Get.Parameters[0]
				assert.Equal(t, "id", param.Name)
				assert.Equal(t, "path", param.In)
				assert.True(t, param.Required)
			},
			Method: http.MethodGet,
			URL:    "/items/abc",
		},
		{
			Name: "param-bypass-validation",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method:             http.MethodGet,
					Path:               "/test",
					SkipValidateParams: true,
				}, func(ctx context.Context, input *struct {
					Search uint `query:"search" required:"true"`
				}) (*struct{}, error) {
					// ... do custom validation here ...
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code)
			},
		},
		{
			Name: "parse-with-textunmarshaler",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/{mytext}",
				}, func(ctx context.Context, i *struct {
					MyText MyTextUnmarshaler `path:"mytext"`
				}) (*struct{}, error) {
					assert.Equal(t, "Hello, World!", i.MyText.value)
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
		},
		{
			Name: "parse-uuid-slice",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					MyUUIDs []uuid.UUID `query:"myuuid"`
				}) (*struct{}, error) {
					assert.Equal(t, uuid.MustParse("9993a7ca-4c0f-4f22-b388-2677e0ec91ab"), i.MyUUIDs[0])
					assert.Equal(t, uuid.MustParse("46cfcb8b-473f-4c0a-9eef-2cdcbc68c2bc"), i.MyUUIDs[1])
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?myuuid=9993a7ca-4c0f-4f22-b388-2677e0ec91ab,46cfcb8b-473f-4c0a-9eef-2cdcbc68c2bc",
		},
		{
			Name: "parse-custom-type-slice",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					MyInts []MyCustomUnmarshaler `query:"mynumber"`
				}) (*struct{}, error) {
					assert.Equal(t, 50, i.MyInts[0].value)
					assert.Equal(t, ">50", i.MyInts[0].raw)
					assert.Equal(t, 100, i.MyInts[1].value)
					assert.Equal(t, "<100", i.MyInts[1].raw)

					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?mynumber=>50,<100",
		},
		{
			Name: "parseSliceInto-unmarshal-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					MyInts []MyCustomUnmarshaler `query:"mynumber"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?mynumber=!fail",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "invalid value")
			},
		},
		{
			Name: "parseSliceInto-unsupported-type",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					// []struct{} represents a struct that does not implement TextUnmarshaler
					MyInts []struct{} `query:"mynumber"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?mynumber=fail",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "unsupported param type: []struct {}")
			},
		},
		{
			Name: "parse-with-param-receiver",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					Param OptionalParam[int] `query:"param"`
				}) (*struct{}, error) {
					assert.Equal(t, 42, i.Param.Value)
					assert.True(t, i.Param.IsSet)
					return nil, nil
				})
			},
			URL:    "/test?param=42",
			Method: http.MethodGet,
		},
		{
			Name: "parse-with-param-receiver-time",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					Param OptionalParam[time.Time] `query:"param"`
				}) (*struct{}, error) {
					expectedTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
					assert.True(t, i.Param.Value.Equal(expectedTime))
					assert.True(t, i.Param.IsSet)
					return nil, nil
				})
			},
			URL:    "/test?param=2023-01-01T12:00:00Z",
			Method: http.MethodGet,
		},
		{
			Name: "param-deepObject-struct",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					Test struct {
						Int     int     `json:"int"`
						Uint    uint    `json:"uint"`
						Float   float64 `json:"float"`
						Bool    bool    `json:"bool"`
						String  string  `json:"string"`
						Any     any     `json:"any"`
						Default string  `json:"default,omitempty" default:"foo"`
					} `query:"test,deepObject"`
				}) (*struct{}, error) {
					assert.Equal(t, 1, i.Test.Int)
					assert.Equal(t, uint(12), i.Test.Uint)
					assert.InDelta(t, 123.0, i.Test.Float, 1e-6)
					assert.True(t, i.Test.Bool)
					assert.Equal(t, "foo", i.Test.String)
					assert.Equal(t, "foo", i.Test.Any)
					assert.Equal(t, "foo", i.Test.Default)
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?test[int]=1&test[uint]=12&test[float]=123.0&test[bool]=true&test[string]=foo&test[any]=foo&test2[foo]=a",
		},
		{
			Name: "param-deepObject-map-required",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					Test map[string]string `query:"test,deepObject" required:"true"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "required query parameter is missing")
			},
		},
		{
			Name: "param-deepObject-map",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					Test map[string]string `query:"test,deepObject"`
				}) (*struct{}, error) {
					assert.Equal(t, "foo_a", i.Test["a"])
					assert.Equal(t, "foo_b", i.Test["b"])
					assert.Equal(t, "foo_c", i.Test["c"])
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?test[a]=foo_a&test[b]=foo_b&test[c]=foo_c",
		},
		{
			Name: "param-deepObject-map-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					Test map[string]int `query:"test,deepObject"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?test[a]=foo_a",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "invalid integer")
			},
		},
		{
			Name: "param-deepObject-struct-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, i *struct {
					Test struct {
						Int   int       `json:"int"`
						Uint  uint      `json:"uint"`
						Float float64   `json:"float"`
						Bool  bool      `json:"bool"`
						Date  time.Time `json:"date"`
					} `query:"test,deepObject"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?test[int]=a&test[uint]=a&test[float]=a&test[bool]=a&test[date]=a",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				assert.Contains(t, resp.Body.String(), "invalid integer")
				assert.Contains(t, resp.Body.String(), "invalid float")
				assert.Contains(t, resp.Body.String(), "invalid boolean")
				assert.Contains(t, resp.Body.String(), "unsupported type")
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
					assert.JSONEq(t, `{"name":"foo"}`, string(input.RawBody))
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
			// Media types are case-insensitive (RFC 9110 §8.3.1). A client sending
			// e.g. `Application/Json` must still be matched to the registered
			// `application/json` format rather than rejected with 415.
			Name: "request-body-content-type-case-insensitive",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					assert.Equal(t, "foo", input.Body.Name)
					return nil, nil
				})
			},
			Method:  http.MethodPut,
			URL:     "/body",
			Headers: map[string]string{"Content-Type": "Application/Json; charset=UTF-8"},
			Body:    `{"name":"foo"}`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code, resp.Body.String())
			},
		},
		{
			Name: "request-body-embed",
			Register: func(t *testing.T, api huma.API) {
				type Input struct {
					RawBody []byte
					Body    struct {
						Name string `json:"name"`
					}
				}
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Input
				}) (*struct{}, error) {
					assert.JSONEq(t, `{"name":"foo"}`, string(input.RawBody))
					assert.Equal(t, "foo", input.Body.Name)
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"name":"foo"}`,
		},
		{
			Name: "request-body-description",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
					RequestBody: &huma.RequestBody{
						Description: "A description",
					},
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					assert.Equal(t, "foo", input.Body.Name)
					return nil, nil
				})
				// Note: the description should be set, but *also* the generated
				// schema should be present since we didn't set it up ourselves.
				b, _ := api.OpenAPI().Paths["/body"].Put.RequestBody.MarshalJSON()
				assert.JSONEq(t, `{
					"description": "A description",
					"required": true,
					"content": {
						"application/json": {
							"schema": {
								"$ref": "#/components/schemas/Request"
							}
						}
					}
				}`, string(b))
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"name":"foo"}`,
		},
		{
			Name: "request-body-examples",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
					RequestBody: &huma.RequestBody{
						Description: "A description",
						Content: map[string]*huma.MediaType{
							"application/json": {
								Examples: map[string]*huma.Example{
									"Example 1": {
										Summary: "Example summary",
										Value: struct {
											Name string `json:"name"`
										}{
											Name: "foo",
										},
									},
									"Example 2": {
										Summary: "Example summary",
										Value: struct {
											Name string `json:"name"`
										}{
											Name: "bar",
										},
									},
								},
							},
						},
					},
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					assert.Equal(t, "foo", input.Body.Name)
					return nil, nil
				})
				b, _ := api.OpenAPI().Paths["/body"].Put.RequestBody.MarshalJSON()
				assert.JSONEq(t, `{
					"description": "A description",
					"required": true,
					"content": {
						"application/json": {
							"examples": {
								"Example 1": {
									"summary": "Example summary",
									"value": {
										"name": "foo"
									}
								},
								"Example 2": {
									"summary": "Example summary",
									"value": {
										"name": "bar"
									}
								}
							},
							"schema": {
								"$ref": "#/components/schemas/Request"
							}
						}
					}
				}`, string(b))
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"name":"foo"}`,
		},
		{
			Name: "request-body-nested-struct-readOnly",
			Register: func(t *testing.T, api huma.API) {
				type NestedStruct struct {
					Foo struct {
						Bar string `json:"bar"`
					} `json:"foo" readOnly:"true"`
					Value string `json:"value"`
				}
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body *NestedStruct
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodPost,
			URL:    "/body",
			Body:   `{"value":"test"}`,
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
						// Test defaults for fields in the same linked struct. Even though
						// we have seen the struct before we still need to set the default
						// since it's a new/different field.
						S1 StructWithDefaultField `json:"s1,omitzero"`
						S2 StructWithDefaultField `json:"s2,omitzero"`
					}
				}) (*struct{}, error) {
					assert.Equal(t, "Huma", input.Body.Name)
					assert.Equal(t, 5, input.Body.Count)
					assert.Equal(t, []string{"foo", "bar"}, input.Body.Tags)
					assert.Equal(t, []int{1, 2, 3}, input.Body.Numbers)
					assert.Equal(t, 1, input.Body.Items[0].ID)
					assert.True(t, input.Body.Items[0].Verified)
					assert.Equal(t, "default", input.Body.S1.Field)
					assert.Equal(t, "default", input.Body.S2.Field)
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"items": [{"id": 1}]}`,
		},
		{
			Name: "request-body-pointer-defaults",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body struct {
						// Test defaults for primitive types.
						Name    *string `json:"name,omitempty" default:"Huma"`
						Enabled *bool   `json:"enabled,omitempty" default:"true"`
						// Test defaults for slices of primitives.
						Tags    []*string `json:"tags,omitempty" default:"foo, bar"`
						Numbers []*int    `json:"numbers,omitempty" default:"[1, 2, 3]"`
						// Test defaults for fields within slices of structs.
						Items []*struct {
							ID       int   `json:"id"`
							Verified *bool `json:"verified,omitempty" default:"true"`
						} `json:"items,omitempty"`
					}
				}) (*struct{}, error) {
					assert.Equal(t, "Huma", *input.Body.Name)
					assert.True(t, *input.Body.Enabled)
					assert.Equal(t, []*string{Ptr("foo"), Ptr("bar")}, input.Body.Tags)
					assert.Equal(t, []*int{Ptr(1), Ptr(2), Ptr(3)}, input.Body.Numbers)
					assert.Equal(t, 1, input.Body.Items[0].ID)
					assert.True(t, *input.Body.Items[0].Verified)
					assert.Equal(t, 2, input.Body.Items[1].ID)
					assert.False(t, *input.Body.Items[1].Verified)
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"items": [{"id": 1}, {"id": 2, "verified": false}]}`,
		},
		{
			Name: "request-body-pointer-defaults-set",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body struct {
						// Test defaults for primitive types.
						Name    *string `json:"name,omitempty" default:"Huma"`
						Enabled *bool   `json:"enabled,omitempty" default:"true"`
					}
				}) (*struct{}, error) {
					// Ensure we can send the zero value and it doesn't get overridden.
					assert.Empty(t, *input.Body.Name)
					assert.False(t, *input.Body.Enabled)
					return nil, nil
				})
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"name": "", "enabled": false}`,
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
			Name: "request-body-nameHint",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					} `nameHint:"ANameHint"`
				}) (*struct{}, error) {
					return nil, nil
				})
				assert.Equal(t, "#/components/schemas/ANameHint", api.OpenAPI().Paths["/body"].Put.RequestBody.Content["application/json"].Schema.Ref)
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"name": "Name"}`,
		},
		{
			Name: "request-body-custom-schema",
			Register: func(t *testing.T, api huma.API) {
				api.OpenAPI().Components.Schemas.Map()["Dummy"] = &huma.Schema{
					Type: huma.TypeObject,
					Properties: map[string]*huma.Schema{
						"name": {Type: huma.TypeString},
					},
				}
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/body",
					RequestBody: &huma.RequestBody{
						Content: map[string]*huma.MediaType{
							"application/json": {
								Schema: &huma.Schema{
									Ref: "#/components/schemas/Dummy",
								},
							},
						},
					},
				}, func(ctx context.Context, input *struct {
					Body struct {
						Name string `json:"name"`
					}
				}) (*struct{}, error) {
					return nil, nil
				})
				assert.Equal(t, "#/components/schemas/Dummy", api.OpenAPI().Paths["/body"].Put.RequestBody.Content["application/json"].Schema.Ref)
			},
			Method: http.MethodPut,
			URL:    "/body",
			Body:   `{"name": "Name"}`,
		},
		{
			Name: "request-body-embed-struct",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/body",
				}, func(ctx context.Context, input *struct {
					BodyContainer
				}) (*struct{}, error) {
					assert.Equal(t, "Name", input.Body.Name)
					return nil, nil
				})
			},
			Method: http.MethodPost,
			URL:    "/body",
			Body:   `{"name": "Name"}`,
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
					assert.JSONEq(t, `{"name":"foo"}`, string(input.RawBody))
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
			Name: "request-body-multipart-json-struct",
			Register: func(t *testing.T, api huma.API) {
				type User struct {
					Name  string `json:"name"`
					Email string `json:"email"`
					Age   int    `json:"age"`
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						UserData User          `form:"user" contentType:"application/json" required:"true"`
						File     huma.FormFile `form:"file" contentType:"text/plain"`
					}]
				}) (*struct{}, error) {
					data := input.RawBody.Data()

					// Verify the struct was correctly deserialized from JSON
					assert.Equal(t, "John Doe", data.UserData.Name)
					assert.Equal(t, "john@example.com", data.UserData.Email)
					assert.Equal(t, 30, data.UserData.Age)

					// Verify the file was also processed correctly
					assert.Equal(t, "test.txt", data.File.Filename)
					content, err := io.ReadAll(data.File)
					require.NoError(t, err)
					assert.Equal(t, "Hello World", string(content))

					return nil, nil
				})

				// The generated encoding must advertise the JSON content type so
				// the published spec matches how the field is actually parsed.
				mpContent := api.OpenAPI().Paths["/upload"].Post.RequestBody.Content["multipart/form-data"]
				assert.Equal(t, "application/json", mpContent.Encoding["user"].ContentType)
				assert.Equal(t, "text/plain", mpContent.Encoding["file"].ContentType)
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="user"
Content-Type: application/json

{"name": "John Doe", "email": "john@example.com", "age": 30}
--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello World
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-json-struct-invalid-json",
			Register: func(t *testing.T, api huma.API) {
				type User struct {
					Name  string `json:"name"`
					Email string `json:"email" format:"email"`
					Age   int    `json:"age"`
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						UserData User          `form:"user" contentType:"application/json" required:"true"`
						File     huma.FormFile `form:"file" contentType:"text/plain"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="user"
Content-Type: application/json

this is not valid json
--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello World
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				if ok := assert.Equal(t, http.StatusUnprocessableEntity, resp.Code); ok {
					var errors huma.ErrorModel
					err := json.Unmarshal(resp.Body.Bytes(), &errors)
					require.NoError(t, err)
					assert.Equal(t, "form.user", errors.Errors[0].Location)
				}
			},
		},
		{
			Name: "request-body-multipart-json-struct-invalid-data",
			Register: func(t *testing.T, api huma.API) {
				type User struct {
					Name  string `json:"name"`
					Email string `json:"email" format:"email"`
					Age   int    `json:"age"`
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						UserData User          `form:"user" contentType:"application/json" required:"true"`
						File     huma.FormFile `form:"file" contentType:"text/plain"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="user"
Content-Type: application/json

{"name": "John Doe", "email": "SOME INVALID EMAIL", "age": 30}
--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello World
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				if ok := assert.Equal(t, http.StatusUnprocessableEntity, resp.Code); ok {
					var errors huma.ErrorModel
					err := json.Unmarshal(resp.Body.Bytes(), &errors)
					require.NoError(t, err)
					assert.Equal(t, "form.user.email", errors.Errors[0].Location)
				}
			},
		},
		{
			Name: "request-body-multipart-json-struct-default-set",
			Register: func(t *testing.T, api huma.API) {
				type User struct {
					Name  string `json:"name,omitempty" default:"Default Name"`
					Email string `json:"email,omitempty" format:"email"`
					Age   *int   `json:"age,omitempty" default:"25"`
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						UserData User          `form:"user" contentType:"application/json" required:"true"`
						File     huma.FormFile `form:"file" contentType:"text/plain"`
					}]
				}) (*struct{}, error) {
					data := input.RawBody.Data()

					// Verify the struct was correctly deserialized from JSON
					assert.Equal(t, "Default Name", data.UserData.Name)
					assert.Equal(t, 25, *data.UserData.Age)
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="user"
Content-Type: application/json

{}
--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello World
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-json-array",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						Numbers []int    `form:"numbers" contentType:"application/json"`
						Tags    []string `form:"tags" contentType:"application/json"`
						Count   int      `form:"count" contentType:"application/json"`
						Active  bool     `form:"active" contentType:"application/json"`
					}]
				}) (*struct{}, error) {
					data := input.RawBody.Data()

					// Verify arrays were correctly deserialized from JSON
					assert.Equal(t, []int{1, 2, 3, 4, 5}, data.Numbers)
					assert.Equal(t, []string{"tag1", "tag2", "tag3"}, data.Tags)

					// Verify scalar types were correctly deserialized from JSON
					assert.Equal(t, 42, data.Count)
					assert.True(t, data.Active)

					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="numbers"
Content-Type: application/json

[1, 2, 3, 4, 5]
--SimpleBoundary
Content-Disposition: form-data; name="tags"
Content-Type: application/json

["tag1", "tag2", "tag3"]
--SimpleBoundary
Content-Disposition: form-data; name="count"
Content-Type: application/json

42
--SimpleBoundary
Content-Disposition: form-data; name="active"
Content-Type: application/json

true
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-file-decoded",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						HelloWorld   huma.FormFile   `form:"file" contentType:"text/plain" required:"true"`
						Greetings    []huma.FormFile `form:"greetings" contentType:"text/plain" required:"true"`
						NoTagBinding huma.FormFile   `contentType:"text/plain"`
						UnusedField  string          // Ignored altogether
					}]
				}) (*struct{}, error) {
					fileData := input.RawBody.Data()

					assert.Equal(t, "text/plain", fileData.HelloWorld.ContentType)
					assert.Equal(t, "test.txt", fileData.HelloWorld.Filename)
					assert.Equal(t, len("Hello, World!"), int(fileData.HelloWorld.Size))
					assert.True(t, fileData.HelloWorld.IsSet)
					b, err := io.ReadAll(fileData.HelloWorld)
					require.NoError(t, err)
					assert.Equal(t, "Hello, World!", string(b))

					assert.Equal(t, "text/plain", fileData.NoTagBinding.ContentType)
					assert.True(t, fileData.NoTagBinding.IsSet)
					b, err = io.ReadAll(fileData.NoTagBinding)
					require.NoError(t, err)
					assert.Equal(t, `Use struct field name as fallback when no "form" tag is provided.`, string(b))

					expected := []string{"Hello", "World"}
					for i, e := range expected {
						assert.Equal(t, "text/plain", fileData.Greetings[i].ContentType)
						assert.Equal(t, fmt.Sprintf("greetings_%d.txt", i+1), fileData.Greetings[i].Filename)
						assert.Equal(t, len(e), int(fileData.Greetings[i].Size))
						assert.True(t, fileData.Greetings[i].IsSet)

						b, err := io.ReadAll(fileData.Greetings[i])
						require.NoError(t, err)
						assert.Equal(t, e, string(b))
					}

					return nil, nil
				})

				// Ensure OpenAPI spec is listed as a multipart/form-data upload with
				// the appropriate schema.
				mpContent := api.OpenAPI().Paths["/upload"].Post.RequestBody.Content["multipart/form-data"]
				assert.Equal(t, "text/plain", mpContent.Encoding["file"].ContentType)
				assert.Equal(t, "text/plain", mpContent.Encoding["greetings"].ContentType)
				assert.Equal(t, "object", mpContent.Schema.Type)
				assert.Equal(t, "binary", mpContent.Schema.Properties["file"].Format)
				assert.Equal(t, "binary", mpContent.Schema.Properties["greetings"].Items.Format)

				// Ensure no invalid "in": "form" parameters are present.
				for _, param := range api.OpenAPI().Paths["/upload"].Post.Parameters {
					assert.NotEqual(t, "form", param.In)
				}
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello, World!
--SimpleBoundary
Content-Disposition: form-data; name="greetings"; filename="greetings_1.txt"
Content-Type: text/plain

Hello
--SimpleBoundary
Content-Disposition: form-data; name="greetings"; filename="greetings_2.txt"
Content-Type: text/plain

World
--SimpleBoundary
Content-Disposition: form-data; name="NoTagBinding"; filename="notag.txt"
Content-Type: text/plain

Use struct field name as fallback when no "form" tag is provided.
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-file-decoded-required",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						HelloWorld huma.FormFile   `form:"file" contentType:"text/plain" required:"true"`
						Sentences  []huma.FormFile `form:"greetings" contentType:"text/plain" required:"true"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})

				// Ensure OpenAPI spec is listed as a multipart/form-data upload with
				// the appropriate schema.
				mpContent := api.OpenAPI().Paths["/upload"].Post.RequestBody.Content["multipart/form-data"]
				assert.Equal(t, "text/plain", mpContent.Encoding["file"].ContentType)
				assert.Equal(t, "object", mpContent.Schema.Type)
				assert.Equal(t, "binary", mpContent.Schema.Properties["file"].Format)
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="bad_key_name"; filename="test.txt"
Content-Type: text/plain

Hello, World!
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				if ok := assert.Equal(t, http.StatusUnprocessableEntity, resp.Code); ok {
					var errors huma.ErrorModel
					err := json.Unmarshal(resp.Body.Bytes(), &errors)
					require.NoError(t, err)
					assert.Equal(t, "file", errors.Errors[0].Location)
					assert.Equal(t, "greetings", errors.Errors[1].Location)
				}
			},
		},
		{
			Name: "request-body-multipart-file-decoded-optional",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						HelloWorld huma.FormFile `form:"file" contentType:"text/plain" required:"false"`
					}]
				}) (*struct{}, error) {
					assert.False(t, input.RawBody.Data().HelloWorld.IsSet)
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body:    `--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-file-decoded-bad-cardinality",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						HelloWorld huma.FormFile `form:"file" contentType:"text/plain" required:"true"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello, World!
--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="text.txt"
Content-Type: text/plain

What are you doing here ?
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				if ok := assert.Equal(t, http.StatusUnprocessableEntity, resp.Code); ok {
					var errors huma.ErrorModel
					err := json.Unmarshal(resp.Body.Bytes(), &errors)
					require.NoError(t, err)
					assert.Equal(t, "file", errors.Errors[0].Location)
				}
			},
		},
		{
			Name: "request-body-multipart-file-decoded-invalid-content-type",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						// Expecting 'image/png', will receive 'text/plain'
						Image  huma.FormFile   `form:"file" contentType:"image/png"`
						Images []huma.FormFile `form:"file" contentType:"image/png"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello, World!
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				var errors huma.ErrorModel
				err := json.Unmarshal(resp.Body.Bytes(), &errors)
				require.NoError(t, err)
				assert.Len(t, errors.Errors, 2) // Both single and multiple file receiver should fail
				assert.Equal(t, "file", errors.Errors[0].Location)
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
			},
		},
		{
			Name: "request-body-multipart-file-decoded-text-value-sent-to-file-field",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						Avatar huma.FormFile `form:"avatar" contentType:"image/jpeg, image/png"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="avatar"

test
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				// Should return validation error, not panic
				var errors huma.ErrorModel
				err := json.Unmarshal(resp.Body.Bytes(), &errors)
				require.NoError(t, err)
				assert.Equal(t, "File required", errors.Errors[0].Message)
				assert.Equal(t, "avatar", errors.Errors[0].Location)
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
			},
		},
		{
			Name: "request-body-multipart-file-decoded-text-value-sent-to-optional-file-field",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						Avatar huma.FormFile `form:"avatar" contentType:"image/jpeg, image/png" required:"false"`
					}]
				}) (*struct{}, error) {
					// Field should be empty, not panic
					assert.False(t, input.RawBody.Data().Avatar.IsSet)
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="avatar"

test
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				// Should succeed - optional field just stays empty (returns 204 No Content)
				assert.Equal(t, http.StatusNoContent, resp.Code)
			},
		},
		{
			Name: "request-body-multipart-file-decoded-content-type-default",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						// No contentType tag: default to "application/octet-stream"
						Image huma.FormFile `form:"file" required:"true"`
					}]
				}) (*struct{}, error) {
					fileData := input.RawBody.Data()
					b, err := io.ReadAll(fileData.Image.File)
					require.NoError(t, err)
					assert.Equal(t, "console.log('Hello, World!')", string(b))
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.js"
Content-Type: text/javascript

console.log('Hello, World!')
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-file-decoded-content-type-wildcard",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						File huma.FormFile `form:"file" contentType:"text/*" required:"true"`
					}]
				}) (*struct{}, error) {
					fileData := input.RawBody.Data()
					b, err := io.ReadAll(fileData.File)
					require.NoError(t, err)
					assert.Equal(t, "console.log('Hello, World!')", string(b))
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.js"
Content-Type: text/javascript

console.log('Hello, World!')
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-file-decoded-image",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						Image huma.FormFile `form:"file" contentType:"image/jpeg,image/png" required:"true"`
					}]
				}) (*struct{}, error) {
					fileData := input.RawBody.Data()
					assert.Equal(t, "image/png", fileData.Image.ContentType)
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: func() string {
				file, err := os.Open("docs/docs/huma.png")
				require.NoError(t, err)
				b, err := io.ReadAll(file)
				require.NoError(t, err)
				return fmt.Sprintf(`--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.js"
Content-Type: image/png

%s
--SimpleBoundary--`, string(b))
			}(),
		},
		{
			Name: "request-body-multipart-file-decoded-image-detect-type",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						Image huma.FormFile `form:"file" contentType:"image/jpeg,image/png" required:"true"`
					}]
				}) (*struct{}, error) {
					fileData := input.RawBody.Data()
					assert.Equal(t, "image/png", fileData.Image.ContentType)
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: func() string {
				file, err := os.Open("docs/docs/huma.png")
				require.NoError(t, err)
				b, err := io.ReadAll(file)
				require.NoError(t, err)
				return fmt.Sprintf(`--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.js"

%s
--SimpleBoundary--`, string(b))
			}(),
		},
		{
			Name: "request-body-multipart-file",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody multipart.Form
				}) (*struct{}, error) {
					for name, fh := range input.RawBody.File {
						for _, f := range fh {
							r, err := f.Open()
							require.NoError(t, err)

							b, err := io.ReadAll(r)
							require.NoError(t, err)

							_ = r.Close()

							assert.Equal(t, "test.txt", f.Filename)
							assert.Equal(t, "text/plain", f.Header.Get("Content-Type"))
							assert.Equal(t, "Hello, World!", string(b))
						}
						assert.Equal(t, "file", name)
					}
					return nil, nil
				})

				// Ensure OpenAPI spec is listed as a multipart/form-data upload with
				// the appropriate schema.
				mpContent := api.OpenAPI().Paths["/upload"].Post.RequestBody.Content["multipart/form-data"]
				assert.Equal(t, "object", mpContent.Schema.Type)
				assert.Equal(t, "binary", mpContent.Schema.Properties["filename"].Format)
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello, World!
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-files",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload-files",
				}, func(ctx context.Context, input *struct {
					RawBody multipart.Form
				}) (*struct{}, error) {
					for name, fh := range input.RawBody.File {
						for _, f := range fh {
							r, err := f.Open()
							require.NoError(t, err)

							b, err := io.ReadAll(r)
							require.NoError(t, err)

							_ = r.Close()

							// get the last char of name
							index := name[len(name)-1:]
							assert.Equal(t, "example"+index+".txt", f.Filename)
							assert.Equal(t, "text/plain", f.Header.Get("Content-Type"))
							assert.Equal(t, "Content of example"+index+".txt.", string(b))
						}
					}
					return nil, nil
				})

				// Ensure OpenAPI spec is listed as a multipart/form-data upload with
				// the appropriate schema.
				mpContent := api.OpenAPI().Paths["/upload-files"].Post.RequestBody.Content["multipart/form-data"]
				assert.Equal(t, "object", mpContent.Schema.Type)
				assert.Equal(t, "binary", mpContent.Schema.Properties["filename"].Format)
			},
			Method:  http.MethodPost,
			URL:     "/upload-files",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=AnotherBoundary"},
			Body: `--AnotherBoundary
Content-Disposition: form-data; name="file1"; filename="example1.txt"
Content-Type: text/plain

Content of example1.txt.
--AnotherBoundary
Content-Disposition: form-data; name="file2"; filename="example2.txt"
Content-Type: text/plain

Content of example2.txt.
--AnotherBoundary--`,
		},
		{
			Name: "request-body-multipart-wrong-content-type",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody multipart.Form
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPut,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "wrong/type"},
			Body:    `some-data`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
			},
		},
		{
			Name: "request-body-multipart-invalid-data",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPut,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody multipart.Form
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPut,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body:    `invalid`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
			},
		},
		{
			Name: "request-body-multipart-file-decoded-with-formvalue-required",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						HelloWorld huma.FormFile `form:"file" contentType:"text/plain"`
						MyString   string        `form:"myString" required:"true"`
						MyStrings  []string      `form:"myStrings" required:"true"`
						MyInt      int           `form:"myInt" required:"true"`
						MyInts     []int         `form:"myInts" required:"true"`
					}]
				}) (*struct{}, error) {
					fileData := input.RawBody.Data()

					assert.Equal(t, "text/plain", fileData.HelloWorld.ContentType)
					assert.Equal(t, "test.txt", fileData.HelloWorld.Filename)
					assert.Equal(t, len("Hello, World!"), int(fileData.HelloWorld.Size))
					assert.True(t, fileData.HelloWorld.IsSet)
					b, err := io.ReadAll(fileData.HelloWorld)
					require.NoError(t, err)
					assert.Equal(t, "Hello, World!", string(b))

					assert.Equal(t, "Some string", fileData.MyString)
					assert.Equal(t, []string{"Some other string"}, fileData.MyStrings)
					assert.Equal(t, 42, fileData.MyInt)
					assert.Equal(t, []int{1, 2}, fileData.MyInts)
					return nil, nil
				})

				// Ensure OpenAPI spec is listed as a multipart/form-data upload with
				// the appropriate schema.
				mpContent := api.OpenAPI().Paths["/upload"].Post.RequestBody.Content["multipart/form-data"]
				assert.Equal(t, "text/plain", mpContent.Encoding["file"].ContentType)
				assert.Equal(t, "text/plain", mpContent.Encoding["myString"].ContentType)
				assert.Equal(t, "string", mpContent.Schema.Properties["myString"].Type)
				assert.Contains(t, mpContent.Schema.Required, "myString")
				assert.Equal(t, "array", mpContent.Schema.Properties["myStrings"].Type)
				assert.Equal(t, "string", mpContent.Schema.Properties["myStrings"].Items.Type)
				assert.Contains(t, mpContent.Schema.Required, "myStrings")
				assert.Equal(t, "integer", mpContent.Schema.Properties["myInt"].Type)
				assert.Contains(t, mpContent.Schema.Required, "myInt")
				assert.Equal(t, "array", mpContent.Schema.Properties["myInts"].Type)
				assert.Equal(t, "integer", mpContent.Schema.Properties["myInts"].Items.Type)
				assert.Contains(t, mpContent.Schema.Required, "myInts")
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="file"; filename="test.txt"
Content-Type: text/plain

Hello, World!
--SimpleBoundary
Content-Disposition: form-data; name="myString"
Content-Type: text/plain

Some string
--SimpleBoundary
Content-Disposition: form-data; name="myStrings"
Content-Type: text/plain

Some other string
--SimpleBoundary
Content-Disposition: form-data; name="myInt"
Content-Type: text/plain

42
--SimpleBoundary
Content-Disposition: form-data; name="myInts"
Content-Type: text/plain

1
--SimpleBoundary
Content-Disposition: form-data; name="myInts"
Content-Type: text/plain

2
--SimpleBoundary--`,
		},
		{
			Name: "request-body-multipart-file-decoded-with-formvalue-required-missing",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						MyString string `form:"myString" required:"true"`
						MyInt    int    `form:"myInt" required:"true"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body:    `--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				if ok := assert.Equal(t, http.StatusUnprocessableEntity, resp.Code); ok {
					var errors huma.ErrorModel
					err := json.Unmarshal(resp.Body.Bytes(), &errors)
					require.NoError(t, err)
					assert.Equal(t, "form.myString", errors.Errors[0].Location)
					assert.Equal(t, "form.myInt", errors.Errors[1].Location)
				}
			},
		},
		{
			Name: "request-body-multipart-file-decoded-with-formvalue-invalid",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/upload",
				}, func(ctx context.Context, input *struct {
					RawBody huma.MultipartFormFiles[struct {
						MyString   string `form:"myString" maxLength:"3"`
						MyInt      int    `form:"myInt" minimum:"500"`
						MyOtherInt int    `form:"myOtherInt"`
					}]
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodPost,
			URL:     "/upload",
			Headers: map[string]string{"Content-Type": "multipart/form-data; boundary=SimpleBoundary"},
			Body: `--SimpleBoundary
Content-Disposition: form-data; name="myString"
Content-Type: text/plain

Your favorite sender
--SimpleBoundary
Content-Disposition: form-data; name="myInt"
Content-Type: text/plain

42
--SimpleBoundary
Content-Disposition: form-data; name="myOtherInt"
Content-Type: text/plain

1
--SimpleBoundary
Content-Disposition: form-data; name="myOtherInt"
Content-Type: text/plain

2
--SimpleBoundary--`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				if ok := assert.Equal(t, http.StatusUnprocessableEntity, resp.Code); ok {
					var errors huma.ErrorModel
					err := json.Unmarshal(resp.Body.Bytes(), &errors)
					require.NoError(t, err)
					assert.Equal(t, "form.myString", errors.Errors[0].Location)
					assert.Contains(t, errors.Errors[0].Message, "expected length \u003c= 3")
					assert.Equal(t, "form.myInt", errors.Errors[1].Location)
					assert.Contains(t, errors.Errors[1].Message, "expected number \u003e= 500")
					assert.Equal(t, "form.myOtherInt", errors.Errors[2].Location)
					assert.Contains(t, errors.Errors[2].Message, "expected at most one value, but received multiple values")
				}
			},
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
			Name: "handler-wrapped-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/error",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, fmt.Errorf("wrapped: %w", huma.Error403Forbidden("nope"))
				})
			},
			Method: http.MethodGet,
			URL:    "/error",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusForbidden, resp.Code)
			},
		},
		{
			Name: "handler-generic-error",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/error",
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, errors.New("whoops")
				})
			},
			Method: http.MethodGet,
			URL:    "/error",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusInternalServerError, resp.Code)
			},
		},
		{
			Name: "response-headers",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Str          string    `header:"str"`
					Int          int       `header:"int"`
					Uint         uint      `header:"uint"`
					Float        float64   `header:"float"`
					Bool         bool      `header:"bool"`
					Date         time.Time `header:"date"`
					Empty        string    `header:"empty"`
					CustomTime   time.Time `header:"custom-time" timeFormat:"2006-01-02"`
					WithoutTag   string    // No header tag - SHOULD be set as a header using field name.
					LastModified time.Time // No header tag - SHOULD be set as a header using field name.
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-headers",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					return &Resp{
						Str:          "str",
						Int:          1,
						Uint:         2,
						Float:        3.45,
						Bool:         true,
						Date:         time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
						CustomTime:   time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC),
						WithoutTag:   "without-tag-value",
						LastModified: time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC),
					}, nil
				})

				headers := api.OpenAPI().Paths["/response-headers"].Get.Responses["204"].Headers

				// Surface-level fields with explicit tags should be documented.
				assert.NotNil(t, headers["str"])
				assert.NotNil(t, headers["int"])
				assert.NotNil(t, headers["uint"])
				assert.NotNil(t, headers["float"])
				assert.NotNil(t, headers["bool"])
				assert.NotNil(t, headers["date"])
				assert.NotNil(t, headers["empty"])
				assert.NotNil(t, headers["custom-time"])

				// Surface-level fields without tags should be documented using field name.
				assert.NotNil(t, headers["WithoutTag"])
				assert.NotNil(t, headers["LastModified"])
			},
			Method: http.MethodGet,
			URL:    "/response-headers",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code)

				// Surface-level fields with explicit tags should be set.
				assert.Equal(t, "str", resp.Header().Get("Str"))
				assert.Equal(t, "1", resp.Header().Get("Int"))
				assert.Equal(t, "2", resp.Header().Get("Uint"))
				assert.Equal(t, "3.45", resp.Header().Get("Float"))
				assert.Equal(t, "true", resp.Header().Get("Bool"))
				assert.Equal(t, "Sun, 01 Jan 2023 12:00:00 GMT", resp.Header().Get("Date"))
				assert.Empty(t, resp.Header().Values("Empty"))
				assert.Equal(t, "2023-06-15", resp.Header().Get("Custom-Time"))

				// Surface-level fields without tags should be set using field name.
				assert.Equal(t, "without-tag-value", resp.Header().Get("WithoutTag"))
				assert.Equal(t, "Thu, 15 Jun 2023 10:30:00 GMT", resp.Header().Get("LastModified"))
			},
		},
		{
			Name: "response-headers-hidden",
			Register: func(t *testing.T, api huma.API) {
				type HiddenHeaders struct {
					HiddenWithTag    string `header:"X-Hidden-With-Tag"`
					HiddenWithoutTag string // No header tag - should be set as header using field name.
				}

				type Resp struct {
					*HiddenHeaders `hidden:"true"`

					VisibleWithTag    string    `header:"X-Visible-With-Tag"`
					VisibleWithoutTag string    // No header tag - SHOULD be set as a header using field name.
					LastModified      time.Time // No header tag - SHOULD be set as a header using field name.
					Body              struct {
						Message string `json:"message"`
					}
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-headers-hidden",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					return &Resp{
						HiddenHeaders: &HiddenHeaders{
							HiddenWithTag:    "hidden-with-tag-value",
							HiddenWithoutTag: "should-be-header",
						},
						VisibleWithTag:    "visible-with-tag-value",
						VisibleWithoutTag: "visible-without-tag-value",
						LastModified:      time.Date(2023, 6, 15, 10, 30, 0, 0, time.UTC),
						Body: struct {
							Message string `json:"message"`
						}{
							Message: "Hello",
						},
					}, nil
				})

				headers := api.OpenAPI().Paths["/response-headers-hidden"].Get.Responses["200"].Headers

				// Hidden headers should NOT appear in OpenAPI documentation.
				assert.Nil(t, headers["X-Hidden-With-Tag"], "hidden header with tag should not appear in OpenAPI docs")
				assert.Nil(t, headers["HiddenWithoutTag"], "hidden header without tag should not appear in OpenAPI docs")

				// Visible surface-level fields should appear in OpenAPI documentation.
				assert.NotNil(t, headers["X-Visible-With-Tag"], "visible header with tag should appear in OpenAPI docs")
				assert.NotNil(t, headers["VisibleWithoutTag"], "visible header without tag should appear in OpenAPI docs")
				assert.NotNil(t, headers["LastModified"], "visible time header should appear in OpenAPI docs")
			},
			Method: http.MethodGet,
			URL:    "/response-headers-hidden",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)

				// Hidden headers with explicit tag SHOULD still be sent at runtime.
				assert.Equal(t, "hidden-with-tag-value", resp.Header().Get("X-Hidden-With-Tag"))

				// Hidden headers without tag SHOULD still be sent at runtime using field name.
				assert.Equal(t, "should-be-header", resp.Header().Get("HiddenWithoutTag"))

				// Visible surface-level fields should be sent at runtime.
				assert.Equal(t, "visible-with-tag-value", resp.Header().Get("X-Visible-With-Tag"))
				assert.Equal(t, "visible-without-tag-value", resp.Header().Get("VisibleWithoutTag"))
				assert.Equal(t, "Thu, 15 Jun 2023 10:30:00 GMT", resp.Header().Get("LastModified"))
			},
		},
		{
			Name: "response-cookie",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					SetCookie http.Cookie `header:"Set-Cookie"`
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-cookie",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.SetCookie = http.Cookie{
						Name:  "foo",
						Value: "bar",
					}
					return resp, nil
				})

				// `http.Cookie` should be treated as a string.
				assert.Equal(t, "string", api.OpenAPI().Paths["/response-cookie"].Get.Responses["204"].Headers["Set-Cookie"].Schema.Type)
			},
			Method: http.MethodGet,
			URL:    "/response-cookie",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code)
				assert.Equal(t, "foo=bar", resp.Header().Get("Set-Cookie"))
			},
		},
		{
			Name: "response-cookies",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					SetCookie []http.Cookie `header:"Set-Cookie"`
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-cookies",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.SetCookie = []http.Cookie{
						{
							Name:  "foo",
							Value: "bar",
						},
						{
							Name:  "baz",
							Value: "123",
						},
					}
					return resp, nil
				})

				// `[]http.Cookie` should be treated as a string.
				assert.Equal(t, "string", api.OpenAPI().Paths["/response-cookies"].Get.Responses["204"].Headers["Set-Cookie"].Schema.Type)
			},
			Method: http.MethodGet,
			URL:    "/response-cookies",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code)
				assert.Equal(t, "foo=bar", resp.Header()["Set-Cookie"][0])
				assert.Equal(t, "baz=123", resp.Header()["Set-Cookie"][1])
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
			Name: "unknown accept header with JSON format fallback",
			Config: huma.Config{
				OpenAPI: &huma.OpenAPI{
					OpenAPI: "3.1.0",
				},
				Formats:          huma.DefaultFormats,
				NoFormatFallback: false, // Explicitly setting false for clarity.
			},
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Body struct {
						Value string `json:"value"`
					}
				}
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-accept",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					out := new(Resp)
					out.Body.Value = "hello"
					return out, nil
				})
			},
			Method: http.MethodGet,
			Headers: map[string]string{
				"Accept": "custom/dne",
			},
			URL: "/response-accept",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.Equal(t, "application/json", resp.Header().Get("Content-Type"))
				assert.JSONEq(t, `{"value":"hello"}`, resp.Body.String())
			},
		},
		{
			Name: "unknown accept header with disabled format fallback returns 406",
			Config: huma.Config{
				OpenAPI: &huma.OpenAPI{
					OpenAPI: "3.1.0",
				},
				Formats:          huma.DefaultFormats,
				NoFormatFallback: true,
			},
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Body struct {
						Value string `json:"value"`
					}
				}
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-accept",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					out := new(Resp)
					out.Body.Value = "hello"
					return out, nil
				})
			},
			Method: http.MethodGet,
			Headers: map[string]string{
				"Accept": "custom/dne",
			},
			URL: "/response-accept",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNotAcceptable, resp.Code)
				assert.Equal(t, "application/problem+json", resp.Header().Get("Content-Type"))
				assert.JSONEq(t, `{"title":"Not Acceptable","status":406,"detail":"unable to marshal response","errors":[{"message":"unknown accept content type"}]}`, resp.Body.String())
			},
		},
		{
			Name: "response-body-nameHint",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Body struct {
						Greeting string `json:"greeting" `
					} `nameHint:"GreetingResp"`
				}
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response",
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					resp := &Resp{}
					resp.Body.Greeting = "Hello, world!"
					return resp, nil
				})
				assert.Equal(t, "#/components/schemas/GreetingResp", api.OpenAPI().Paths["/response"].Get.Responses["200"].Content["application/json"].Schema.Ref)
			},
			Method: http.MethodGet,
			URL:    "/response",
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
				assert.JSONEq(t, `{"$schema": "http://localhost/schemas/RespBody.json", "greeting":"Hello, world!"}`, resp.Body.String())
			},
		},
		{
			Name: "response-nil",
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
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/response",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				// This should not panic and should return the default status code,
				// which for responses which normally have a body is 200.
				assert.Equal(t, http.StatusOK, resp.Code)
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
			Name: "response-image",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					ContentType string `header:"Content-Type"`
					Body        []byte
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response-image",
					Responses: map[string]*huma.Response{
						"200": {
							Description: "Image response",
							Content: map[string]*huma.MediaType{
								"image/png": {
									Schema: &huma.Schema{Type: "string", Format: "binary"},
								},
							},
						},
					},
				}, func(ctx context.Context, input *struct{}) (*Resp, error) {
					return &Resp{ContentType: "image/png", Body: []byte("abc")}, nil
				})

				// Ensure the OpenAPI spec is correct.
				assert.Len(t, api.OpenAPI().Paths["/response-image"].Get.Responses["200"].Content, 1)
				assert.NotEmpty(t, api.OpenAPI().Paths["/response-image"].Get.Responses["200"].Content["image/png"])
			},
			Method: http.MethodGet,
			URL:    "/response-image",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.Equal(t, `abc`, resp.Body.String())
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
			Name: "response-transform-nil-body",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				huma.Get(api, "/transform", func(ctx context.Context, i *struct{}) (*struct {
					Body *struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body *struct {
							Field string `json:"field"`
						}
					}{}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/transform",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
			},
		},
		{
			Name: "response-transform-nil-interface-body",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				huma.Get(api, "/transform", func(ctx context.Context, i *struct{}) (*struct {
					Body any
				}, error) {
					return &struct {
						Body any
					}{Body: nil}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/transform",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.JSONEq(t, `null`, resp.Body.String())
			},
		},
		{
			Name: "schema-url-from-x-forwarded-host",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Headers: map[string]string{
				"X-Forwarded-Host": "api.example.com",
			},
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "https://api.example.com/schemas/Get-testResponse.json", body["$schema"])
			},
		},
		{
			Name: "schema-url-from-forwarded-header",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Headers: map[string]string{
				"Forwarded": "for=192.0.2.1;host=api.example.com;proto=https",
			},
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "https://api.example.com/schemas/Get-testResponse.json", body["$schema"])
			},
		},
		{
			Name: "schema-url-x-forwarded-host-takes-precedence",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Headers: map[string]string{
				"X-Forwarded-Host": "preferred.example.com",
				"Forwarded":        "host=fallback.example.com",
			},
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "https://preferred.example.com/schemas/Get-testResponse.json", body["$schema"])
			},
		},
		{
			Name: "schema-url-localhost-fallback",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "http://localhost/schemas/Get-testResponse.json", body["$schema"])
			},
		},
		{
			Name: "schema-url-openapi-server-fallback",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				api.OpenAPI().Servers = []*huma.Server{
					{URL: "https://api.production.com"},
				}
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "https://api.production.com/schemas/Get-testResponse.json", body["$schema"])
			},
		},
		{
			Name: "schema-url-openapi-server-without-scheme",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				api.OpenAPI().Servers = []*huma.Server{
					{URL: "api.production.com"},
				}
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "https://api.production.com/schemas/Get-testResponse.json", body["$schema"])
			},
		},
		{
			Name: "schema-url-forwarded-host-overrides-openapi-server",
			Transformers: []huma.Transformer{
				huma.NewSchemaLinkTransformer("/", "/").Transform,
			},
			Register: func(t *testing.T, api huma.API) {
				api.OpenAPI().Servers = []*huma.Server{
					{URL: "https://api.production.com"},
				}
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Headers: map[string]string{
				"X-Forwarded-Host": "custom.example.com",
			},
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "https://custom.example.com/schemas/Get-testResponse.json", body["$schema"])
			},
		},
		{
			Name: "response-transform-error",
			Transformers: []huma.Transformer{
				func(ctx huma.Context, status string, v any) (any, error) {
					return nil, http.ErrNotSupported
				},
			},
			Register: func(t *testing.T, api huma.API) {
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					called := false
					defer func() {
						if err := recover(); err != nil {
							// Ensure the error is the one we expect, possibly wrapped with
							// additional info.
							assert.ErrorIs(t, err.(error), http.ErrNotSupported)
						}
						called = true
					}()
					next(ctx)
					assert.True(t, called)
				})
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
			Name: "schema-url-relative-openapi-server",
			Register: func(t *testing.T, api huma.API) {
				api.OpenAPI().Servers = []*huma.Server{
					{URL: "/base"},
				}
				huma.Get(api, "/test", func(ctx context.Context, i *struct{}) (*struct {
					Body struct {
						Field string `json:"field"`
					}
				}, error) {
					return &struct {
						Body struct {
							Field string `json:"field"`
						}
					}{Body: struct {
						Field string `json:"field"`
					}{Field: "value"}}, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)

				// Verify the OpenAPI spec reflects the expected example URL
				// This triggers the `addSchemaField` logic in `transforms.go`
				// We can check this on the API object if we have access to it,
				// but here we can just verify the Link header or the body.
				assert.Contains(t, resp.Header().Get("Link"), "/base/schemas/Get-testResponse.json")

				var body map[string]any
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				// Note: at runtime the $schema field in the body depends on the request host.
				// By default in tests it's http://localhost
				assert.Contains(t, body["$schema"], "/base/schemas/Get-testResponse.json")
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
			Name: "response-external-schema",
			Register: func(t *testing.T, api huma.API) {
				type Resp struct {
					Body struct {
						Greeting string `json:"greeting"`
					}
				}

				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/response",
					Responses: map[string]*huma.Response{
						"200": {
							Description: "Success",
							Content: map[string]*huma.MediaType{
								"application/json": {
									Schema: &huma.Schema{
										// Using an external schema should not break.
										// https://github.com/danielgtaylor/huma/issues/703
										Ref: "http://example.com/schemas/foo.json",
									},
								},
							},
						},
					},
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
				assert.JSONEq(t, `{"greeting":"Hello, world!"}`, resp.Body.String())
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
			Name: "recursive schema",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/recursive-schema",
				}, func(ctx context.Context, input *struct {
					Body Node
				}) (*struct {
					Body Node
				}, error) {
					return input, nil
				})
			},
			Method: http.MethodPost,
			URL:    "/recursive-schema",
			Body:   `{"name": "root", "nodes": [{"name": "child"}], "left": {"name": "left"}, "named": {"child1": {"name": "child1"}}}`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusOK, resp.Code)
				assert.JSONEq(t, `{"name": "root", "nodes": [{"name": "child"}], "left": {"name": "left"}, "named": {"child1": {"name": "child1"}}}`, resp.Body.String())
			},
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
		{
			Name: "reject-unknown-query-params",
			Config: func() huma.Config {
				cfg := huma.DefaultConfig("Test API", "1.0.0")
				cfg.RejectUnknownQueryParameters = true
				return cfg
			}(),
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, input *struct {
					Known string `query:"known"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?known=ok&unknown=bad",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				var body huma.ErrorModel
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "validation failed", body.Detail)
				found := false
				for _, e := range body.Errors {
					if e.Message == "unknown query parameter" && e.Location == "query.unknown" {
						found = true
						break
					}
				}
				require.True(t, found, "expected unknown query parameter error for query.unknown; got: %v", body.Errors)
			},
		},
		{
			Name: "reject-unknown-query-params-group",
			Config: func() huma.Config {
				cfg := huma.DefaultConfig("Test API", "1.0.0")
				cfg.RejectUnknownQueryParameters = true
				return cfg
			}(),
			Register: func(t *testing.T, api huma.API) {
				g := huma.NewGroup(api, "/base")
				huma.Register(g, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, input *struct {
					Known string `query:"known"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/base/test?known=ok&unknown=bad",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				var body huma.ErrorModel
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "validation failed", body.Detail)
				found := false
				for _, e := range body.Errors {
					if e.Message == "unknown query parameter" && e.Location == "query.unknown" {
						found = true
						break
					}
				}
				require.True(t, found, "expected unknown query parameter error for query.unknown; got: %v", body.Errors)
			},
		},
		{
			Name: "reject-unknown-query-params-deepobject-allowed",
			Config: func() huma.Config {
				cfg := huma.DefaultConfig("Test API", "1.0.0")
				cfg.RejectUnknownQueryParameters = true
				return cfg
			}(),
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, input *struct {
					Test struct {
						Int    int    `json:"int"`
						String string `json:"string"`
					} `query:"test,deepObject"`
				}) (*struct{}, error) {
					// Should parse and succeed, no validation error.
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?test[int]=1&test[string]=foo",
			// No Assert: default check ensures status < 300.
		},
		{
			Name: "reject-unknown-query-params-skip-non-query",
			Config: func() huma.Config {
				cfg := huma.DefaultConfig("Test API", "1.0.0")
				cfg.RejectUnknownQueryParameters = true
				return cfg
			}(),
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/hdr",
				}, func(ctx context.Context, input *struct {
					Header string `header:"X-Test"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method:  http.MethodGet,
			URL:     "/hdr",
			Headers: map[string]string{"X-Test": "ok"},
		},
		{
			Name: "resolver-skip-nil-optional",
			Register: func(t *testing.T, api huma.API) {
				resolverCalls = 0
				huma.Register(api, huma.Operation{
					Method: http.MethodPost,
					Path:   "/opt",
				}, func(ctx context.Context, input *struct {
					Body struct {
						Optional *CountingInner `json:"optional,omitempty"`
					}
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodPost,
			URL:    "/opt",
			Body:   `{}`,
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusNoContent, resp.Code)
				assert.Equal(t, 0, resolverCalls)
			},
		},
		{
			Name: "reject-unknown-query-params-deepobject-unknown",
			Config: func() huma.Config {
				cfg := huma.DefaultConfig("Test API", "1.0.0")
				cfg.RejectUnknownQueryParameters = true
				return cfg
			}(),
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method: http.MethodGet,
					Path:   "/test",
				}, func(ctx context.Context, input *struct {
					Test struct {
						Int    int    `json:"int"`
						String string `json:"string"`
					} `query:"test,deepObject"`
				}) (*struct{}, error) {
					return nil, nil
				})
			},
			Method: http.MethodGet,
			URL:    "/test?test[int]=1&test[string]=foo&test2[foo]=a",
			Assert: func(t *testing.T, resp *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
				var body huma.ErrorModel
				require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
				assert.Equal(t, "validation failed", body.Detail)
				found := false
				for _, e := range body.Errors {
					if e.Message == "unknown query parameter" && e.Location == "query.test2[foo]" {
						found = true
						break
					}
				}
				require.True(t, found, "expected unknown query parameter error for query.test2[foo]; got: %v", body.Errors)
			},
		},
		{
			Name: "security-override-public",
			Register: func(t *testing.T, api huma.API) {
				huma.Register(api, huma.Operation{
					Method:   http.MethodGet,
					Path:     "/public",
					Security: []map[string][]string{}, // No security for this call!
				}, func(ctx context.Context, input *struct{}) (*struct{}, error) {
					return nil, nil
				})
				// Note: the empty security object should be serialized as an empty
				// array in the OpenAPI document.
				b, _ := api.OpenAPI().Paths["/public"].Get.MarshalJSON()
				assert.Contains(t, string(b), `"security":[]`)
			},
			Method: http.MethodGet,
			URL:    "/public",
		},
	} {
		t.Run(feature.Name, func(t *testing.T) {
			r := http.NewServeMux()
			if feature.Config.OpenAPI == nil {
				feature.Config = huma.DefaultConfig("Features Test API", "1.0.0")
			}
			if feature.Transformers != nil {
				feature.Config.Transformers = append(feature.Config.Transformers, feature.Transformers...)
			}
			api := humatest.Wrap(t, humago.New(r, feature.Config))
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
			Recoverer(r).ServeHTTP(w, req)
			b, _ := api.OpenAPI().YAML()
			t.Log(string(b))
			b, _ = httputil.DumpResponse(w.Result(), true)
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

func TestQueryArrayDefaults(t *testing.T) {
	type input struct {
		Exploded []string `query:"exploded,explode" default:"a,b"`
		Plain    []string `query:"plain" default:"a,b"`
		Optional []string `query:"optional,explode"`
	}

	_, api := humatest.New(t)
	var got *input
	huma.Get(api, "/defaults", func(ctx context.Context, in *input) (*struct{}, error) {
		got = in
		return nil, nil
	})

	for _, tc := range []struct {
		name     string
		query    string
		exploded []string
		plain    []string
	}{
		{"absent", "", []string{"a", "b"}, []string{"a", "b"}},
		{"unrelated", "?other=value", []string{"a", "b"}, []string{"a", "b"}},
		{"explicit", "?exploded=x&exploded=y&plain=x,y", []string{"x", "y"}, []string{"x", "y"}},
		{"single", "?exploded=x&plain=x", []string{"x"}, []string{"x"}},
		{"literal comma", "?exploded=x%2Cy", []string{"x,y"}, []string{"a", "b"}},
		{"explicit empty", "?exploded=&plain=", []string{""}, []string{"a", "b"}},
		{"empty first value", "?exploded=&exploded=x", []string{"", "x"}, []string{"a", "b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got = nil
			resp := api.Get("/defaults" + tc.query)
			require.Equal(t, http.StatusNoContent, resp.Code, resp.Body.String())
			assert.Equal(t, tc.exploded, got.Exploded)
			assert.Equal(t, tc.plain, got.Plain)
			assert.Nil(t, got.Optional)
		})
	}
}

// TestMultipartJSONContentTypeMatching ensures the `contentType` tag is matched
// as a media type: parameters like `charset` and structured `+json` suffixes
// still select JSON handling.
func TestMultipartJSONContentTypeMatching(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "1.0.0"))

	type Meta struct {
		Name string `json:"name"`
	}

	huma.Register(api, huma.Operation{
		Method: http.MethodPost,
		Path:   "/upload",
	}, func(ctx context.Context, input *struct {
		RawBody huma.MultipartFormFiles[struct {
			Charset Meta `form:"charset" contentType:"application/json; charset=utf-8"`
			Vendor  Meta `form:"vendor" contentType:"application/vnd.api+json"`
		}]
	}) (*struct{}, error) {
		data := input.RawBody.Data()
		assert.Equal(t, "a", data.Charset.Name)
		assert.Equal(t, "b", data.Vendor.Name)
		return nil, nil
	})

	body := "--B\r\n" +
		"Content-Disposition: form-data; name=\"charset\"\r\n\r\n" +
		`{"name":"a"}` + "\r\n" +
		"--B\r\n" +
		"Content-Disposition: form-data; name=\"vendor\"\r\n\r\n" +
		`{"name":"b"}` + "\r\n" +
		"--B--\r\n"

	r := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(body))
	r.Header.Set("Content-Type", "multipart/form-data; boundary=B")
	w := httptest.NewRecorder()
	api.Adapter().ServeHTTP(w, r)
	assert.Less(t, w.Code, 300, w.Body.String())

	// The generated encoding must reflect the tagged content types.
	enc := api.OpenAPI().Paths["/upload"].Post.RequestBody.Content["multipart/form-data"].Encoding
	assert.Equal(t, "application/json; charset=utf-8", enc["charset"].ContentType)
	assert.Equal(t, "application/vnd.api+json", enc["vendor"].ContentType)
}

// TestMultipartStructFieldRequiresJSONTag ensures a non-parseable struct form
// field without a JSON content type fails fast at registration with actionable
// guidance instead of a confusing request-time error.
func TestMultipartStructFieldRequiresJSONTag(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "1.0.0"))

	type Meta struct {
		Name string `json:"name"`
	}

	assert.PanicsWithError(t,
		`multipart form field 'meta' of type 'huma_test.Meta' requires contentType:"application/json" to be unmarshalled as JSON`,
		func() {
			huma.Register(api, huma.Operation{
				Method: http.MethodPost,
				Path:   "/upload",
			}, func(ctx context.Context, input *struct {
				RawBody huma.MultipartFormFiles[struct {
					Meta Meta `form:"meta"`
				}]
			}) (*struct{}, error) {
				return nil, nil
			})
		})
}

func TestOpenAPI(t *testing.T) {
	r, api := humatest.New(t, huma.DefaultConfig("Features Test API", "1.0.0"))

	// Used to validate exclusion of embedded structs from response headers
	type PaginationHeaders struct {
		Link string `header:"link"`
	}

	type Resp struct {
		PaginationHeaders
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

	for _, url := range []string{
		"/openapi.json",
		"/openapi-3.0.json",
		"/openapi.yaml",
		"/openapi-3.0.yaml",
		"/docs",
		"/schemas/RespBody.json",
	} {
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code, w.Body.String())
	}

	t.Run("ignore-anonymous-header-structs", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/openapi.yaml", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		openapiBody := w.Body.String()
		assert.Equal(t, 200, w.Code, openapiBody)
		assert.Contains(t, openapiBody, "link")
		assert.NotContains(t, openapiBody, "PaginationHeaders")
	})
}

type CTFilterBody struct {
	Field string `json:"field"`
}

func (b *CTFilterBody) ContentType(ct string) string {
	return "application/custom+json"
}

var _ huma.ContentTypeFilter = (*CTFilterBody)(nil)

func TestContentTypeFilter(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Get(api, "/ct-filter", func(ctx context.Context, i *struct{}) (*struct {
		Body CTFilterBody
	}, error) {
		return nil, nil
	})

	responses := api.OpenAPI().Paths["/ct-filter"].Get.Responses["200"].Content
	assert.Len(t, responses, 1)
	for k := range responses {
		assert.Equal(t, "application/custom+json", k)
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
	return []error{errors.New("body resolver error")}
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
				"$schema": "http://localhost/schemas/ErrorModel.json",
		"title": "Unprocessable Entity",
		"status": 422,
		"detail": "validation failed",
		"errors": [
			{
				"message": "expected number <= 10",
				"location": "path.id",
				"value": 15
			}, {
				"message": "expected number >= 1",
				"location": "body.count",
				"value": -6
			}, {
				"message": "expected length <= 10",
				"location": "body.name",
				"value": "12345678901"
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

type WrappedResolverErrorInput struct{}

func (*WrappedResolverErrorInput) Resolve(huma.Context) []error {
	err := huma.ErrorWithHeaders(
		huma.Error401Unauthorized("credentials required"),
		http.Header{
			"WWW-Authenticate": {"Bearer realm=\"protected\""},
			"Set-Cookie":       {"session=; Max-Age=0", "refresh=; Max-Age=0"},
		},
	)
	return []error{fmt.Errorf("resolver failed: %w", err)}
}

func TestWrappedResolverErrorStatusAndHeaders(t *testing.T) {
	router, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	handlerCalled := false
	huma.Get(api, "/resolver-error", func(context.Context, *WrappedResolverErrorInput) (*struct{}, error) {
		handlerCalled = true
		return nil, nil
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/resolver-error", nil))

	assert.Equal(t, http.StatusUnauthorized, resp.Code)
	assert.Equal(t, "Bearer realm=\"protected\"", resp.Header().Get("WWW-Authenticate"))
	assert.Equal(t, []string{"session=; Max-Age=0", "refresh=; Max-Age=0"}, resp.Header().Values("Set-Cookie"))
	assert.False(t, handlerCalled)
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
		return nil, huma.Error404NotFound("not found", errors.New("some-other-error"))
	})

	resp := api.Get("/error", "Host: localhost")
	assert.JSONEq(t, `{"$schema":"http://localhost/schemas/MyError.json","message":"not found","details":["some-other-error"]}`, resp.Body.String())
}

type BrokenWriter struct {
	http.ResponseWriter
}

func (br *BrokenWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("failed writing")
}

func TestClientDisconnect(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	huma.Get(api, "/error", func(ctx context.Context, i *struct{}) (*struct {
		Body string
	}, error) {
		return &struct{ Body string }{Body: "test"}, nil
	})

	// Create and immediately cancel the context. This simulates a client
	// that has disconnected.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/error", nil)

	// Also make the response writer fail when writing.
	recorder := httptest.NewRecorder()
	resp := &BrokenWriter{recorder}

	// We do not want any panics as this is not a real error.
	assert.NotPanics(t, func() {
		api.Adapter().ServeHTTP(resp, req)
	})
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

type FixedArrayResolverItem struct {
	Value    string `json:"value,omitempty" default:"default"`
	Resolved bool   `json:"-"`
	Path     string `json:"-"`
}

func (i *FixedArrayResolverItem) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	i.Resolved = true
	i.Path = prefix.String()
	return nil
}

func TestResolverInFixedArray(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	var got FixedArrayResolverItem
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPost,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		Body struct {
			Items [1]FixedArrayResolverItem `json:"items"`
		}
	}) (*struct{}, error) {
		got = input.Body.Items[0]
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"items":[{}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, "default", got.Value)
	assert.True(t, got.Resolved)
	assert.Equal(t, "body.items[0]", got.Path)
}

// CollectionResolverArray and CollectionResolverSlice have resolvers on the
// collection type itself rather than on an element type.
type CollectionResolverArray [2]float64

func (c *CollectionResolverArray) Resolve(_ huma.Context) []error {
	*c = CollectionResolverArray{1, 2}
	return nil
}

type CollectionResolverSlice []float64

func (c *CollectionResolverSlice) Resolve(_ huma.Context) []error {
	*c = CollectionResolverSlice{3, 4}
	return nil
}

func TestResolverOnCollectionType(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	var got struct {
		Array  CollectionResolverArray
		Slice  CollectionResolverSlice
		Nested []CollectionResolverArray
	}
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPost,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		Body struct {
			Array  CollectionResolverArray   `json:"array"`
			Slice  CollectionResolverSlice   `json:"slice"`
			Nested []CollectionResolverArray `json:"nested"`
		}
	}) (*struct{}, error) {
		got.Array = input.Body.Array
		got.Slice = input.Body.Slice
		got.Nested = input.Body.Nested
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"array":[0,0],"slice":[0],"nested":[[0,0],[0,0]]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, CollectionResolverArray{1, 2}, got.Array)
	assert.Equal(t, CollectionResolverSlice{3, 4}, got.Slice)
	// Each element of the outer slice is itself the match, so the resolver runs
	// on the elements rather than on the slice.
	assert.Equal(t, []CollectionResolverArray{{1, 2}, {1, 2}}, got.Nested)
}

// BothResolverItems and BothResolverItem both have a resolver, so both the
// collection and its elements must be resolved, each exactly once.
type BothResolverItem struct {
	Name  string `json:"name"`
	Count int    `json:"count,omitempty"`
}

func (i *BothResolverItem) Resolve(_ huma.Context) []error {
	i.Count++
	return nil
}

type BothResolverItems []BothResolverItem

func (i *BothResolverItems) Resolve(_ huma.Context) []error {
	*i = append(*i, BothResolverItem{Name: "appended"})
	return nil
}

func TestResolverOnCollectionAndElement(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	var got BothResolverItems
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPost,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		Body struct {
			Items BothResolverItems `json:"items"`
		}
	}) (*struct{}, error) {
		got = input.Body.Items
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"items":[{"name":"a"}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	require.Len(t, got, 2, "the collection resolver should run exactly once")
	assert.Equal(t, "a", got[0].Name)
	assert.Equal(t, 1, got[0].Count, "the element resolver should run exactly once")
}

type MapValueResolverItem struct {
	Value    string `json:"value,omitempty" default:"default"`
	Resolved string `json:"resolved,omitempty"`
}

func (i *MapValueResolverItem) Resolve(_ huma.Context) []error {
	i.Resolved = "yes"
	return nil
}

// TestDefaultsInMapValue covers defaults and resolver mutations on values only
// reachable through a map. Map values are copies, so they are written back to
// the map after being visited, otherwise the changes would be lost.
func TestDefaultsInMapValue(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	var got struct {
		Structs map[string]MapValueResolverItem
		Arrays  map[string][1]MapValueResolverItem
	}
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPost,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		Body struct {
			Structs map[string]MapValueResolverItem    `json:"structs"`
			Arrays  map[string][1]MapValueResolverItem `json:"arrays"`
		}
	}) (*struct{}, error) {
		got.Structs = input.Body.Structs
		got.Arrays = input.Body.Arrays
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"structs":{"a":{}},"arrays":{"a":[{}]}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() { r.ServeHTTP(w, req) })
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, "default", got.Structs["a"].Value)
	assert.Equal(t, "yes", got.Structs["a"].Resolved)
	assert.Equal(t, "default", got.Arrays["a"][0].Value)
	assert.Equal(t, "yes", got.Arrays["a"][0].Resolved)
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

type ResolverCalls struct {
	Calls int
}

func (r *ResolverCalls) Resolve(ctx huma.Context) []error {
	r.Calls++
	return nil
}

func TestResolverCompositionCalledOnce(t *testing.T) {
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPut,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		ResolverCalls
	}) (*struct{}, error) {
		// Exactly one call should have been made to the resolver.
		assert.Equal(t, 1, input.Calls)
		return nil, nil
	})

	req, _ := http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

type ResolverWithPointer struct {
	Ptr *string
}

func (r *ResolverWithPointer) Resolve(ctx huma.Context) []error {
	r.Ptr = new(string)
	*r.Ptr = "String"
	return nil
}

func TestResolverWithPointer(t *testing.T) {
	// Allow using pointers in input structs if they are not path/query/header/cookie parameters
	r, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPut,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		ResolverWithPointer
	}) (*struct{}, error) {
		// Exactly one call should have been made to the resolver.
		assert.Equal(t, "String", *input.Ptr)
		return nil, nil
	})

	req, _ := http.NewRequest(http.MethodPut, "/test", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

func TestParamPointerPanics(t *testing.T) {
	// For now, we don't support these, so we panic rather than have subtle
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
	// For now, we don't support these, so we panic rather than have subtle
	// bugs that are hard to track down.
	_, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	assert.Panics(t, func() {
		huma.Register(app, huma.Operation{
			OperationID: "bug",
			Method:      http.MethodGet,
			Path:        "/bug",
		}, func(ctx context.Context, input *struct {
			Body struct {
				Value *struct {
					Field string `json:"field"`
				} `json:"value,omitempty" default:"{}"`
			}
		}) (*struct{}, error) {
			return nil, nil
		})
	})
}

func TestConvenienceMethods(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	path := "/things"
	type Input struct {
		Owner string `path:"owner"`
		Repo  string `path:"repo"`
	}

	huma.Get(api, path, func(ctx context.Context, input *Input) (*struct {
		Body []struct{}
	}, error) {
		return nil, nil
	}, huma.OperationTags("Things"))
	assert.Equal(t, "list-things", api.OpenAPI().Paths[path].Get.OperationID)
	assert.Equal(t, []string{"Things"}, api.OpenAPI().Paths[path].Get.Tags)

	huma.Post(api, path, func(ctx context.Context, input *Input) (*struct{}, error) {
		return nil, nil
	}, huma.OperationTags("Things"))
	assert.Equal(t, "post-things", api.OpenAPI().Paths[path].Post.OperationID)
	assert.Equal(t, []string{"Things"}, api.OpenAPI().Paths[path].Post.Tags)

	path = path + "/{thing-id}"
	huma.Head(api, path, func(ctx context.Context, input *Input) (*struct{}, error) {
		return nil, nil
	}, huma.OperationTags("Things"))
	assert.Equal(t, "head-things-by-thing-id", api.OpenAPI().Paths[path].Head.OperationID)
	assert.Equal(t, []string{"Things"}, api.OpenAPI().Paths[path].Head.Tags)

	huma.Put(api, path, func(ctx context.Context, input *Input) (*struct{}, error) {
		return nil, nil
	}, huma.OperationTags("Things"))
	assert.Equal(t, "put-things-by-thing-id", api.OpenAPI().Paths[path].Put.OperationID)
	assert.Equal(t, []string{"Things"}, api.OpenAPI().Paths[path].Put.Tags)

	huma.Patch(api, path, func(ctx context.Context, input *Input) (*struct{}, error) {
		return nil, nil
	}, huma.OperationTags("Things"))
	assert.Equal(t, "patch-things-by-thing-id", api.OpenAPI().Paths[path].Patch.OperationID)
	assert.Equal(t, []string{"Things"}, api.OpenAPI().Paths[path].Patch.Tags)

	huma.Delete(api, path, func(ctx context.Context, input *Input) (*struct{}, error) {
		return nil, nil
	}, huma.OperationTags("Things"))
	assert.Equal(t, "delete-things-by-thing-id", api.OpenAPI().Paths[path].Delete.OperationID)
	assert.Equal(t, []string{"Things"}, api.OpenAPI().Paths[path].Delete.Tags)
}

type EmbeddedWithMethod struct{}

func (e EmbeddedWithMethod) Method() {}

func TestUnsupportedEmbeddedTypeWithMethods(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	// Should not panic!
	huma.Post(api, "/things", func(ctx context.Context, input *struct{}) (*struct {
		Body struct {
			EmbeddedWithMethod
		}
	}, error) {
		return nil, nil
	})
}

type SchemaWithExample int

func (*SchemaWithExample) Schema(r huma.Registry) *huma.Schema {
	schema := &huma.Schema{
		Type:     huma.TypeInteger,
		Examples: []any{1},
	}
	return schema
}

func TestSchemaWithExample(t *testing.T) {
	_, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Register(app, huma.Operation{
		OperationID: "test",
		Method:      http.MethodGet,
		Path:        "/test",
	}, func(ctx context.Context, input *struct {
		Test SchemaWithExample `query:"test"`
	}) (*struct{}, error) {
		return nil, nil
	})

	example := app.OpenAPI().Paths["/test"].Get.Parameters[0].Schema.Examples[0]
	assert.Equal(t, 1, example)
}

func TestParameterExampleRedundancy(t *testing.T) {
	_, app := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	type Input struct {
		Name string `query:"name" example:"world"`
	}

	huma.Get(app, "/test", func(ctx context.Context, input *Input) (*struct{}, error) {
		return nil, nil
	})

	param := app.OpenAPI().Paths["/test"].Get.Parameters[0]
	assert.Equal(t, "name", param.Name)

	// The example should be in the schema, not at the parameter level.
	// OpenAPI 3.1+ prefers examples in the schema.
	assert.Nil(t, param.Example)
	require.NotNil(t, param.Schema)
	assert.Equal(t, []any{"world"}, param.Schema.Examples)
}

func TestCustomSchemaErrors(t *testing.T) {
	// Ensure that custom schema errors are correctly reported without having
	// to manually call `schema.PrecomputeMessages()`.
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "test",
		Method:      http.MethodPost,
		Path:        "/test",
		RequestBody: &huma.RequestBody{
			Content: map[string]*huma.MediaType{
				"application/json": {
					Schema: &huma.Schema{
						Type:                 huma.TypeObject,
						Required:             []string{"test"},
						AdditionalProperties: false,
						Properties: map[string]*huma.Schema{
							"test": {
								Type:    huma.TypeInteger,
								Minimum: Ptr(10.0),
							},
						},
					},
				},
			},
		},
	}, func(ctx context.Context, input *struct {
		RawBody []byte
	}) (*struct{}, error) {
		return nil, nil
	})

	resp := api.Post("/test", map[string]any{"test": 1})

	assert.Equal(t, http.StatusUnprocessableEntity, resp.Result().StatusCode)
	assert.Contains(t, resp.Body.String(), `expected number >= 10`)
}

func TestBodyRace(t *testing.T) {
	// Run with the following:
	// go test -run=TestBodyRace -race -parallel=100
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Post(api, "/ping", func(ctx context.Context, input *struct {
		Body struct {
			Value string `json:"value"`
		}
		RawBody []byte
	}) (*struct{}, error) {
		// Access/modify the raw input to detect races.
		input.RawBody[1] = 'a'
		return nil, nil
	})

	for i := range 100 {
		t.Run(fmt.Sprintf("test-%d", i), func(tt *testing.T) {
			tt.Parallel()
			resp := api.Post("/ping", map[string]any{"value": "hello"})
			assert.Equal(tt, 204, resp.Result().StatusCode)
		})
	}
}

type CustomMapValue string

func (v *CustomMapValue) Resolve(ctx huma.Context) []error {
	return nil
}

func TestResolverCustomTypePrimitive(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Post(api, "/test", func(ctx context.Context, input *struct {
		Body struct {
			Tags map[string]CustomMapValue `json:"tags"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})

	assert.NotPanics(t, func() {
		api.Post("/test", map[string]any{"tags": map[string]string{"foo": "bar"}})
	})
}

func TestCustomValidationErrorStatus(t *testing.T) {
	orig := huma.NewError
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		if status == 422 {
			status = 400
		}
		return orig(status, message, errs...)
	}
	t.Cleanup(func() {
		huma.NewError = orig
	})

	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))
	huma.Post(api, "/test", func(ctx context.Context, input *struct {
		Body struct {
			Value string `json:"value" minLength:"5"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})

	resp := api.Post("/test", map[string]any{"value": "foo"})
	assert.Equal(t, http.StatusBadRequest, resp.Result().StatusCode)
	assert.Contains(t, resp.Body.String(), "Bad Request")
}

func TestMinItemsValidation(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test API", "1.0.0"))

	huma.Get(api, "/test", func(ctx context.Context, input *struct {
		Names []string `query:"names" minItems:"2" required:"true"`
	}) (*struct{}, error) {
		return nil, nil
	})

	// 1. Missing query parameter should fail because it is required
	resp := api.Get("/test")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Result().StatusCode)
	assert.Contains(t, resp.Body.String(), "required query parameter is missing")

	// 2. Query parameter with 1 item should fail minItems
	resp = api.Get("/test?names=foo")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.Result().StatusCode)
	assert.Contains(t, resp.Body.String(), "expected array length >= 2")

	huma.Get(api, "/optional", func(ctx context.Context, input *struct {
		Names []string `query:"names" minItems:"2"`
	}) (*struct{}, error) {
		return nil, nil
	})

	// Query Optional (should pass when missing)
	resp = api.Get("/optional")
	assert.Contains(t, []int{200, 204}, resp.Code)

	// But still fail if provided with too few items
	assert.Equal(t, 422, api.Get("/optional?names=foo").Code)
}

// func BenchmarkSecondDecode(b *testing.B) {
// 	//nolint: musttag
// 	type MediumSized struct {
// 		ID   int      `json:"id"`
// 		Name string   `json:"name"`
// 		Tags []string `json:"tags"`
// 		// Created time.Time `json:"created"`
// 		// Updated time.Time `json:"updated"`
// 		Rating float64 `json:"rating"`
// 		Owner  struct {
// 			ID    int    `json:"id"`
// 			Name  string `json:"name"`
// 			Email string `json:"email"`
// 		} `json:"owner"`
// 		Categories []struct {
// 			Name    string   `json:"name"`
// 			Order   int      `json:"order"`
// 			Visible bool     `json:"visible"`
// 			Aliases []string `json:"aliases"`
// 		} `json:"categories"`
// 	}

// 	data := []byte(`{
// 		"id": 123,
// 		"name": "Test",
// 		"tags": ["one", "two", "three"],
// 		"created": "2021-01-01T12:00:00Z",
// 		"updated": "2021-01-01T12:00:00Z",
// 		"rating": 5.0,
// 		"owner": {
// 			"id": 4,
// 			"name": "Alice",
// 			"email": "alice@example.com"
// 		},
// 		"categories": [
// 			{
// 				"name": "First",
// 				"order": 1,
// 				"visible": true
// 			},
// 			{
// 				"name": "Second",
// 				"order": 2,
// 				"visible": false,
// 				"aliases": ["foo", "bar"]
// 			}
// 		]
// 	}`)

// 	pb := huma.NewPathBuffer([]byte{}, 0)
// 	res := &huma.ValidateResult{}
// 	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
// 	fmt.Println("name", reflect.TypeOf(MediumSized{}).Name())
// 	schema := registry.Schema(reflect.TypeOf(MediumSized{}), false, "")

// 	b.Run("json.Unmarshal", func(b *testing.B) {
// 		b.ReportAllocs()
// 		for i := 0; i < b.N; i++ {
// 			var tmp any
// 			if err := json.Unmarshal(data, &tmp); err != nil {
// 				panic(err)
// 			}

// 			huma.Validate(registry, schema, pb, huma.ModeReadFromServer, tmp, res)

// 			var out MediumSized
// 			if err := json.Unmarshal(data, &out); err != nil {
// 				panic(err)
// 			}
// 		}
// 	})

// 	b.Run("mapstructure.Decode", func(b *testing.B) {
// 		b.ReportAllocs()
// 		for i := 0; i < b.N; i++ {
// 			var tmp any
// 			if err := json.Unmarshal(data, &tmp); err != nil {
// 				panic(err)
// 			}

// 			huma.Validate(registry, schema, pb, huma.ModeReadFromServer, tmp, res)

// 			var out MediumSized
// 			if err := mapstructure.Decode(tmp, &out); err != nil {
// 				panic(err)
// 			}
// 		}
// 	})
// }

func globalHandler(_ context.Context, input *struct {
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

func TestGenerateFuncsPanicWithDescriptiveMessage(t *testing.T) {
	var resp *int
	assert.PanicsWithValue(t, "Response type must be a struct", func() {
		huma.GenerateOperationID("GET", "/foo", resp)
	})

	assert.PanicsWithValue(t, "Response type must be a struct", func() {
		huma.GenerateSummary("GET", "/foo", resp)
	})

}

func TestNonJSONValidation(t *testing.T) {
	// Test that validation works when only non-JSON content type is specified.
	// This tests the fix for supporting validation schemas from non-JSON content types.
	type CBORInput struct {
		Body struct {
			Name string `json:"name" minLength:"2" doc:"User name"`
		} `contentType:"application/cbor"`
	}

	// Use default config which includes CBOR via the import above.
	_, api := humatest.New(t, huma.DefaultConfig("Test", "1.0.0"))

	huma.Post(api, "/cbor-test", func(ctx context.Context, input *CBORInput) (*struct{}, error) {
		return &struct{}{}, nil
	})

	// Check that CBOR is in the OpenAPI spec (not JSON).
	op := api.OpenAPI().Paths["/cbor-test"].Post
	assert.NotNil(t, op.RequestBody)
	assert.Contains(t, op.RequestBody.Content, "application/cbor")
	assert.NotContains(t, op.RequestBody.Content, "application/json")

	// Marshal valid data as CBOR.
	var validBuf bytes.Buffer
	huma.DefaultFormats["application/cbor"].Marshal(&validBuf, map[string]any{"name": "John"})

	// Valid CBOR request should work (name >= 2 chars).
	w := api.Post("/cbor-test", "Content-Type: application/cbor", &validBuf)
	assert.Equal(t, 204, w.Code)

	// Marshal invalid data as CBOR (name < 2 chars).
	var invalidBuf bytes.Buffer
	huma.DefaultFormats["application/cbor"].Marshal(&invalidBuf, map[string]any{"name": "J"})

	// Invalid CBOR request should fail validation.
	w = api.Post("/cbor-test", "Content-Type: application/cbor", &invalidBuf)
	assert.Equal(t, 422, w.Code)
}

func TestFieldsOptionalByDefault(t *testing.T) {
	type MyInput struct {
		Body struct {
			Name string `json:"name"`
			Age  int    `json:"age" required:"true"`
		}
	}

	// Default behavior.
	{
		config := huma.DefaultConfig("Test", "1.0.0")
		config.FieldsOptionalByDefault = false
		_, api := humatest.New(t, config)

		huma.Post(api, "/test", func(ctx context.Context, input *MyInput) (*struct{}, error) {
			return nil, nil
		})

		// Missing name should fail because it's required by default.
		resp := api.Post("/test", map[string]any{
			"age": 25,
		})
		assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
		assert.Contains(t, resp.Body.String(), "required property name")
	}

	// Mark fields optional by default.
	{
		config := huma.DefaultConfig("Test", "1.0.0")
		config.FieldsOptionalByDefault = true
		_, api := humatest.New(t, config)

		huma.Post(api, "/test", func(ctx context.Context, input *MyInput) (*struct{}, error) {
			return nil, nil
		})

		// Missing name should pass because it's optional by default.
		resp := api.Post("/test", map[string]any{
			"age": 25,
		})
		assert.Equal(t, http.StatusNoContent, resp.Code)

		// Missing age should still fail because it's explicitly marked as required.
		resp = api.Post("/test", map[string]any{
			"name": "John",
		})
		assert.Equal(t, http.StatusUnprocessableEntity, resp.Code)
		assert.Contains(t, resp.Body.String(), "required property age")
	}
}

func TestSchemaLinkDuplicateTypes(t *testing.T) {
	config := huma.DefaultConfig("Test API", "1.0.0")
	_, api := humatest.New(t, config)

	// Both operations use an identical anonymous struct type for the body.
	// In Go, identical anonymous structs are the same type.
	huma.Get(api, "/test1", func(ctx context.Context, input *struct{}) (*struct {
		Body struct {
			Message string `json:"message"`
		}
	}, error) {
		return &struct {
			Body struct {
				Message string `json:"message"`
			}
		}{
			Body: struct {
				Message string `json:"message"`
			}{Message: "hello from test1"},
		}, nil
	})

	huma.Get(api, "/test2", func(ctx context.Context, input *struct{}) (*struct {
		Body struct {
			Message string `json:"message"`
		}
	}, error) {
		return &struct {
			Body struct {
				Message string `json:"message"`
			}
		}{
			Body: struct {
				Message string `json:"message"`
			}{Message: "hello from test2"},
		}, nil
	})

	// Verify test1.
	resp1 := api.Get("/test1")
	assert.Equal(t, http.StatusOK, resp1.Code)
	// The schema name is derived from OperationID + Response -> Get-test1Response.
	assert.Contains(t, resp1.Body.String(), "Get-test1Response.json")
	assert.Contains(t, resp1.Header().Get("Link"), "Get-test1Response.json")

	// Verify test2.
	resp2 := api.Get("/test2")
	assert.Equal(t, http.StatusOK, resp2.Code)
	// The schema name is derived from OperationID + Response -> Get-test2Response.
	assert.Contains(t, resp2.Body.String(), "Get-test2Response.json")
	assert.Contains(t, resp2.Header().Get("Link"), "Get-test2Response.json")

	// Crucially, they should NOT be the same.
	assert.NotEqual(t, resp1.Header().Get("Link"), resp2.Header().Get("Link"))
}

func TestBodyFallbackContentType(t *testing.T) {
	mux := http.NewServeMux()
	config := huma.DefaultConfig("Test", "1.0.0")
	config.Formats = map[string]huma.Format{
		"test-ct-custom": huma.DefaultJSONFormat,
	}
	config.DefaultFormat = "test-ct-custom"
	api := humago.New(mux, config)

	type HelloInput struct {
		Body struct {
			Name string `json:"name"`
		} `contentType:"test-ct-custom"`
	}

	huma.Register(api, huma.Operation{
		Method: http.MethodPost,
		Path:   "/hello",
	}, func(ctx context.Context, input *HelloInput) (*struct{}, error) {
		assert.Equal(t, "world", input.Body.Name)
		return nil, nil
	})

	// Request without "Content-Type" header.
	req := httptest.NewRequest(http.MethodPost, "/hello", strings.NewReader(`{"name":"world"}`))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestWriteResponseTransformErrorStatus(t *testing.T) {
	// This test verifies that if a transformer fails, the client still receives
	// a 500 Internal Server Error status code.
	config := huma.DefaultConfig("Test API", "1.0.0")
	config.Transformers = append(config.Transformers, func(ctx huma.Context, status string, v any) (any, error) {
		return nil, errors.New("transform error")
	})

	mux := http.NewServeMux()
	api := humago.New(mux, config)

	huma.Get(api, "/test", func(ctx context.Context, input *struct{}) (*struct {
		Status int
		Body   string
	}, error) {
		return &struct {
			Status int
			Body   string
		}{Status: http.StatusInternalServerError, Body: "hello"}, nil
	})

	// Use a custom adapter that doesn't call WriteHeader twice if we can.
	// Actually, just let it panic and recover.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				// We don't want to call WriteHeader here if it was already called.
				// But we can't easily check if it was called on a raw http.ResponseWriter.
				return
			}
		}()
		mux.ServeHTTP(w, r)
	})

	ts := httptest.NewServer(handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/test")
	require.NoError(t, err)
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)

	assert.Equal(t, http.StatusInternalServerError, res.StatusCode)
	assert.Contains(t, string(body), "error transforming response")
}

// Typed validation error values: assertions use public HTTP and transformer seams.
type errorSecret string

func (s errorSecret) MarshalJSON() ([]byte, error) { return json.Marshal("json:" + string(s)) }

type errorText string

func (s errorText) MarshalText() ([]byte, error) { return []byte("text:" + string(s)), nil }

type errorPointer string

func (s *errorPointer) MarshalJSON() ([]byte, error) {
	if s == nil {
		panic("nil marshaler invoked")
	}
	return json.Marshal("pointer:" + string(*s))
}

type errorPointerText string

func (s *errorPointerText) MarshalText() ([]byte, error) { return []byte("ptrtext:" + string(*s)), nil }

type errorBoth string

func (s errorBoth) MarshalText() ([]byte, error)  { return []byte("wrong-text"), nil }
func (s *errorBoth) MarshalJSON() ([]byte, error) { return json.Marshal("both:" + string(*s)) }

type errorStringer string

func (s errorStringer) String() string { return "not-json" }

type errorInt int

func (s errorInt) MarshalJSON() ([]byte, error) { return json.Marshal(fmt.Sprintf("int:%d", s)) }

type errorFloat float64

func (s errorFloat) MarshalJSON() ([]byte, error) { return json.Marshal(fmt.Sprintf("float:%g", s)) }

type errorBool bool

func (s errorBool) MarshalJSON() ([]byte, error) { return json.Marshal(fmt.Sprintf("bool:%t", s)) }

type errorKey string
type errorLeaf struct {
	Secret errorSecret `json:"secret" minLength:"5"`
}
type ErrorEmbedded struct {
	Secret errorSecret `json:"renamed" minLength:"5"`
}
type errorEnvelope struct {
	ErrorEmbedded
	Other errorSecret `json:"other,omitempty" minLength:"5"`
}
type errorRootMap map[string]errorPointer

func (m errorRootMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("map:%d", len(m)))
}

type errorRootSlice []errorSecret

func (s errorRootSlice) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("slice:%d", len(s)))
}

type errorObject struct {
	Secret   errorSecret `json:"secret"`
	Required string      `json:"required"`
}

func (o errorObject) MarshalJSON() ([]byte, error) { return json.Marshal("object:" + string(o.Secret)) }

type errorNormalize string

func (s *errorNormalize) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = errorNormalize("normalized:" + raw)
	return nil
}
func (s errorNormalize) MarshalJSON() ([]byte, error) { return json.Marshal(string(s)) }

type errorReject string

func (s *errorReject) UnmarshalJSON(data []byte) error {
	*s = "partial"
	return errors.New("typed rejection")
}

type errorMutating struct {
	Secret *errorPointer          `json:"secret" minLength:"5"`
	Items  []errorSecret          `json:"items" minItems:"3"`
	Values map[string]errorSecret `json:"values" minProperties:"3"`
}

func (b *errorMutating) Resolve(ctx huma.Context) []error {
	if b.Secret != nil {
		*b.Secret = "changed"
	}
	if len(b.Items) > 0 {
		b.Items[0] = "changed"
	}
	b.Values["a"] = "changed"
	return nil
}

type errorValueResponse struct {
	details []*huma.ErrorDetail
	wire    []map[string]any
	code    int
	called  bool
}

func errorValueRequest[B any](t *testing.T, raw string, modify func(*huma.Schema), contentType ...string) errorValueResponse {
	t.Helper()
	config := huma.DefaultConfig("Typed errors", "1")
	config.CreateHooks = nil
	var result errorValueResponse
	config.Transformers = []huma.Transformer{func(_ huma.Context, _ string, v any) (any, error) {
		if model, ok := v.(*huma.ErrorModel); ok {
			result.details = model.Errors
		}
		return v, nil
	}}
	mux := http.NewServeMux()
	api := humago.New(mux, config)
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[B](), true, "")
	if modify != nil {
		target := schema
		if target.Ref != "" {
			target = api.OpenAPI().Components.Schemas.SchemaFromRef(target.Ref)
		}
		modify(target)
		target.PrecomputeMessages()
	}
	huma.Register(api, huma.Operation{Method: "POST", Path: "/values", OperationID: "values", RequestBody: &huma.RequestBody{Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}}, func(_ context.Context, _ *struct{ Body B }) (*struct{}, error) { result.called = true; return nil, nil })
	req := httptest.NewRequest("POST", "/values", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if len(contentType) > 0 {
		req.Header.Set("Content-Type", contentType[0])
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	result.code = w.Code
	if w.Code >= 400 {
		var document struct {
			Errors []map[string]any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &document), w.Body.String())
		result.wire = document.Errors
	}
	return result
}
func errorValueCheck(t *testing.T, r errorValueResponse, index int, want any, typ reflect.Type) {
	t.Helper()
	require.Equal(t, 422, r.code)
	require.False(t, r.called)
	require.Greater(t, len(r.details), index)
	require.Greater(t, len(r.wire), index)
	assert.Equal(t, want, r.wire[index]["value"])
	if typ != nil {
		v := reflect.ValueOf(r.details[index].Value)
		require.True(t, v.IsValid())
		if v.Kind() == reflect.Pointer && typ.Kind() != reflect.Pointer {
			v = v.Elem()
		}
		require.True(t, v.IsValid())
		assert.Equal(t, typ, v.Type())
	}
	message, _ := r.wire[index]["message"].(string)
	assert.Equal(t, r.details[index].Message, message)
	assert.True(t, strings.HasPrefix(r.details[index].Location, "body"))
}

type errorFoldLowerFirst struct {
	Lower errorSecret  `json:"foo" minLength:"5"`
	Upper errorPointer `json:"FOO" minLength:"5"`
}

type errorFoldUpperFirst struct {
	Upper errorPointer `json:"FOO" minLength:"5"`
	Lower errorSecret  `json:"foo" minLength:"5"`
}

type ErrorFoldPromoted struct {
	Lower errorSecret `json:"foo" minLength:"5"`
}

type errorFoldEmbeddedFirst struct {
	ErrorFoldPromoted
	Upper errorPointer `json:"FOO" minLength:"5"`
}

func checkTypedErrorFoldOrder[B any](t *testing.T, raw string, selected func(*B) any, locations ...string) {
	t.Helper()
	var decoded B
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	want := selected(&decoded)
	encoded, err := json.Marshal(want)
	require.NoError(t, err)
	var serialized any
	require.NoError(t, json.Unmarshal(encoded, &serialized))
	r := errorValueRequest[B](t, raw, nil)
	require.Len(t, r.details, len(locations))
	for i, location := range locations {
		errorValueCheck(t, r, i, serialized, reflect.TypeOf(want))
		assert.Equal(t, location, r.details[i].Location)
	}
}

func TestTypedErrorValuesFoldOrder(t *testing.T) {
	t.Run("lower_declared_first", func(t *testing.T) {
		checkTypedErrorFoldOrder(t, `{"FoO":"one"}`, func(b *errorFoldLowerFirst) any {
			require.Equal(t, errorSecret("one"), b.Lower)
			require.Empty(t, b.Upper)
			return b.Lower
		}, "body.FOO", "body.foo")
	})
	t.Run("upper_declared_first", func(t *testing.T) {
		checkTypedErrorFoldOrder(t, `{"FoO":"one"}`, func(b *errorFoldUpperFirst) any {
			require.Equal(t, errorPointer("one"), b.Upper)
			require.Empty(t, b.Lower)
			return &b.Upper
		}, "body.FOO", "body.foo")
	})
	t.Run("exact_upper_over_earlier_folded", func(t *testing.T) {
		checkTypedErrorFoldOrder(t, `{"FOO":"one"}`, func(b *errorFoldLowerFirst) any {
			require.Equal(t, errorPointer("one"), b.Upper)
			require.Empty(t, b.Lower)
			return &b.Upper
		}, "body.FOO", "body.foo")
	})
	t.Run("exact_lower_over_earlier_folded", func(t *testing.T) {
		checkTypedErrorFoldOrder(t, `{"foo":"one"}`, func(b *errorFoldUpperFirst) any {
			require.Equal(t, errorSecret("one"), b.Lower)
			require.Empty(t, b.Upper)
			return b.Lower
		}, "body.FOO", "body.foo")
	})
	t.Run("promoted_index_before_shallower_field", func(t *testing.T) {
		checkTypedErrorFoldOrder(t, `{"FoO":"one"}`, func(b *errorFoldEmbeddedFirst) any {
			require.Equal(t, errorSecret("one"), b.Lower)
			require.Empty(t, b.Upper)
			return b.Lower
		}, "body.FOO", "body.foo")
	})
	t.Run("exact_shallower_over_promoted_folded", func(t *testing.T) {
		checkTypedErrorFoldOrder(t, `{"FOO":"one"}`, func(b *errorFoldEmbeddedFirst) any {
			require.Equal(t, errorPointer("one"), b.Upper)
			require.Empty(t, b.Lower)
			return &b.Upper
		}, "body.FOO", "body.foo")
	})
	t.Run("unicode_folded", func(t *testing.T) {
		type body struct {
			Lower errorSecret  `json:"σ" minLength:"5"`
			Upper errorPointer `json:"Σ" minLength:"5"`
		}
		checkTypedErrorFoldOrder(t, `{"ς":"one"}`, func(b *body) any {
			require.Equal(t, errorSecret("one"), b.Lower)
			require.Empty(t, b.Upper)
			return b.Lower
		}, "body.Σ", "body.σ")
	})
}

type errorRawNumberBody[N any] struct {
	N N `json:"n" maximum:"1" minimum:"-1"`
}

type errorWireLexeme string

func (v *errorWireLexeme) UnmarshalJSON(data []byte) error {
	*v = errorWireLexeme(data)
	return nil
}
func (v errorWireLexeme) MarshalJSON() ([]byte, error) { return json.Marshal("raw:" + string(v)) }
func (errorWireLexeme) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Enum: []any{"never-selected"}}
}

type errorRawLexicalBody struct {
	Value errorWireLexeme `json:"value"`
}

func checkTypedErrorRawCollision[B any](t *testing.T, raw, location string, want any) {
	t.Helper()
	r := errorValueRequest[B](t, raw, nil)
	require.Len(t, r.details, 1)
	encoded, err := json.Marshal(want)
	require.NoError(t, err)
	var serialized any
	require.NoError(t, json.Unmarshal(encoded, &serialized))
	errorValueCheck(t, r, 0, serialized, reflect.TypeOf(want))
	assert.Equal(t, want, r.details[0].Value)
	assert.Equal(t, location, r.details[0].Location)
}

func TestTypedErrorValuesRawCollisions(t *testing.T) {
	t.Run("int64_precision", func(t *testing.T) {
		checkTypedErrorRawCollision[errorRawNumberBody[int64]](t, `{"n":9007199254740993,"N":1}`, "body.n", int64(9007199254740993))
	})
	t.Run("int64_maximum", func(t *testing.T) {
		checkTypedErrorRawCollision[errorRawNumberBody[int64]](t, `{"n":9223372036854775807,"N":1}`, "body.n", int64(9223372036854775807))
	})
	t.Run("int64_negative", func(t *testing.T) {
		checkTypedErrorRawCollision[errorRawNumberBody[int64]](t, `{"n":-9007199254740993,"N":1}`, "body.n", int64(-9007199254740993))
	})
	t.Run("uint64_maximum", func(t *testing.T) {
		checkTypedErrorRawCollision[errorRawNumberBody[uint64]](t, `{"n":18446744073709551615,"N":1}`, "body.n", uint64(18446744073709551615))
	})
	t.Run("exact_after_folded", func(t *testing.T) {
		checkTypedErrorRawCollision[errorRawNumberBody[int64]](t, `{"N":1,"n":9007199254740993}`, "body.n", int64(9007199254740993))
	})
	t.Run("slice_index", func(t *testing.T) {
		checkTypedErrorRawCollision[[]errorRawNumberBody[int64]](t, `[{"n":0},{"n":9007199254740993,"N":1}]`, "body[1].n", int64(9007199254740993))
	})
	t.Run("array_index", func(t *testing.T) {
		checkTypedErrorRawCollision[[2]errorRawNumberBody[int64]](t, `[{"n":0},{"n":9007199254740993,"N":1}]`, "body[1].n", int64(9007199254740993))
	})
	t.Run("map_literal_key", func(t *testing.T) {
		checkTypedErrorRawCollision[map[string]errorRawNumberBody[int64]](t, `{"a.b[0]":{"n":9007199254740993,"N":1}}`, "body.a.b[0].n", int64(9007199254740993))
	})
	for _, tc := range []struct{ name, raw string }{
		{"nested_pointer", `{"child":{"n":9007199254740993,"N":1}}`},
		{"ancestor_collision", `{"child":{"n":9007199254740993},"CHILD":{"n":1}}`},
		{"two_collisions", `{"child":{"n":9007199254740993,"N":1},"CHILD":{"n":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkTypedErrorRawCollision[struct {
				Child *errorRawNumberBody[int64] `json:"child"`
			}](t, tc.raw, "body.child.n", int64(9007199254740993))
		})
	}
	for _, tc := range []struct{ name, token string }{
		{"exponent", `1e+03`},
		{"negative_zero", `-0`},
		{"escaped_string", `"\u006fn\u0065"`},
		{"escaped_slash", `"o\/ne"`},
		{"object_spacing_order", `{ "z": 1e+03, "a": "\u006f" }`},
		{"array_spacing", `[ 1e+03, "\u006f" ]`},
		{"duplicate_object_members", `{"z":1,"z":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkTypedErrorRawCollision[errorRawLexicalBody](t, `{"value":`+tc.token+`,"VALUE":"replacement"}`, "body.value", errorWireLexeme(tc.token))
		})
	}
	t.Run("lexical_nested_escaped_keys", func(t *testing.T) {
		checkTypedErrorRawCollision[map[string][]errorRawLexicalBody](t, `{"a\u002eb[0]":[{"v\u0061lue":"\u006fne","VALUE":"replacement"}]}`, "body.a.b[0][0].value", errorWireLexeme(`"\u006fne"`))
	})
}

func errorMinLength(s *huma.Schema) { n := 5; s.MinLength = &n }
func errorMinItems(s *huma.Schema)  { n := 3; s.MinItems = &n }

func TestTypedErrorValuesScalars(t *testing.T) {
	checkTypedErrorCompatibility(t)
	t.Run("json", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorSecret](t, `"one"`, errorMinLength), 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("text", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorText](t, `"two"`, errorMinLength), 0, "text:two", reflect.TypeFor[errorText]())
	})
	t.Run("pointer", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorPointer](t, `"red"`, errorMinLength), 0, "pointer:red", reflect.TypeFor[errorPointer]())
	})
	t.Run("pointer_body", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[*errorPointer](t, `"red"`, errorMinLength), 0, "pointer:red", reflect.TypeFor[*errorPointer]())
	})
	t.Run("pointer_text", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorPointerText](t, `"txt"`, errorMinLength), 0, "ptrtext:txt", reflect.TypeFor[errorPointerText]())
	})
	t.Run("precedence", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorBoth](t, `"one"`, errorMinLength), 0, "both:one", reflect.TypeFor[errorBoth]())
	})
	t.Run("stringer", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorStringer](t, `"raw"`, errorMinLength), 0, "raw", reflect.TypeFor[errorStringer]())
	})
	t.Run("integer", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorInt](t, `2`, func(s *huma.Schema) { n := 5.0; s.Minimum = &n }), 0, "int:2", reflect.TypeFor[errorInt]())
	})
	t.Run("float", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorFloat](t, `2.5`, func(s *huma.Schema) { n := 5.0; s.Minimum = &n }), 0, "float:2.5", reflect.TypeFor[errorFloat]())
	})
	t.Run("bool", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorBool](t, `false`, func(s *huma.Schema) { s.Enum = []any{true} }), 0, "bool:false", reflect.TypeFor[errorBool]())
	})
	t.Run("zero", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorSecret](t, `""`, errorMinLength), 0, "json:", reflect.TypeFor[errorSecret]())
	})
	for _, ct := range []string{"application/json; charset=utf-8", "application/problem+json", "APPLICATION/JSON", ""} {
		t.Run("content_"+ct, func(t *testing.T) {
			errorValueCheck(t, errorValueRequest[errorSecret](t, `"one"`, errorMinLength, ct), 0, "json:one", reflect.TypeFor[errorSecret]())
		})
	}
}
func TestTypedErrorValuesShapes(t *testing.T) {
	t.Run("nested", func(t *testing.T) {
		r := errorValueRequest[struct {
			Child *errorLeaf `json:"child"`
		}](t, `{"child":{"secret":"one"}}`, nil)
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
		assert.Equal(t, "body.child.secret", r.details[0].Location)
	})
	t.Run("embedded", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorEnvelope](t, `{"renamed":"one"}`, nil), 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("embedded_pointer", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[struct{ *ErrorEmbedded }](t, `{"renamed":"one"}`, nil), 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("named_embed", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[struct {
			ErrorEmbedded `json:"inner"`
		}](t, `{"inner":{"renamed":"one"}}`, nil), 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("casing", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorLeaf](t, `{"SECRET":"one"}`, nil), 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("exact_folded_collision", func(t *testing.T) {
		r := errorValueRequest[errorLeaf](t, `{"secret":"one","SECRET":"long-enough"}`, nil)
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
		assert.Equal(t, "body.secret", r.details[0].Location)
	})
	t.Run("exact_folded_pointer_collision", func(t *testing.T) {
		r := errorValueRequest[struct {
			Secret *errorPointer `json:"secret" minLength:"5"`
		}](t, `{"secret":"one","SECRET":null}`, nil)
		errorValueCheck(t, r, 0, "pointer:one", reflect.TypeFor[*errorPointer]())
	})
	t.Run("slice", func(t *testing.T) {
		r := errorValueRequest[[]errorLeaf](t, `[{"secret":"one"},{"secret":"two"}]`, nil)
		for i, w := range []string{"json:one", "json:two"} {
			errorValueCheck(t, r, i, w, reflect.TypeFor[errorSecret]())
		}
	})
	t.Run("array", func(t *testing.T) {
		r := errorValueRequest[[2]errorLeaf](t, `[{"secret":"one"},{"secret":"two"}]`, nil)
		for i, w := range []string{"json:one", "json:two"} {
			errorValueCheck(t, r, i, w, reflect.TypeFor[errorSecret]())
		}
	})
	for _, key := range []string{"plain", "a.b", "a[0]", "", "0", "\u03bb", "x.y[2]"} {
		t.Run("map_"+key, func(t *testing.T) {
			data, _ := json.Marshal(map[string]string{key: "one"})
			r := errorValueRequest[map[errorKey]errorPointer](t, string(data), func(s *huma.Schema) { errorMinLength(s.AdditionalProperties.(*huma.Schema)) })
			errorValueCheck(t, r, 0, "pointer:one", reflect.TypeFor[errorPointer]())
		})
	}
	t.Run("map_text", func(t *testing.T) {
		r := errorValueRequest[map[string]errorText](t, `{"key":"one"}`, func(s *huma.Schema) { errorMinLength(s.AdditionalProperties.(*huma.Schema)) })
		errorValueCheck(t, r, 0, "text:one", reflect.TypeFor[errorText]())
	})
	t.Run("map_pointer_text", func(t *testing.T) {
		r := errorValueRequest[map[string]errorPointerText](t, `{"key":"one"}`, func(s *huma.Schema) { errorMinLength(s.AdditionalProperties.(*huma.Schema)) })
		errorValueCheck(t, r, 0, "ptrtext:one", reflect.TypeFor[errorPointerText]())
	})
	t.Run("nested_map_slice", func(t *testing.T) {
		r := errorValueRequest[map[string][]errorLeaf](t, `{"a.b[0]":[{"secret":"one"},{"secret":"two"}]}`, nil)
		for i, w := range []string{"json:one", "json:two"} {
			errorValueCheck(t, r, i, w, reflect.TypeFor[errorSecret]())
		}
	})
	t.Run("dotted_collision", func(t *testing.T) {
		r := errorValueRequest[struct {
			Literal errorSecret `json:"a.b" minLength:"5"`
			A       struct {
				B errorPointer `json:"b" minLength:"5"`
			} `json:"a"`
		}](t, `{"a.b":"one","a":{"b":"two"}}`, nil)
		values := make([]any, 0, len(r.wire))
		for _, e := range r.wire {
			values = append(values, e["value"])
		}
		assert.ElementsMatch(t, []any{"json:one", "pointer:two"}, values)
	})
	t.Run("bracket_collision", func(t *testing.T) {
		r := errorValueRequest[struct {
			Literal errorSecret    `json:"a[0]" minLength:"5"`
			A       []errorPointer `json:"a"`
		}](t, `{"a[0]":"one","a":["two"]}`, func(s *huma.Schema) { errorMinLength(s.Properties["a"].Items) })
		values := make([]any, 0, len(r.wire))
		for _, e := range r.wire {
			values = append(values, e["value"])
		}
		assert.ElementsMatch(t, []any{"json:one", "pointer:two"}, values)
	})
}
func TestTypedErrorValuesContainers(t *testing.T) {
	t.Run("required", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorObject](t, `{"secret":"one"}`, nil), 0, "object:one", reflect.TypeFor[errorObject]())
	})
	t.Run("unexpected", func(t *testing.T) {
		r := errorValueRequest[errorObject](t, `{"secret":"one","required":"ok","extra":"raw"}`, nil)
		errorValueCheck(t, r, 0, "object:one", reflect.TypeFor[errorObject]())
		assert.Equal(t, "body.extra", r.details[0].Location)
	})
	t.Run("dependent", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorObject](t, `{"secret":"one"}`, func(s *huma.Schema) {
			s.Required = nil
			s.DependentRequired = map[string][]string{"secret": {"required"}}
		}), 0, "object:one", reflect.TypeFor[errorObject]())
	})
	t.Run("object_children", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorLeaf](t, `{"secret":"valid","extra":true}`, nil), 0, map[string]any{"secret": "json:valid"}, reflect.TypeFor[errorLeaf]())
	})
	t.Run("map_min", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorRootMap](t, `{"a":"one"}`, func(s *huma.Schema) { n := 2; s.MinProperties = &n }), 0, "map:1", reflect.TypeFor[errorRootMap]())
	})
	t.Run("slice_min", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorRootSlice](t, `["one"]`, errorMinItems), 0, "slice:1", reflect.TypeFor[errorRootSlice]())
	})
	t.Run("empty_slice", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorRootSlice](t, `[]`, errorMinItems), 0, "slice:0", reflect.TypeFor[errorRootSlice]())
	})
	t.Run("empty_map", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorRootMap](t, `{}`, func(s *huma.Schema) { n := 1; s.MinProperties = &n }), 0, "map:0", reflect.TypeFor[errorRootMap]())
	})
	t.Run("unique", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorRootSlice](t, `["one","one"]`, func(s *huma.Schema) { s.UniqueItems = true }), 0, "slice:2", reflect.TypeFor[errorRootSlice]())
	})
	t.Run("array_container", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[[2]errorSecret](t, `["one","two"]`, errorMinItems), 0, []any{"json:one", "json:two"}, reflect.TypeFor[[2]errorSecret]())
	})
	t.Run("multiple", func(t *testing.T) {
		r := errorValueRequest[errorSecret](t, `"one"`, func(s *huma.Schema) { errorMinLength(s); s.Pattern = "^z"; s.Enum = []any{"different"} })
		require.Len(t, r.details, 3)
		for i := range r.details {
			errorValueCheck(t, r, i, "json:one", reflect.TypeFor[errorSecret]())
		}
	})
	for _, composition := range []string{"allOf", "anyOf", "oneOf", "not"} {
		t.Run(composition, func(t *testing.T) {
			r := errorValueRequest[errorSecret](t, `"one"`, func(s *huma.Schema) {
				sub := &huma.Schema{Type: "string"}
				errorMinLength(sub)
				switch composition {
				case "allOf":
					s.AllOf = []*huma.Schema{sub}
				case "anyOf":
					s.AnyOf = []*huma.Schema{sub}
				case "oneOf":
					s.OneOf = []*huma.Schema{sub}
				case "not":
					s.Not = &huma.Schema{Type: "string"}
				}
			})
			errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
		})
	}
}
func checkTypedErrorSnapshotFailure[B any](t *testing.T, raw string, populate func(*B), modify func(*huma.Schema)) {
	t.Helper()
	var standard B
	require.Error(t, json.Unmarshal([]byte(raw), &standard))
	config := huma.DefaultConfig("Decoder divergence", "1")
	config.CreateHooks = nil
	decodes := 0
	format := huma.DefaultJSONFormat
	format.Unmarshal = func(data []byte, target any) error {
		if body, ok := target.(*B); ok {
			decodes++
			populate(body)
			return nil
		}
		return json.Unmarshal(data, target)
	}
	config.Formats = map[string]huma.Format{"application/json": format, "json": format}
	var details []*huma.ErrorDetail
	config.Transformers = []huma.Transformer{func(_ huma.Context, _ string, value any) (any, error) {
		if model, ok := value.(*huma.ErrorModel); ok {
			details = model.Errors
		}
		return value, nil
	}}
	mux := http.NewServeMux()
	api := humago.New(mux, config)
	schema := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[B](), true, "")
	target := schema
	if target.Ref != "" {
		target = api.OpenAPI().Components.Schemas.SchemaFromRef(target.Ref)
	}
	if modify != nil {
		modify(target)
		target.PrecomputeMessages()
	}
	called := false
	huma.Register(api, huma.Operation{Method: "POST", Path: "/", OperationID: "divergence", RequestBody: &huma.RequestBody{Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}}, func(_ context.Context, _ *struct{ Body B }) (*struct{}, error) {
		called = true
		return nil, nil
	})
	req := httptest.NewRequest("POST", "/", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	require.Equal(t, 422, w.Code)
	require.Equal(t, 1, decodes)
	require.False(t, called)
	var model struct {
		Errors []map[string]any `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &model))
	require.Len(t, model.Errors, 2)
	require.Len(t, details, 2)
	for i, detail := range model.Errors {
		_, present := detail["value"]
		assert.False(t, present, "partial typed value leaked: %v", detail)
		assert.Nil(t, details[i].Value)
		assert.True(t, strings.HasPrefix(details[i].Location, "body"))
	}
}

func TestTypedErrorValuesSnapshotFailure(t *testing.T) {
	constraints := func(s *huma.Schema) {
		errorMinLength(s)
		s.Pattern = "^z"
	}
	t.Run("configured_scalar_succeeds", func(t *testing.T) {
		checkTypedErrorSnapshotFailure(t, `"one"`, func(v *errorReject) { *v = "configured-success" }, constraints)
	})
	t.Run("configured_pointer_succeeds", func(t *testing.T) {
		checkTypedErrorSnapshotFailure(t, `"one"`, func(v **errorReject) { value := errorReject("configured-success"); *v = &value }, constraints)
	})
	t.Run("discard_all_partial_struct_fields", func(t *testing.T) {
		type body struct {
			Good errorSecret `json:"good" minLength:"5"`
			Bad  errorReject `json:"bad" minLength:"5"`
		}
		checkTypedErrorSnapshotFailure(t, `{"good":"one","bad":"two"}`, func(v *body) {
			v.Good = "configured-good"
			v.Bad = "configured-bad"
		}, nil)
	})
}

func TestTypedErrorValuesLifecycle(t *testing.T) {
	checkTypedErrorLifecycleCompatibility(t)
	checkTypedErrorDecodeCompatibility(t)
	t.Run("wire_before_normalization", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorNormalize](t, `"one"`, errorMinLength), 0, "normalized:one", reflect.TypeFor[errorNormalize]())
	})
	t.Run("before_default", func(t *testing.T) {
		r := errorValueRequest[struct {
			Secret errorSecret `json:"secret" minLength:"5" default:"default"`
		}](t, `{"secret":""}`, nil)
		errorValueCheck(t, r, 0, "json:", reflect.TypeFor[errorSecret]())
	})
	t.Run("before_resolver", func(t *testing.T) {
		r := errorValueRequest[errorMutating](t, `{"secret":"one","items":["old"],"values":{"a":"old"}}`, nil)
		require.Len(t, r.wire, 3)
		values := map[string]any{}
		for _, e := range r.wire {
			values[e["location"].(string)] = e["value"]
		}
		assert.Equal(t, "pointer:one", values["body.secret"])
		assert.Equal(t, []any{"json:old"}, values["body.items"])
		assert.Equal(t, map[string]any{"a": "json:old"}, values["body.values"])
	})
	for _, raw := range []string{`{"secret":"one","number":"bad"}`, `{"secret":"one","number":999999999999999999999999999999}`} {
		t.Run("decode_failure_"+raw, func(t *testing.T) {
			r := errorValueRequest[struct {
				Secret errorSecret `json:"secret" minLength:"5"`
				Number int8        `json:"number"`
			}](t, raw, nil)
			require.Equal(t, 422, r.code)
			require.NotEmpty(t, r.wire)
			for _, e := range r.wire {
				_, ok := e["value"]
				assert.False(t, ok, "untrusted value: %v", e)
			}
		})
	}
	t.Run("partial_decoder", func(t *testing.T) {
		r := errorValueRequest[errorReject](t, `"one"`, errorMinLength)
		require.Equal(t, 422, r.code)
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
	t.Run("schema_only", func(t *testing.T) {
		r := errorValueRequest[struct{}](t, `{"virtual":"one"}`, func(s *huma.Schema) {
			sub := &huma.Schema{Type: "string"}
			errorMinLength(sub)
			s.Properties = map[string]*huma.Schema{"virtual": sub}
		})
		require.Equal(t, 422, r.code)
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
}

type errorCounted string

func (s *errorCounted) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if *s != "" {
		return errors.New("typed value decoded twice")
	}
	*s = errorCounted(raw)
	return nil
}
func (s errorCounted) MarshalJSON() ([]byte, error) { return json.Marshal("counted:" + string(s)) }

type ErrorFirst struct {
	Value errorSecret `json:"value" minLength:"5"`
}
type ErrorSecond struct {
	Value errorPointer `json:"value" minLength:"5"`
}
type ErrorUntagged struct {
	Value errorSecret `minLength:"5"`
}
type ErrorTagged struct {
	Other errorPointer `json:"Value" minLength:"5"`
}
type errorRecursive struct {
	*errorRecursive             //nolint:unused // Exercises recursive embedding through reflection.
	Value           errorSecret `json:"value" minLength:"5"`
}
type errorInterface struct {
	Value any `json:"value"`
}

func (v *errorInterface) UnmarshalJSON(data []byte) error {
	v.Value = errorSecret("inside")
	return nil
}

type errorTextInterface struct {
	Value any `json:"value"`
}

func (v *errorTextInterface) UnmarshalJSON([]byte) error {
	v.Value = errorText("inside")
	return nil
}

type errorPointerTextInterface struct {
	Value any `json:"value"`
}

func (v *errorPointerTextInterface) UnmarshalJSON([]byte) error {
	v.Value = errorPointerText("inside")
	return nil
}

type errorShortSlice []errorSecret

func (v *errorShortSlice) UnmarshalJSON(data []byte) error { *v = []errorSecret{"kept"}; return nil }

type errorMissingMap map[string]errorSecret

func (v *errorMissingMap) UnmarshalJSON(data []byte) error { *v = map[string]errorSecret{}; return nil }

type errorNilParent struct {
	Child *errorLeaf `json:"child"`
}

func (v *errorNilParent) UnmarshalJSON(data []byte) error { v.Child = nil; return nil }

type errorSnapshot struct {
	Value    any                    `json:"value"`
	Pointer  *errorPointer          `json:"pointer"`
	NilMap   map[string]errorSecret `json:"nilMap"`
	NilSlice []errorSecret          `json:"nilSlice"`
	NilAny   any                    `json:"nilAny"`
	Array    [1]*errorPointer       `json:"array"`
	hidden   string                 //nolint:unused // Reflection must ignore private fields.
}

func (v *errorSnapshot) Resolve(huma.Context) []error {
	if m, ok := v.Value.(map[string]any); ok {
		m["a"] = "changed"
	}
	v.Value = "changed"
	*v.Array[0] = "changed"
	return nil
}

func TestTypedErrorValuesBoundaries(t *testing.T) {
	checkTypedErrorNullCompatibility(t)
	t.Run("direct_dominates", func(t *testing.T) {
		r := errorValueRequest[struct {
			ErrorFirst
			Value errorPointer `json:"value" minLength:"5"`
		}](t, `{"value":"one"}`, nil)
		errorValueCheck(t, r, 0, "pointer:one", reflect.TypeFor[errorPointer]())
	})
	t.Run("tag_dominates", func(t *testing.T) {
		r := errorValueRequest[struct {
			ErrorUntagged
			ErrorTagged
		}](t, `{"Value":"one"}`, nil)
		errorValueCheck(t, r, 0, "pointer:one", reflect.TypeFor[errorPointer]())
	})
	t.Run("ambiguous", func(t *testing.T) {
		r := errorValueRequest[struct {
			ErrorFirst
			ErrorSecond //nolint:govet // Duplicate promoted JSON names are the case under test.
		}](t, `{"value":"one"}`, nil)
		require.Equal(t, 422, r.code)
		require.NotEmpty(t, r.wire)
		for _, e := range r.wire {
			_, ok := e["value"]
			assert.False(t, ok)
		}
	})
	t.Run("recursive_embedding", func(t *testing.T) {
		errorValueCheck(t, errorValueRequest[errorRecursive](t, `{"value":"one"}`, nil), 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("invalid_json_tag", func(t *testing.T) {
		r := errorValueRequest[struct {
			Value errorSecret `json:"\\" minLength:"5"` //nolint:staticcheck // Invalid names must follow encoding/json fallback semantics.
		}](t, `{"Value":"one"}`, func(s *huma.Schema) {
			s.Properties = map[string]*huma.Schema{"Value": {Type: "string"}}
			errorMinLength(s.Properties["Value"])
			s.Required = nil
		})
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("ignored_unexported", func(t *testing.T) {
		r := errorValueRequest[struct {
			Skip   string `json:"-"`
			hidden string
			Value  errorSecret `json:"value" minLength:"5"`
		}](t, `{"value":"one"}`, nil)
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("dynamic_interface", func(t *testing.T) {
		r := errorValueRequest[errorInterface](t, `{"value":"one"}`, func(s *huma.Schema) {
			s.Properties["value"] = &huma.Schema{Type: "string"}
			errorMinLength(s.Properties["value"])
		})
		errorValueCheck(t, r, 0, "json:inside", nil)
	})
	t.Run("dynamic_text_interface", func(t *testing.T) {
		r := errorValueRequest[errorTextInterface](t, `{"value":"one"}`, func(s *huma.Schema) {
			s.Properties["value"] = &huma.Schema{Type: "string"}
			errorMinLength(s.Properties["value"])
		})
		errorValueCheck(t, r, 0, "text:inside", reflect.TypeFor[errorText]())
	})
	t.Run("dynamic_pointer_text_interface", func(t *testing.T) {
		r := errorValueRequest[errorPointerTextInterface](t, `{"value":"one"}`, func(s *huma.Schema) {
			s.Properties["value"] = &huma.Schema{Type: "string"}
			errorMinLength(s.Properties["value"])
		})
		errorValueCheck(t, r, 0, "ptrtext:inside", reflect.TypeFor[errorPointerText]())
	})
	t.Run("short_array_representation", func(t *testing.T) {
		r := errorValueRequest[errorShortSlice](t, `["one","two"]`, func(s *huma.Schema) { errorMinLength(s.Items) })
		require.Len(t, r.wire, 2)
		errorValueCheck(t, r, 0, "json:kept", reflect.TypeFor[errorSecret]())
		_, ok := r.wire[1]["value"]
		assert.False(t, ok)
	})
	t.Run("missing_map_representation", func(t *testing.T) {
		r := errorValueRequest[errorMissingMap](t, `{"a":"one"}`, func(s *huma.Schema) { v := s.AdditionalProperties.(*huma.Schema); errorMinLength(v) })
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
	t.Run("nil_parent_representation", func(t *testing.T) {
		r := errorValueRequest[errorNilParent](t, `{"child":{"secret":"one"}}`, nil)
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
	t.Run("nil_embedded_representation", func(t *testing.T) {
		r := errorValueRequest[errorNilEmbedded](t, `{"renamed":"one"}`, nil)
		require.Equal(t, 422, r.code)
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
	t.Run("container_snapshot", func(t *testing.T) {
		r := errorValueRequest[errorSnapshot](t, `{"value":{"a":"old"},"pointer":null,"nilMap":null,"nilSlice":null,"nilAny":null,"array":["old"]}`, func(s *huma.Schema) { n := 10; s.MinProperties = &n })
		errorValueCheck(t, r, 0, map[string]any{"value": map[string]any{"a": "old"}, "pointer": nil, "nilMap": nil, "nilSlice": nil, "nilAny": nil, "array": []any{"pointer:old"}}, reflect.TypeFor[errorSnapshot]())
	})
}

func checkTypedErrorCompatibility(t *testing.T) {
	func() {
		r := errorValueRequest[*errorPointer](t, `null`, func(s *huma.Schema) { s.Nullable = false; s.Enum = []any{"not-null"} })
		require.Equal(t, 422, r.code)
		require.NotEmpty(t, r.wire)
		for _, e := range r.wire {
			_, ok := e["value"]
			assert.False(t, ok)
		}
	}()
	func() {
		r := errorValueRequest[string](t, `"one"`, errorMinLength)
		errorValueCheck(t, r, 0, "one", reflect.TypeFor[string]())
	}()
	func() {
		r := errorValueRequest[errorSecret](t, `{`, errorMinLength)
		require.Equal(t, 400, r.code)
		require.False(t, r.called)
		require.Len(t, r.wire, 1)
		assert.Equal(t, "{", r.wire[0]["value"])
	}()
	func() {
		r := errorValueRequest[errorSecret](t, `"one"`, errorMinLength, "application/unknown")
		assert.Equal(t, 415, r.code)
		assert.False(t, r.called)
	}()
	func() {
		r := errorValueRequest[errorSecret](t, `"one"`, errorMinLength, "application/json;plus+json")
		assert.Equal(t, 415, r.code)
	}()
	func() {
		r := errorValueRequest[errorSecret](t, `"long-enough"`, errorMinLength)
		assert.True(t, r.called)
		assert.Less(t, r.code, 400)
	}()
	func() {
		schema := &huma.Schema{Type: "string"}
		errorMinLength(schema)
		schema.PrecomputeMessages()
		res := &huma.ValidateResult{}
		pb := huma.NewPathBuffer(nil, 0)
		pb.Push("standalone")
		huma.Validate(huma.NewMapRegistry("#/", huma.DefaultSchemaNamer), schema, pb, huma.ModeWriteToServer, "one", res)
		require.Len(t, res.Errors, 1)
		assert.Equal(t, "one", res.Errors[0].(*huma.ErrorDetail).Value)
	}()
}

func TestTypedErrorValuesRepeated(t *testing.T) {
	config := huma.DefaultConfig("Repeated", "1")
	config.CreateHooks = nil
	decodes := 0
	format := huma.DefaultJSONFormat
	format.Unmarshal = func(data []byte, v any) error {
		if reflect.TypeOf(v) != reflect.TypeFor[*any]() {
			decodes++
		}
		return json.Unmarshal(data, v)
	}
	config.Formats = map[string]huma.Format{"application/json": format, "json": format}
	mux := http.NewServeMux()
	api := humago.New(mux, config)
	huma.Post(api, "/a", func(_ context.Context, _ *struct {
		Body struct {
			Value errorCounted `json:"value" minLength:"5" pattern:"^z"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})
	huma.Post(api, "/b", func(_ context.Context, _ *struct {
		Body struct {
			Value errorPointer `json:"value" minLength:"5"`
		}
	}) (*struct{}, error) {
		return nil, nil
	})
	for i, path := range []string{"/a", "/b", "/a", "/b"} {
		t.Run(fmt.Sprintf("request_%d", i), func(t *testing.T) {
			raw := fmt.Sprintf(`{"value":"v%d"}`, i)
			req := httptest.NewRequest("POST", path, strings.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			before := decodes
			mux.ServeHTTP(w, req)
			assert.Equal(t, before+1, decodes)
			require.Equal(t, 422, w.Code)
			var model struct {
				Errors []map[string]any `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &model))
			prefix := "counted:"
			if path == "/b" {
				prefix = "pointer:"
			}
			for _, e := range model.Errors {
				assert.Equal(t, fmt.Sprintf("%sv%d", prefix, i), e["value"])
			}
			success := httptest.NewRequest("POST", path, strings.NewReader(`{"value":"z-good"}`))
			success.Header.Set("Content-Type", "application/json")
			successResult := httptest.NewRecorder()
			before = decodes
			mux.ServeHTTP(successResult, success)
			assert.Equal(t, before+1, decodes)
			assert.Less(t, successResult.Code, 400)

		})
	}
}

type errorNilEmbedded struct{ *ErrorEmbedded }

func (v *errorNilEmbedded) UnmarshalJSON([]byte) error { v.ErrorEmbedded = nil; return nil }

type errorDynamicPointer struct {
	Value any `json:"value"`
}

func (v *errorDynamicPointer) UnmarshalJSON([]byte) error {
	v.Value = errorPointer("dynamic")
	return nil
}

type errorWrongShape struct {
	Value any `json:"value"`
}

func (v *errorWrongShape) UnmarshalJSON([]byte) error { v.Value = 123; return nil }
func TestTypedErrorValuesAdversarial(t *testing.T) {
	t.Run("dynamic_pointer_method", func(t *testing.T) {
		r := errorValueRequest[errorDynamicPointer](t, `{"value":"one"}`, func(s *huma.Schema) {
			s.Properties["value"] = &huma.Schema{Type: "string"}
			errorMinLength(s.Properties["value"])
		})
		errorValueCheck(t, r, 0, "pointer:dynamic", reflect.TypeFor[errorPointer]())
	})
	t.Run("wrong_shape", func(t *testing.T) {
		r := errorValueRequest[errorWrongShape](t, `{"value":{"leaf":"one"}}`, func(s *huma.Schema) {
			s.Properties["value"] = &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{"leaf": {Type: "string"}}}
			errorMinLength(s.Properties["value"].Properties["leaf"])
		})
		require.Equal(t, 422, r.code)
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
	t.Run("discriminator_container", func(t *testing.T) {
		r := errorValueRequest[errorObject](t, `{"secret":"one"}`, func(s *huma.Schema) {
			s.OneOf = []*huma.Schema{{Type: "object"}}
			s.Discriminator = &huma.Discriminator{PropertyName: "kind", Mapping: map[string]string{}}
		})
		require.GreaterOrEqual(t, len(r.wire), 2)
		for i := range r.wire {
			errorValueCheck(t, r, i, "object:one", reflect.TypeFor[errorObject]())
		}
	})
}

func TestTypedErrorValuesParallel(t *testing.T) {
	config := huma.DefaultConfig("Parallel", "1")
	config.CreateHooks = nil
	mux := http.NewServeMux()
	api := humago.New(mux, config)
	huma.Post(api, "/values", func(_ context.Context, _ *struct{ Body errorLeaf }) (*struct{}, error) { return nil, nil })
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprintf("request_%d", i), func(t *testing.T) {
			t.Parallel()
			raw := fmt.Sprintf(`{"secret":"p%d"}`, i)
			req := httptest.NewRequest("POST", "/values", strings.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			require.Equal(t, 422, w.Code)
			var model struct {
				Errors []map[string]any `json:"errors"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &model))
			require.Len(t, model.Errors, 1)
			assert.Equal(t, fmt.Sprintf("json:p%d", i), model.Errors[0]["value"])
		})
	}
}

func checkTypedErrorLifecycleCompatibility(t *testing.T) {
	func() {
		config := huma.DefaultConfig("Skip", "1")
		mux := http.NewServeMux()
		api := humago.New(mux, config)
		called := false
		huma.Register(api, huma.Operation{Method: "POST", Path: "/", OperationID: "skip", SkipValidateBody: true}, func(_ context.Context, i *struct {
			Body struct {
				Value errorSecret `json:"value" minLength:"5" default:"filled"`
			}
		}) (*struct{}, error) {
			called = true
			assert.Equal(t, errorSecret("filled"), i.Body.Value)
			return nil, nil
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"value":""}`))
		r.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(w, r)
		assert.True(t, called)
		assert.Less(t, w.Code, 400)
	}()
	func() {
		config := huma.DefaultConfig("Nonjson", "1")
		config.CreateHooks = nil
		config.Formats = map[string]huma.Format{"application/json": huma.DefaultJSONFormat, "application/test": huma.DefaultJSONFormat, "json": huma.DefaultJSONFormat}
		mux := http.NewServeMux()
		api := humago.New(mux, config)
		huma.Post(api, "/", func(_ context.Context, _ *struct{ Body errorLeaf }) (*struct{}, error) {
			t.Error("invalid input reached handler")
			return nil, nil
		})
		r := httptest.NewRequest("POST", "/", strings.NewReader(`{"secret":"one"}`))
		r.Header.Set("Content-Type", "application/test")
		r.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		require.Equal(t, 422, w.Code)
		var model struct {
			Errors []map[string]any `json:"errors"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &model))
		require.Len(t, model.Errors, 1)
		assert.Equal(t, "one", model.Errors[0]["value"])
	}()
}

func TestTypedErrorValuesMapCasing(t *testing.T) {
	r := errorValueRequest[map[string]errorSecret](t, `{"SECRET":"one"}`, func(s *huma.Schema) {
		s.Properties = map[string]*huma.Schema{"secret": {Type: "string"}}
		errorMinLength(s.Properties["secret"])
		s.AdditionalProperties = true
	})
	errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
}
func TestTypedErrorValuesMixedModes(t *testing.T) {
	config := huma.DefaultConfig("Mixed modes", "1")
	config.CreateHooks = nil
	config.Formats = map[string]huma.Format{"application/json": huma.DefaultJSONFormat, "json": huma.DefaultJSONFormat, "application/test": huma.DefaultJSONFormat}
	mux := http.NewServeMux()
	api := humago.New(mux, config)
	huma.Post(api, "/", func(_ context.Context, _ *struct {
		Q    string `query:"q" minLength:"5"`
		Body struct {
			Secret errorSecret `json:"secret" minLength:"5"`
			Number int         `json:"number"`
		}
	}) (*struct{}, error) {
		t.Error("invalid input reached handler")
		return nil, nil
	})
	for _, ct := range []string{"application/json", "application/test", "application/json"} {
		for _, number := range []string{"1", `"bad"`} {
			func() {
				r := httptest.NewRequest("POST", "/?q=raw", strings.NewReader(`{"secret":"one","number":`+number+`}`))
				r.Header.Set("Content-Type", ct)
				r.Header.Set("Accept", "application/json")
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, r)
				require.Equal(t, 422, w.Code)
				var model struct {
					Errors []map[string]any `json:"errors"`
				}
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &model))
				require.GreaterOrEqual(t, len(model.Errors), 2)
				seen := false
				for _, e := range model.Errors {
					if e["location"] == "query.q" {
						assert.Equal(t, "raw", e["value"])
					} else {
						seen = true
						if ct == "application/json" {
							if number == "1" {
								assert.Equal(t, "json:one", e["value"])
							} else {
								_, ok := e["value"]
								assert.False(t, ok)
							}
						} else if e["location"] == "body.secret" {
							assert.Equal(t, "one", e["value"])
						}
					}
				}
				assert.True(t, seen)
			}()
		}
	}
}

type errorHiddenField struct{ hidden errorSecret } //nolint:unused // Reflection must ignore private fields.
type errorIgnoredField struct {
	Skip errorSecret `json:"-"`
}

func TestTypedErrorValuesMutationEdges(t *testing.T) {
	t.Run("digit_tag", func(t *testing.T) {
		r := errorValueRequest[struct {
			Value errorSecret `json:"v1" minLength:"5"`
		}](t, `{"v1":"one"}`, nil)
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("hidden_field", func(t *testing.T) {
		r := errorValueRequest[errorHiddenField](t, `{"hidden":"one"}`, func(s *huma.Schema) {
			s.Properties = map[string]*huma.Schema{"hidden": {Type: "string"}}
			errorMinLength(s.Properties["hidden"])
		})
		require.Equal(t, 422, r.code)
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
	t.Run("ignored_field", func(t *testing.T) {
		r := errorValueRequest[errorIgnoredField](t, `{"-":"one"}`, func(s *huma.Schema) {
			s.Properties = map[string]*huma.Schema{"-": {Type: "string"}}
			errorMinLength(s.Properties["-"])
		})
		require.Equal(t, 422, r.code)
		require.Len(t, r.wire, 1)
		_, ok := r.wire[0]["value"]
		assert.False(t, ok)
	})
}

type errorNullNormalizer string

func (v *errorNullNormalizer) UnmarshalJSON([]byte) error  { *v = "synthesized"; return nil }
func (v errorNullNormalizer) MarshalJSON() ([]byte, error) { return json.Marshal(string(v)) }
func checkTypedErrorNullCompatibility(t *testing.T) {
	r := errorValueRequest[errorNullNormalizer](t, `null`, func(s *huma.Schema) { s.Nullable = false })
	require.Equal(t, 422, r.code)
	require.NotEmpty(t, r.wire)
	for _, e := range r.wire {
		_, ok := e["value"]
		assert.False(t, ok)
	}
}

func TestTypedErrorValuesUnresolvedReference(t *testing.T) {
	missing := func(s *huma.Schema) { s.Properties["secret"] = &huma.Schema{Ref: "#/components/schemas/Absent"} }
	for _, key := range []string{"secret", "SECRET"} {
		t.Run(key, func(t *testing.T) {
			r := errorValueRequest[errorLeaf](t, `{"`+key+`":"one"}`, missing)
			errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
			assert.Equal(t, "body.secret", r.details[0].Location)
			assert.Contains(t, r.details[0].Message, "#/components/schemas/Absent")
			registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
			schema := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{"secret": {Ref: "#/components/schemas/Absent"}}}
			schema.PrecomputeMessages()
			validation := &huma.ValidateResult{}
			huma.Validate(registry, schema, &huma.PathBuffer{}, huma.ModeWriteToServer, map[string]any{key: "one"}, validation)
			require.NotEmpty(t, validation.Errors)
			if key == "secret" {
				assert.Equal(t, "one", validation.Errors[0].(*huma.ErrorDetail).Value)
			} else {
				assert.Nil(t, validation.Errors[0].(*huma.ErrorDetail).Value)
			}
			// Null or absent input has no typed offending value, even when
			// reference resolution itself produces an error.
			for _, raw := range []string{`{"` + key + `":null}`, `{}`} {
				n := errorValueRequest[errorLeaf](t, raw, missing)
				require.Equal(t, 422, n.code)
				require.NotEmpty(t, n.wire)
				_, present := n.wire[0]["value"]
				assert.False(t, present)
			}
		})
	}
	t.Run("map_unicode_path", func(t *testing.T) {
		r := errorValueRequest[map[string]errorSecret](t, `{"\u00c9.[KEY]":"one"}`, func(s *huma.Schema) {
			s.Properties = map[string]*huma.Schema{"\u00e9.[key]": {Ref: "#/components/schemas/Absent"}}
			s.AdditionalProperties = true
		})
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
		assert.Equal(t, "body.\u00e9.[key]", r.details[0].Location)
	})
	t.Run("exact_priority", func(t *testing.T) {
		r := errorValueRequest[map[string]errorSecret](t, `{"secret":"one","SECRET":"other"}`, func(s *huma.Schema) {
			s.Properties = map[string]*huma.Schema{"secret": {Ref: "#/components/schemas/Absent"}}
			s.AdditionalProperties = true
		})
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
		assert.Equal(t, "body.secret", r.details[0].Location)
	})
	t.Run("nested_composition", func(t *testing.T) {
		r := errorValueRequest[struct {
			Child map[string]errorSecret `json:"child"`
		}](t, `{"CHILD":{"SECRET":"one"}}`, func(s *huma.Schema) {
			s.Properties["child"] = &huma.Schema{AllOf: []*huma.Schema{{Type: "object", Properties: map[string]*huma.Schema{"secret": {Ref: "#/components/schemas/Absent"}}}}}
		})
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
		assert.Equal(t, "body.child.secret", r.details[0].Location)
	})
	t.Run("strict_exact", func(t *testing.T) {
		old := huma.ValidateStrictCasing
		huma.ValidateStrictCasing = true
		defer func() { huma.ValidateStrictCasing = old }()
		r := errorValueRequest[errorLeaf](t, `{"secret":"one"}`, missing)
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
		n := errorValueRequest[errorLeaf](t, `{"SECRET":"one"}`, missing)
		require.Equal(t, 422, n.code)
		require.NotEmpty(t, n.wire)
		_, present := n.wire[0]["value"]
		assert.False(t, present)
	})
}

type ErrorAmbiguous struct {
	ErrorFirst
	ErrorSecond //nolint:govet // Duplicate promoted JSON names are the case under test.
}
type ErrorDeepLeaf struct {
	Value errorSecret `json:"value" minLength:"5"`
}
type ErrorDeepOne struct{ ErrorDeepLeaf }
type ErrorDeepTwo struct{ ErrorDeepOne }

func TestTypedErrorValuesEmbeddingDepth(t *testing.T) {
	r := errorValueRequest[struct {
		ErrorAmbiguous //nolint:govet // Ambiguous shallow fields must hide the deeper field.
		ErrorDeepTwo
	}](t, `{"value":"one"}`, nil)
	require.Equal(t, 422, r.code)
	require.NotEmpty(t, r.wire)
	for _, e := range r.wire {
		_, ok := e["value"]
		assert.False(t, ok)
	}
}

var errorMarshalFailure = errors.New("custom value serialization failed")

type errorFailMarshal string

func (errorFailMarshal) MarshalJSON() ([]byte, error) { return nil, errorMarshalFailure }
func TestTypedErrorValuesMarshalerFailure(t *testing.T) {
	config := huma.DefaultConfig("Marshaler failure", "1")
	config.CreateHooks = nil
	var observed error
	format := huma.DefaultJSONFormat
	format.Marshal = func(w io.Writer, v any) error {
		_, err := json.Marshal(v)
		if err != nil {
			observed = err
		}
		return err
	}
	config.Formats = map[string]huma.Format{"application/json": format, "json": format}
	mux := http.NewServeMux()
	api := humago.New(mux, config)
	huma.Post(api, "/", func(_ context.Context, _ *struct {
		Body struct {
			Value errorFailMarshal `json:"value" minLength:"5"`
		}
	}) (*struct{}, error) {
		t.Error("invalid body reached handler")
		return nil, nil
	})
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"value":"one"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	require.ErrorIs(t, observed, errorMarshalFailure)
}

func checkTypedErrorDecodeCompatibility(t *testing.T) {
	r := errorValueRequest[errorReject](t, `"long-enough"`, errorMinLength)
	require.Equal(t, 422, r.code)
	require.Len(t, r.wire, 1)
	assert.Equal(t, "typed rejection", r.wire[0]["message"])
	assert.Equal(t, `"long-enough"`, r.wire[0]["value"])
}

type errorOpaqueValue struct {
	data   []byte
	labels map[string]*errorSecret
	held   struct{ value *errorPointer }
}

func (errorOpaqueValue) Schema(huma.Registry) *huma.Schema { return &huma.Schema{Type: "string"} }
func (v *errorOpaqueValue) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	v.data = []byte(raw)
	s := errorSecret(raw)
	v.labels = map[string]*errorSecret{"x": &s}
	p := errorPointer(raw)
	v.held.value = &p
	return nil
}
func (v errorOpaqueValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"data": string(v.data), "label": v.labels["x"], "held": v.held.value})
}
func (v *errorOpaqueValue) Resolve(huma.Context) []error {
	v.data[0] = 'X'
	*v.labels["x"] = "changed"
	*v.held.value = "changed"
	return nil
}
func TestTypedErrorValuesPrivateMarshalerState(t *testing.T) {
	r := errorValueRequest[errorOpaqueValue](t, `"one"`, errorMinLength)
	errorValueCheck(t, r, 0, map[string]any{"data": "one", "label": "json:one", "held": "pointer:one"}, reflect.TypeFor[errorOpaqueValue]())
}

func errorSchemaOnlyRequest[I any](t *testing.T, raw, contentType string, skip bool) (int, []map[string]any, bool) {
	t.Helper()
	config := huma.DefaultConfig("Schema only", "1")
	config.CreateHooks = nil
	config.Formats = map[string]huma.Format{"application/json": huma.DefaultJSONFormat, "json": huma.DefaultJSONFormat, "application/test": huma.DefaultJSONFormat}
	mux := http.NewServeMux()
	api := humago.New(mux, config)
	called := false
	schema := &huma.Schema{Type: huma.TypeObject, Required: []string{"required"}, AdditionalProperties: false, Properties: map[string]*huma.Schema{
		"virtual":  {Type: huma.TypeString, MinLength: Ptr(5)},
		"test":     {Type: huma.TypeInteger, Minimum: Ptr(10.0)},
		"required": {Type: huma.TypeString},
	}}
	schema.PrecomputeMessages()
	huma.Register(api, huma.Operation{Method: "POST", Path: "/", OperationID: "schema-only", SkipValidateBody: skip, RequestBody: &huma.RequestBody{Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}}, func(_ context.Context, _ *I) (*struct{}, error) {
		called = true
		return nil, nil
	})
	req := httptest.NewRequest("POST", "/?q=raw", strings.NewReader(raw))
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var model struct {
		Errors []map[string]any `json:"errors"`
	}
	if w.Code >= 400 {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &model))
	}
	return w.Code, model.Errors, called
}

func TestTypedErrorValuesSchemaOnly(t *testing.T) {
	checkTypedErrorSchemaCompatibility(t)
	for _, shape := range []string{"raw", "empty", "query"} {
		t.Run(shape, func(t *testing.T) {
			for _, ct := range []string{"application/json", "application/json; charset=utf-8"} {
				t.Run(ct, func(t *testing.T) {
					raw := `{"virtual":"one","test":1,"extra":true}`
					var code int
					var details []map[string]any
					var called bool
					switch shape {
					case "raw":
						code, details, called = errorSchemaOnlyRequest[struct{ RawBody []byte }](t, raw, ct, false)
					case "empty":
						code, details, called = errorSchemaOnlyRequest[struct{}](t, raw, ct, false)
					case "query":
						code, details, called = errorSchemaOnlyRequest[struct {
							Q string `query:"q" minLength:"5"`
						}](t, raw, ct, false)
					}
					require.Equal(t, 422, code)
					require.False(t, called)
					locations := map[string]bool{}
					for _, detail := range details {
						location := detail["location"].(string)
						locations[location] = true
						if location == "query.q" {
							assert.Equal(t, "raw", detail["value"])
							continue
						}
						assert.True(t, location == "body" || strings.HasPrefix(location, "body."))
						assert.NotEmpty(t, detail["message"])
						assert.NotContains(t, detail, "value", "untyped body value at %s", location)
					}
					for _, name := range []string{"virtual", "test", "extra"} {
						assert.True(t, locations["body."+name], name)
					}
					assert.True(t, locations["body"], "required-property container error")
					if shape == "query" {
						assert.True(t, locations["query.q"])
					}
				})
			}
		})
	}
}

func TestTypedErrorValuesSemicolonTag(t *testing.T) {
	type body struct {
		Value errorSecret `json:";" minLength:"5"`
	}
	var decoded body
	require.NoError(t, json.Unmarshal([]byte(`{";":"one"}`), &decoded))
	require.Equal(t, errorSecret("one"), decoded.Value)
	r := errorValueRequest[body](t, `{";":"one"}`, nil)
	errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
	require.Len(t, r.wire, 1)
	assert.Equal(t, "body.;", r.details[0].Location)
}

func checkTypedErrorSchemaCompatibility(t *testing.T) {
	func() {
		code, details, called := errorSchemaOnlyRequest[struct{ RawBody []byte }](t, `{"virtual":"one","test":1,"required":"ok"}`, "application/test", false)
		require.Equal(t, 422, code)
		require.False(t, called)
		require.Len(t, details, 2)
		for _, detail := range details {
			if detail["location"] == "body.virtual" {
				assert.Equal(t, "one", detail["value"])
			} else {
				assert.InDelta(t, float64(1), detail["value"], 0)
			}
		}
	}()
	func() {
		code, details, called := errorSchemaOnlyRequest[struct{}](t, `{`, "application/json", false)
		require.Equal(t, 400, code)
		require.False(t, called)
		require.Len(t, details, 1)
		assert.Equal(t, "{", details[0]["value"])
	}()
	for _, skip := range []bool{false, true} {
		func() {
			raw := `{"virtual":"valid","test":10,"required":"ok"}`
			if skip {
				raw = `{"virtual":"one"}`
			}
			code, details, called := errorSchemaOnlyRequest[struct{ RawBody []byte }](t, raw, "application/json", skip)
			assert.Equal(t, 204, code)
			assert.True(t, called)
			assert.Empty(t, details)
		}()
	}
}

func TestTypedErrorValuesMapSliceContainer(t *testing.T) {
	r := errorValueRequest[map[string]errorRootSlice](t, `{"x":["one"]}`, func(s *huma.Schema) {
		s.AdditionalProperties = &huma.Schema{Type: "array", Items: &huma.Schema{Type: "string"}, MinItems: func() *int { n := 2; return &n }()}
	})
	errorValueCheck(t, r, 0, "slice:1", reflect.TypeFor[errorRootSlice]())
}

type errorDiscardedContainer[T any] struct {
	Value T `json:"value"`
}

func (v *errorDiscardedContainer[T]) UnmarshalJSON([]byte) error { return nil }

type errorNilDynamicContainer[T any] struct {
	Value any `json:"value"`
}

func (v *errorNilDynamicContainer[T]) UnmarshalJSON([]byte) error {
	var zero T
	v.Value = zero
	return nil
}

type errorNilMap map[string]errorSecret
type errorNilSlice []errorSecret

func (v *errorNilMap) UnmarshalJSON([]byte) error   { *v = nil; return nil }
func (v *errorNilSlice) UnmarshalJSON([]byte) error { *v = nil; return nil }

func checkTypedErrorAbsentContainer[B any](t *testing.T, raw string, schema *huma.Schema) {
	t.Helper()
	r := errorValueRequest[B](t, raw, func(s *huma.Schema) { *s = *schema })
	require.Equal(t, http.StatusUnprocessableEntity, r.code)
	require.False(t, r.called)
	require.Len(t, r.wire, 1)
	require.Len(t, r.details, 1)
	_, present := r.wire[0]["value"]
	assert.False(t, present, "an absent typed container must omit value, not emit null")
	assert.Nil(t, r.details[0].Value)
	assert.Equal(t, r.details[0].Location, r.wire[0]["location"])
}

func TestTypedErrorValuesAbsentContainers(t *testing.T) {
	two := 2
	object := &huma.Schema{Type: "object", MinProperties: &two, AdditionalProperties: true}
	array := &huma.Schema{Type: "array", MinItems: &two, Items: &huma.Schema{Type: "string"}}
	wrap := func(child *huma.Schema) *huma.Schema {
		return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{"value": child}, AdditionalProperties: true}
	}
	for _, folded := range []bool{false, true} {
		suffix := "}"
		if folded {
			suffix = `,"VALUE":null}`
		}
		for _, tc := range []struct {
			name string
			run  func(*testing.T)
		}{
			{"pointer", func(t *testing.T) {
				checkTypedErrorAbsentContainer[errorDiscardedContainer[*errorLeaf]](t, `{"value":{"secret":"one"}`+suffix, wrap(object))
			}},
			{"map", func(t *testing.T) {
				checkTypedErrorAbsentContainer[errorDiscardedContainer[map[string]errorSecret]](t, `{"value":{"secret":"one"}`+suffix, wrap(object))
			}},
			{"slice", func(t *testing.T) {
				checkTypedErrorAbsentContainer[errorDiscardedContainer[[]errorSecret]](t, `{"value":["one"]`+suffix, wrap(array))
			}},
			{"interface_pointer", func(t *testing.T) {
				checkTypedErrorAbsentContainer[errorNilDynamicContainer[*errorLeaf]](t, `{"value":{"secret":"one"}`+suffix, wrap(object))
			}},
			{"interface_map", func(t *testing.T) {
				checkTypedErrorAbsentContainer[errorNilDynamicContainer[map[string]errorSecret]](t, `{"value":{"secret":"one"}`+suffix, wrap(object))
			}},
			{"interface_slice", func(t *testing.T) {
				checkTypedErrorAbsentContainer[errorNilDynamicContainer[[]errorSecret]](t, `{"value":["one"]`+suffix, wrap(array))
			}},
			{"absent_parent", func(t *testing.T) {
				child := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{"secret": {Type: "string", MinLength: &two}}, AdditionalProperties: true}
				checkTypedErrorAbsentContainer[errorDiscardedContainer[*errorLeaf]](t, `{"value":{"secret":"x","SECRET":"long"}`+suffix, wrap(child))
			}},
			{"absent_map_entry", func(t *testing.T) {
				child := &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{"secret": object}, AdditionalProperties: true}
				checkTypedErrorAbsentContainer[errorDiscardedContainer[map[string]*errorLeaf]](t, `{"value":{"secret":{"secret":"one"},"SECRET":null}`+suffix, wrap(child))
			}},
		} {
			t.Run(fmt.Sprintf("%s/folded_%t", tc.name, folded), tc.run)
		}
	}
	t.Run("root_map", func(t *testing.T) {
		checkTypedErrorAbsentContainer[errorNilMap](t, `{"secret":"one"}`, object)
	})
	t.Run("root_slice", func(t *testing.T) {
		checkTypedErrorAbsentContainer[errorNilSlice](t, `["one"]`, array)
	})
	t.Run("pointer_to_nil_map", func(t *testing.T) {
		checkTypedErrorAbsentContainer[*errorNilMap](t, `{"secret":"one"}`, object)
	})
	t.Run("map_element", func(t *testing.T) {
		checkTypedErrorAbsentContainer[map[string]errorNilMap](t, `{"value":{"secret":"one"}}`, wrap(object))
	})
	t.Run("slice_element", func(t *testing.T) {
		checkTypedErrorAbsentContainer[[]errorNilSlice](t, `[["one"]]`, &huma.Schema{Type: "array", Items: array})
	})
	t.Run("folded_map_decoder", func(t *testing.T) {
		checkTypedErrorAbsentContainer[struct {
			Value errorNilMap `json:"value"`
		}](t, `{"value":{"secret":"one"},"VALUE":null}`, wrap(object))
	})
	t.Run("folded_slice_decoder", func(t *testing.T) {
		checkTypedErrorAbsentContainer[struct {
			Value errorNilSlice `json:"value"`
		}](t, `{"value":["one"],"VALUE":null}`, wrap(array))
	})
}

func TestTypedErrorValuesRepresentedCollisions(t *testing.T) {
	t.Run("parent_overwritten_by_null", func(t *testing.T) {
		r := errorValueRequest[struct {
			Child *errorLeaf `json:"child"`
		}](t, `{"child":{"secret":"one"},"CHILD":null}`, nil)
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("slice_overwritten_by_empty", func(t *testing.T) {
		r := errorValueRequest[struct {
			Children []errorLeaf `json:"children"`
		}](t, `{"children":[{"secret":"one"}],"CHILDREN":[]}`, nil)
		errorValueCheck(t, r, 0, "json:one", reflect.TypeFor[errorSecret]())
	})
	t.Run("decoded_empty_map", func(t *testing.T) {
		r := errorValueRequest[errorMissingMap](t, `{"key":"long-enough"}`, func(s *huma.Schema) { n := 2; s.MinProperties = &n })
		errorValueCheck(t, r, 0, map[string]any{}, reflect.TypeFor[errorMissingMap]())
	})
}
