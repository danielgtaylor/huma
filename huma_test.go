package huma_test

import (
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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
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

// BodyContainer is an embed request body struct to test request body unmarshalling
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

func (m *MyTextUnmarshaler) UnmarshalText(text []byte) error {
	m.value = "Hello, World!"
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
			Name: "param-unsupported-500",
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
				assert.Equal(t, http.StatusInternalServerError, resp.Code)
			},
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
						S1 StructWithDefaultField `json:"s1,omitempty"`
						S2 StructWithDefaultField `json:"s2,omitempty"`
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
						HelloWorld huma.FormFile `form:"file" contentType:"text/plain"`
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
		}, {
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
				assert.JSONEq(t, `{"$schema": "https:///schemas/RespBody.json", "greeting":"Hello, world!"}`, resp.Body.String())
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
			config := huma.DefaultConfig("Features Test API", "1.0.0")
			if feature.Transformers != nil {
				config.Transformers = append(config.Transformers, feature.Transformers...)
			}
			api := humatest.Wrap(t, humago.New(r, config))
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

	example := app.OpenAPI().Paths["/test"].Get.Parameters[0].Example
	assert.Equal(t, 1, example)
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

	for i := 0; i < 100; i++ {
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

func TestGenerateFuncsPanicWithDescriptiveMessage(t *testing.T) {
	var resp *int
	assert.PanicsWithValue(t, "Response type must be a struct", func() {
		huma.GenerateOperationID("GET", "/foo", resp)
	})

	assert.PanicsWithValue(t, "Response type must be a struct", func() {
		huma.GenerateSummary("GET", "/foo", resp)
	})

}
