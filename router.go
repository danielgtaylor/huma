// Package huma is a modern, simple, fast & opinionated REST API framework.
// Based on OpenAPI 3 and JSON Schema so it can be used to automatically
// generate an OpenAPI spec, interactive documentation, client SDKs in many
// languages, and a CLI for scripting. Pronounced IPA: /'hjuːmɑ/.
//
// Start by creating a `Router` and attaching resources and operations to
// it:
//
//   // Create a new router
//   r := huma.NewRouter("Ping API", "1.0.0")
//
//   // Add a simple ping/pong
//   r.Resource("/ping").Get("Ping", func() string {
//   	return "pong"
//   })
//
//   // Run it!
//   r.Run()
//
// Now you can access the API, generated documentation, and API description:
//
//   # Access the API
//   $ curl http://localhost:8888/hello
//
//   # Read the generated documentation
//   $ open http://localhost:8888/docs
//
//   # See the OpenAPI 3 spec
//   $ curl http://localhost:8888/openapi.json
//
// You can add tests with the help of the `humatest` module:
//
//   func TestPint(t *testing.T) {
//   	r := humatest.NewRouter(t)
//
//   	// Add your service routes to the test router.
//   	registerRoutes(r)
//
//   	// Make a test request.
//   	w := httptest.NewRecorder()
//   	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
//   	r.ServeHTTP(w, req)
//   	assert.Equal(t, http.StatusOK, w.Code)
//   	assert.Equal(t, "pong", w.Body.String())
//   }
//
// See https://github.com/danielgtaylor/huma#readme for more high-level feature
// docs with examples.
package huma

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/schema"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"github.com/xeipuuv/gojsonschema"
	"go.uber.org/zap"
)

// ErrInvalidParamLocation is returned when the `in` field of the parameter
// is not a valid value.
var ErrInvalidParamLocation = errors.New("invalid parameter location")

// connContextKey is used to get/set the underlying `net.Conn` from a request
// context value.
var connContextKey = struct{}{}

var timeType = reflect.TypeOf(time.Time{})

type unsafeHandler struct {
	handler func(inputs ...interface{}) []interface{}
}

// UnsafeHandler is used to register programmatic handlers without argument
// count and type checking. This is useful for libraries that want to
// programmatically create new resources/operations. Using UnsafeHandler outside
// of that use-case is discouraged.
//
// The function's inputs are the ordered resolved dependencies, parsed
// parameters, and potentially an input body for PUT/POST requests that have
// a request schema defined. The output is a slice of response headers and
// response models.
//
// When using UnsafeHandler, you must manually define schemas for request
// and response bodies. They will be unmarshalled as `interface{}` when
// passed to the handler.
func UnsafeHandler(handler func(inputs ...interface{}) []interface{}) interface{} {
	return &unsafeHandler{handler}
}

// getConn gets the underlying `net.Conn` from a request.
func getConn(r *http.Request) net.Conn {
	conn := r.Context().Value(connContextKey)
	if conn != nil {
		return conn.(net.Conn)
	}
	return nil
}

// abortWithError is a convenience function for setting an error on a Gin
// context with a detail string and optional error strings.
func abortWithError(c *gin.Context, status int, detail string, errors ...string) {
	c.Header("content-type", "application/problem+json")
	c.AbortWithStatusJSON(status, &ErrorModel{
		Status: status,
		Title:  http.StatusText(status),
		Detail: detail,
		Errors: errors,
	})
}

// Checks if data validates against the given schema. Returns false on failure.
func validAgainstSchema(c *gin.Context, label string, schema *schema.Schema, data []byte) bool {
	defer func() {
		// Catch panics from the `gojsonschema` library.
		if err := recover(); err != nil {
			abortWithError(c, http.StatusBadRequest, "Invalid input: "+label, err.(error).Error()+": "+string(data))
		}
	}()

	loader := gojsonschema.NewGoLoader(schema)
	doc := gojsonschema.NewBytesLoader(data)
	s, err := gojsonschema.NewSchema(loader)
	if err != nil {
		panic(err)
	}
	result, err := s.Validate(doc)
	if err != nil {
		panic(err)
	}

	if !result.Valid() {
		errors := []string{}
		for _, desc := range result.Errors() {
			errors = append(errors, fmt.Sprintf("%s", desc))
		}
		abortWithError(c, http.StatusBadRequest, "Invalid input: "+label, errors...)
		return false
	}

	return true
}

func parseParamValue(c *gin.Context, name string, typ reflect.Type, timeFormat string, pstr string) (interface{}, bool) {
	var pv interface{}
	switch typ.Kind() {
	case reflect.Bool:
		converted, err := strconv.ParseBool(pstr)
		if err != nil {
			abortWithError(c, http.StatusBadRequest, fmt.Sprintf("cannot parse boolean for param %s: %s", name, pstr))
			return nil, false
		}
		pv = converted
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		converted, err := strconv.Atoi(pstr)
		if err != nil {
			abortWithError(c, http.StatusBadRequest, fmt.Sprintf("cannot parse integer for param %s: %s", name, pstr))
			return nil, false
		}
		pv = reflect.ValueOf(converted).Convert(typ).Interface()
	case reflect.Float32:
		converted, err := strconv.ParseFloat(pstr, 32)
		if err != nil {
			abortWithError(c, http.StatusBadRequest, fmt.Sprintf("cannot parse float for param %s: %s", name, pstr))
			return nil, false
		}
		pv = float32(converted)
	case reflect.Float64:
		converted, err := strconv.ParseFloat(pstr, 64)
		if err != nil {
			abortWithError(c, http.StatusBadRequest, fmt.Sprintf("cannot parse float for param %s: %s", name, pstr))
			return nil, false
		}
		pv = converted
	case reflect.Slice:
		if len(pstr) > 1 && pstr[0] == '[' {
			pstr = pstr[1 : len(pstr)-1]
		}
		slice := reflect.MakeSlice(typ, 0, 0)
		for _, item := range strings.Split(pstr, ",") {
			if itemValue, ok := parseParamValue(c, name, typ.Elem(), timeFormat, item); ok {
				slice = reflect.Append(slice, reflect.ValueOf(itemValue))
			} else {
				// Error is already handled, just return.
				return nil, false
			}
		}
		pv = slice.Interface()
	default:
		if typ == timeType {
			dt, err := time.Parse(timeFormat, pstr)
			if err != nil {
				abortWithError(c, http.StatusBadRequest, fmt.Sprintf("cannot parse time for param %s: %s", name, pstr))
				return nil, false
			}
			pv = dt
		} else {
			pv = pstr
		}
	}

	return pv, true
}

func getParamValue(c *gin.Context, param *openAPIParam) (interface{}, bool) {
	var pstr string
	timeFormat := time.RFC3339Nano

	switch param.In {
	case inPath:
		pstr = c.Param(param.Name)
	case inQuery:
		pstr = c.Query(param.Name)
		if pstr == "" {
			return param.def, true
		}
	case inHeader:
		pstr = c.GetHeader(param.Name)
		if pstr == "" {
			return param.def, true
		}

		// Some headers have special time formats that aren't ISO8601/RFC3339.
		lowerName := strings.ToLower(param.Name)
		if lowerName == "if-modified-since" || lowerName == "if-unmodified-since" {
			timeFormat = http.TimeFormat
		}
	default:
		panic(fmt.Errorf("%s: %w", param.In, ErrInvalidParamLocation))
	}

	if param.Schema.HasValidation() {
		data := pstr
		if param.Schema.Type == "string" {
			// Strings are special in that we don't expect users to provide them
			// with quotes, so wrap them here for the parser that does the
			// validation step below.
			data = `"` + data + `"`
		} else if param.Schema.Type == "array" {
			// Array type needs to have `[` and `]` added.
			if param.Schema.Items.Type == "string" {
				// Same as above, quote each item.
				data = `"` + strings.Join(strings.Split(data, ","), `","`) + `"`
			}
			if len(data) > 0 && data[0] != '[' {
				data = "[" + data + "]"
			}
		}
		if !validAgainstSchema(c, param.Name, param.Schema, []byte(data)) {
			return nil, false
		}
	}

	pv, ok := parseParamValue(c, param.Name, param.typ, timeFormat, pstr)
	if !ok {
		return nil, false
	}

	return pv, true
}

func getRequestBody(c *gin.Context, t reflect.Type, op *openAPIOperation) (interface{}, bool) {
	var val interface{}

	if t != nil {
		// If we have a type, then use it. Otherwise the body will unmarshal into
		// a generic `map[string]interface{}` or `[]interface{}`.
		val = reflect.New(t).Interface()
	}

	if op.requestSchema != nil {
		body, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			if strings.Contains(err.Error(), "request body too large") {
				abortWithError(c, http.StatusRequestEntityTooLarge, fmt.Sprintf("Request body too large, limit = %d bytes", op.maxBodyBytes))
			} else if e, ok := err.(net.Error); ok && e.Timeout() {
				abortWithError(c, http.StatusRequestTimeout, fmt.Sprintf("Request body took too long to read: timed out after %v", op.bodyReadTimeout))
			} else {
				panic(err)
			}
			return nil, false
		}

		c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		if !validAgainstSchema(c, "request body", op.requestSchema, body) {
			// Error already handled, just return.
			return nil, false
		}
	}

	if err := c.ShouldBindJSON(val); err != nil {
		panic(err)
	}

	return val, true
}

// Router handles API requests.
type Router struct {
	api         *openAPI
	engine      *gin.Engine
	root        *cobra.Command
	prestart    []func()
	docsHandler Handler
	corsHandler Handler

	// Tracks the currently running server for graceful shutdown.
	server     *http.Server
	serverLock sync.Mutex
}

// NewRouter creates a new Huma router for handling API requests with
// default middleware and routes attached. The `docs` and `version` arguments
// will be used to set the title/description and version of the OpenAPI spec.
// If `docs` is multiline, the first line is used for the title and all other
// lines are used for the description. Pass options to customize the created
// router and OpenAPI.
func NewRouter(docs, version string, options ...RouterOption) *Router {
	// Setup default Gin instance with our middleware.
	g := gin.New()
	g.Use(Recovery())
	g.Use(LogMiddleware())
	g.Use(PreferMinimalMiddleware())
	g.Use(ServiceLinkMiddleware())
	g.NoRoute(Handler404())

	title, desc := splitDocs(docs)

	// Create the default router.
	r := &Router{
		api: &openAPI{
			Title:           title,
			Description:     desc,
			Version:         version,
			Servers:         make([]*openAPIServer, 0),
			SecuritySchemes: make(map[string]*openAPISecurityScheme, 0),
			Security:        make([]openAPISecurityRequirement, 0),
			Paths:           make(map[string]map[string]*openAPIOperation),
			Extra:           make(map[string]interface{}),
		},
		engine:      g,
		prestart:    []func(){},
		docsHandler: RapiDocHandler(title),
		corsHandler: cors.Default(),
	}

	r.setupCLI()

	// Apply any passed options.
	for _, option := range options {
		option.ApplyRouter(r)
	}

	// Apply CORS handler *after* options in case a custom Gin Engine is passed.
	r.GinEngine().Use(func(c *gin.Context) {
		r.corsHandler(c)
	})

	// Validate the router/API setup.
	if err := r.api.validate(); err != nil {
		panic(err)
	}

	// Set up handlers for the auto-generated spec and docs.
	r.engine.GET("/openapi.json", openAPIHandlerJSON(r))
	r.engine.GET("/openapi.yaml", openAPIHandlerYAML(r))

	r.engine.GET("/docs", func(c *gin.Context) {
		r.docsHandler(c)
	})

	// If downloads like a CLI or SDKs are available, serve them automatically
	// so you can reference them from e.g. the docs.
	if _, err := os.Stat("downloads"); err == nil {
		r.engine.Static("/downloads", "downloads")
	}

	return r
}

// GinEngine returns the underlying low-level Gin engine.
func (r *Router) GinEngine() *gin.Engine {
	return r.engine
}

// ServeHTTP conforms to the `http.Handler` interface.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.engine.ServeHTTP(w, req)
}

// Resource creates a new resource at the given path with the given
// dependencies, parameters, response headers, and responses defined.
func (r *Router) Resource(path string, options ...ResourceOption) *Resource {
	return NewResource(r, path).With(options...)
}

// register a new operation.
func (r *Router) register(method, path string, op *openAPIOperation) {
	// First, make sure the operation and handler make sense, as well as pre-
	// generating any schemas for use later during request handling.
	op.validate(method, path)

	// Add the operation to the list of operations for the path entry.
	if r.api.Paths[path] == nil {
		r.api.Paths[path] = make(map[string]*openAPIOperation)
	}

	r.api.Paths[path][method] = op

	// Next, figure out which Gin function to call.
	var f func(string, ...gin.HandlerFunc) gin.IRoutes

	switch method {
	case "OPTIONS":
		f = r.engine.OPTIONS
	case "HEAD":
		f = r.engine.HEAD
	case "GET":
		f = r.engine.GET
	case "POST":
		f = r.engine.POST
	case "PUT":
		f = r.engine.PUT
	case "PATCH":
		f = r.engine.PATCH
	case "DELETE":
		f = r.engine.DELETE
	default:
		panic("unsupported HTTP method " + method)
	}

	if strings.Contains(path, "{") {
		// Convert from OpenAPI-style parameters to gin-style params
		path = paramRe.ReplaceAllString(path, ":$1$2")
	}

	// Then call it to register our handler function.
	f(path, func(c *gin.Context) {
		var method reflect.Value
		if op.unsafe() {
			method = reflect.ValueOf(op.handler.(*unsafeHandler).handler)
		} else {
			method = reflect.ValueOf(op.handler)
		}

		in := make([]reflect.Value, 0, len(op.dependencies)+len(op.params)+1)

		// Limit the body size
		if c.Request.Body != nil {
			maxBody := op.maxBodyBytes
			if maxBody == 0 {
				// 1 MiB default
				maxBody = 1024 * 1024
			}

			// -1 is a special value which means set no limit.
			if maxBody != -1 {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
			}
		}

		// Process any dependencies first.
		for _, dep := range op.dependencies {
			headers, value, err := dep.resolve(c, op)
			if err != nil {
				if !c.IsAborted() {
					// Nothing else has handled the error, so treat it like a general
					// internal server error.
					abortWithError(c, http.StatusInternalServerError, "Couldn't get dependency")
				}
			}

			for k, v := range headers {
				c.Header(k, v)
			}

			in = append(in, reflect.ValueOf(value))
		}

		for _, param := range op.params {
			pv, ok := getParamValue(c, param)
			if !ok {
				// Error has already been handled.
				return
			}

			in = append(in, reflect.ValueOf(pv))
		}

		readTimeout := op.bodyReadTimeout
		if op.requestSchema != nil {
			if readTimeout == 0 {
				// Default to 15s when reading/parsing/validating automatically.
				readTimeout = 15 * time.Second
			}

			if conn := getConn(c.Request); readTimeout > 0 && conn != nil {
				conn.SetReadDeadline(time.Now().Add(readTimeout))
			}

			// Parse body
			i := len(in)

			var bodyType reflect.Type
			if op.unsafe() {
				bodyType = reflect.TypeOf(map[string]interface{}{})
			} else {
				bodyType = method.Type().In(i)
			}

			b, success := getRequestBody(c, bodyType, op)
			if !success {
				// Error was already handled in `getRequestBody`.
				return
			}
			bval := reflect.ValueOf(b)
			if bval.Kind() == reflect.Ptr {
				bval = bval.Elem()
			}
			in = append(in, bval)
		} else if readTimeout > 0 {
			// We aren't processing the input, but still set the timeout.
			if conn := getConn(c.Request); conn != nil {
				conn.SetReadDeadline(time.Now().Add(readTimeout))
			}
		}

		out := method.Call(in)

		if op.unsafe() {
			// Normal handlers return multiple values. Unsafe handlers return one
			// single list of response values. Here we convert.
			newOut := make([]reflect.Value, out[0].Len())

			for i := 0; i < out[0].Len(); i++ {
				newOut[i] = out[0].Index(i)
			}

			out = newOut
		}

		// Find and return the first non-zero response. The status code comes
		// from the registered `huma.Response` struct.
		// This breaks down with scalar types... so they need to be passed
		// as a pointer and we'll dereference it automatically.
		for i, o := range out[len(op.responseHeaders):] {
			if o.Kind() == reflect.Interface {
				// Unsafe handlers return slices of interfaces and IsZero will never
				// evaluate to true on items within them. Instead, pull out the
				// underlying data which may or may not be zero.
				o = o.Elem()
			}

			if !o.IsZero() {
				body := o.Interface()

				r := op.responses[i]

				// Set response headers
				for j, header := range op.responseHeaders {
					value := out[j]

					found := false
					for _, name := range r.Headers {
						if name == header.Name {
							found = true
							break
						}
					}

					if !found {
						if !value.IsZero() {
							// Log an error to be fixed later if using the logging middleware.
							if l, ok := c.Get("log"); ok {
								if log, ok := l.(*zap.SugaredLogger); ok {
									log.Errorf("Header '%s' with value '%v' set on a response that did not declare it", header.Name, value)
								}
							}
						}
						// Skip this header as the response doesn't list it.
						continue
					}

					if !value.IsZero() {
						v := value.Interface()
						if value.Kind() == reflect.Ptr {
							v = value.Elem().Interface()
						}
						c.Header(header.Name, fmt.Sprintf("%v", v))
					}
				}

				if r.ContentType == "" {
					// No body allowed, e.g. for HTTP 204.
					c.Status(r.StatusCode)
					break
				}

				if err, ok := body.(*ErrorModel); ok {
					// This is an error response. Automatically set some values if missing.
					if err.Status == 0 {
						err.Status = r.StatusCode
					}

					if err.Title == "" {
						err.Title = http.StatusText(r.StatusCode)
					}
				}

				if strings.HasPrefix(r.ContentType, "application/json") || strings.HasSuffix(r.ContentType, "+json") {
					c.JSON(r.StatusCode, body)
				} else if strings.HasPrefix(r.ContentType, "application/yaml") || strings.HasSuffix(r.ContentType, "+yaml") {
					c.YAML(r.StatusCode, body)
				} else {
					if o.Kind() == reflect.Ptr {
						// This is a pointer to something, so we derefernce it and get
						// its value before converting to a string because Printf will
						// by default print pointer addresses instead of their value.
						body = o.Elem().Interface()
					}
					c.Data(r.StatusCode, r.ContentType, []byte(fmt.Sprintf("%v", body)))
				}
				break
			}
		}
	})
}

func (r *Router) listen(addr, certFile, keyFile string) error {
	r.serverLock.Lock()
	if r.server == nil {
		r.server = &http.Server{
			Addr:              addr,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       15 * time.Second,
			Handler:           r,
			ConnContext: func(ctx context.Context, c net.Conn) context.Context {
				return context.WithValue(ctx, connContextKey, c)
			},
		}
	} else {
		r.server.Addr = addr

		// Wrap the ConnContext method to inject the current connection into the
		// request context. This is useful to e.g. set deadlines.
		orig := r.server.ConnContext
		r.server.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
			if orig != nil {
				ctx = orig(ctx, c)
			}
			return context.WithValue(ctx, connContextKey, c)
		}
	}
	r.serverLock.Unlock()

	if certFile != "" {
		return r.server.ListenAndServeTLS(certFile, keyFile)
	}

	return r.server.ListenAndServe()
}

// Listen for new connections.
func (r *Router) Listen(addr string) error {
	return r.listen(addr, "", "")
}

// ListenTLS listens for new connections using HTTPS & HTTP2
func (r *Router) ListenTLS(addr, certFile, keyFile string) error {
	return r.listen(addr, certFile, keyFile)
}

// Shutdown gracefully shuts down the server.
func (r *Router) Shutdown(ctx context.Context) error {
	r.serverLock.Lock()
	defer r.serverLock.Unlock()

	if r.server == nil {
		panic("no server started")
	}
	return r.server.Shutdown(ctx)
}

// Run executes the router command.
func (r *Router) Run() {
	if err := r.root.Execute(); err != nil {
		panic(err)
	}
}
