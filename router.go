package huma

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xeipuuv/gojsonschema"
)

// ErrInvalidParamLocation is returned when the `in` field of the parameter
// is not a valid value.
var ErrInvalidParamLocation = errors.New("invalid parameter location")

// ErrDependencyInvalid is returned when registering a dependency fails.
var ErrDependencyInvalid = errors.New("dependency invalid")

func getParamValue(c *gin.Context, param *Param) (interface{}, error) {
	var pstr string
	switch param.In {
	case "path":
		pstr = c.Param(param.Name)
	case "query":
		pstr = c.Query(param.Name)
		if pstr == "" {
			return param.def, nil
		}
	case "header":
		pstr = c.GetHeader(param.Name)
		if pstr == "" {
			return param.def, nil
		}
	default:
		return nil, fmt.Errorf("%s: %w", param.In, ErrInvalidParamLocation)
	}

	if pstr == "" && !param.Required {
		// Optional and not passed, so set it to its zero value.
		return reflect.New(param.typ).Elem().Interface(), nil
	}

	var pv interface{}
	switch param.typ.Kind() {
	case reflect.Bool:
		converted, err := strconv.ParseBool(pstr)
		if err != nil {
			return nil, err
		}
		pv = converted
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		converted, err := strconv.Atoi(pstr)
		if err != nil {
			return nil, err
		}
		pv = converted
	case reflect.Float32:
		converted, err := strconv.ParseFloat(pstr, 32)
		if err != nil {
			return nil, err
		}
		pv = converted
	case reflect.Float64:
		converted, err := strconv.ParseFloat(pstr, 64)
		if err != nil {
			return nil, err
		}
		pv = converted
	default:
		pv = pstr
	}

	return pv, nil
}

func getRequestBody(c *gin.Context, t reflect.Type, op *Operation) (interface{}, bool) {
	val := reflect.New(t).Interface()

	if op.RequestSchema != nil {
		body, err := ioutil.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithError(500, err)
			return nil, false
		}

		c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		loader := gojsonschema.NewGoLoader(op.RequestSchema)
		doc := gojsonschema.NewBytesLoader(body)
		s, err := gojsonschema.NewSchema(loader)
		if err != nil {
			c.AbortWithError(500, err)
			return nil, false
		}
		result, err := s.Validate(doc)
		if err != nil {
			c.AbortWithError(500, err)
			return nil, false
		}

		if !result.Valid() {
			errors := []string{}
			for _, desc := range result.Errors() {
				errors = append(errors, fmt.Sprintf("%s", desc))
			}
			c.AbortWithStatusJSON(400, &ErrorInvalidModel{
				Message: "Invalid input",
				Errors:  errors,
			})
			return nil, false
		}
	}

	if err := c.ShouldBindJSON(val); err != nil {
		c.AbortWithError(500, err)
		return nil, false
	}

	return val, true
}

// Router handles API requests.
type Router struct {
	api    *OpenAPI
	engine *gin.Engine
	deps   map[reflect.Type]interface{}
}

// NewRouter creates a new Huma router for handling API requests with
// default middleware and routes attached.
func NewRouter(api *OpenAPI) *Router {
	return NewRouterWithGin(gin.Default(), api)
}

// NewRouterWithGin creates a new Huma router with the given Gin instance
// which may be preconfigured with custom options and middleware.
func NewRouterWithGin(engine *gin.Engine, api *OpenAPI) *Router {
	r := &Router{
		api:    api,
		engine: engine,
	}

	if r.api.Paths == nil {
		r.api.Paths = make(map[string][]*Operation)
	}

	// Add the default context dependency.
	r.deps = make(map[reflect.Type]interface{})
	r.Dependency(func(c *gin.Context, o *Operation) (*gin.Context, error) {
		return c, nil
	})

	// Set up handlers for the auto-generated spec and docs.
	r.engine.GET("/openapi.json", OpenAPIHandler(r.api))
	r.engine.GET("/docs", func(c *gin.Context) {
		c.Data(200, "text/html", []byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
  <head>
    <title>%s</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link href="https://fonts.googleapis.com/css?family=Montserrat:300,400,700|Roboto:300,400,700" rel="stylesheet">
		<style>body { margin: 0; padding: 0; }</style>
  </head>
  <body>
    <redoc spec-url='/openapi.json'></redoc>
    <script src="https://cdn.jsdelivr.net/npm/redoc@next/bundles/redoc.standalone.js"> </script>
  </body>
</html>`, r.api.Title)))
	})

	return r
}

// GinEngine returns the underlying low-level Gin engine.
func (r *Router) GinEngine() *gin.Engine {
	return r.engine
}

// Use attaches middleware to the router.
func (r *Router) Use(middleware ...gin.HandlerFunc) {
	r.engine.Use(middleware...)
}

// ServeHTTP conforms to the `http.Handler` interface.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.engine.ServeHTTP(w, req)
}

// Dependency registers a new dependency type to be injected into handler
// functions, e.g. for loggers, metrics, datastores, etc. Provide a value
// or a function to return a contextual value/error.
//
//  // Register a global dependency like a datastore
//  router.Dependency(&MyDB{...})
//
//  // Register a contextual dependency like a logger
//  router.Dependency(func (c *gin.Context, o *huma.Operation) (*MyLogger, error) {
//    return &MyLogger{Tags: []string{c.Request.RemoteAddr}}, nil
//  })
//
// Then use the dependency in a handler function:
//
//  router.Register(&huma.Operation{
//    ...
//    Handler: func(db *MyDB, log *MyLogger) *MyItem {
//      item := db.GetItem("some-id")
//      log.Info("Got item!")
//      return item
//    }
//  })
func (r *Router) Dependency(f interface{}) {
	fVal := reflect.ValueOf(f)
	outType := fVal.Type()

	if fVal.Kind() == reflect.Func {
		fType := fVal.Type()
		if fType.NumIn() != 2 {
			panic(fmt.Errorf("function should take 2 arguments (*gin.Context, *huma.Operation) but got %s: %w", fType.String(), ErrDependencyInvalid))
		}

		if fType.In(0).String() != "*gin.Context" || fType.In(1).String() != "*huma.Operation" {
			panic(fmt.Errorf("function should take (*gin.Context, *huma.Operation) but got (%s, %s): %w", fType.In(0).String(), fType.In(1).String(), ErrDependencyInvalid))
		}

		if fVal.Type().NumOut() != 2 || fVal.Type().Out(1).Name() != "error" {
			panic(fmt.Errorf("function should return (your-type, error): %w", ErrDependencyInvalid))
		}

		outType = fVal.Type().Out(0)

		if _, ok := r.deps[outType]; ok {
			panic(fmt.Errorf("duplicate type %s: %w", outType.String(), ErrDependencyInvalid))
		}
	}

	// To prevent mistakes we limit dependencies to non-scalar types, since
	// scalars like strings/numbers are typically used for params like headers.
	switch outType.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
		panic(fmt.Errorf("dependeny cannot be scalar type %s: %w", outType.Kind(), ErrDependencyInvalid))
	}

	r.deps[outType] = f
}

// Register a new operation.
func (r *Router) Register(op *Operation) {
	// First, make sure the operation and handler make sense, as well as pre-
	// generating any schemas for use later during request handling.
	if err := op.validate(r.deps); err != nil {
		panic(err)
	}

	// Add the operation to the list of operations for the path entry.
	if r.api.Paths[op.Path] == nil {
		r.api.Paths[op.Path] = make([]*Operation, 0, 1)
	}

	r.api.Paths[op.Path] = append(r.api.Paths[op.Path], op)

	// Next, figure out which Gin function to call.
	var f func(string, ...gin.HandlerFunc) gin.IRoutes

	switch op.Method {
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
		panic("unsupported HTTP method")
	}

	// Then call it to register our handler function.
	f(op.Path, func(c *gin.Context) {
		method := reflect.ValueOf(op.Handler)
		in := make([]reflect.Value, 0, method.Type().NumIn())

		// Process any dependencies first.
		for i := 0; i < method.Type().NumIn(); i++ {
			argType := method.Type().In(i)
			if f, ok := r.deps[argType]; ok {
				// This handler argument matches a known registered dependency. If it's
				// a function, then call it, otherwise just use the value.
				var v reflect.Value
				vf := reflect.ValueOf(f)
				if vf.Kind() == reflect.Func {
					args := []reflect.Value{reflect.ValueOf(c), reflect.ValueOf(op)}
					out := vf.Call(args)
					if !out[1].IsNil() {
						c.AbortWithError(500, out[1].Interface().(error))
						return
					}
					v = out[0]
				} else {
					v = reflect.ValueOf(f)
				}
				in = append(in, v)
			} else {
				// No match, so we're done with dependencies. Keep going below
				// processing params.
				break
			}
		}

		for _, param := range op.Params {
			pv, err := getParamValue(c, param)
			if err != nil {
				// TODO expose error to user
				c.AbortWithError(400, err)
				return
			}

			in = append(in, reflect.ValueOf(pv))
		}

		if len(in) != method.Type().NumIn() {
			// Parse body
			i := len(in)
			val, success := getRequestBody(c, method.Type().In(i), op)
			if !success {
				// Error was already handled in `getRequestBody`.
				return
			}
			in = append(in, reflect.ValueOf(val))
			if in[i].Kind() == reflect.Ptr {
				in[i] = in[i].Elem()
			}
		}

		out := method.Call(in)

		// Find and return the first non-zero response. The status code comes
		// from the registered `huma.Response` struct.
		// This breaks down with scalar types... so they need to be passed
		// as a pointer and we'll dereference it automatically.
		for i, o := range out[len(op.ResponseHeaders):] {
			if !o.IsZero() {
				body := o.Interface()

				r := op.Responses[i]

				// Set response headers
				for j, header := range op.ResponseHeaders {
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
							// TODO: log warning? This shouldn't be set if it won't get sent.
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

				if r.StatusCode == http.StatusNoContent {
					// No body allowed.
					c.Status(r.StatusCode)
					break
				}

				if strings.HasPrefix(r.ContentType, "application/json") {
					c.JSON(r.StatusCode, body)
				} else if strings.HasPrefix(r.ContentType, "application/yaml") {
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

// Run the server.
func (r *Router) Run(addr string) {
	r.engine.Run(addr)
}
