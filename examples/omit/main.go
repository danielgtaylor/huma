// This example shows how to handle omittable/nullable fields and an optional
// body in JSON input. Try the following requests:
//
//	# Omit the body
//	restish post :8888/omittable
//
//	# Send a null body
//	echo '{}' | restish post :8888/omittable
//
//	# Send a body with a null name
//	restish post :8888/omittable name: null
//
//	# Send a body with a name
//	restish post :8888/omittable name: Kari
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// Options for the CLI.
type Options struct {
	Port int `help:"Port to listen on" default:"8888"`
}

// OmittableNullable is a field which can be omitted from the input,
// set to `null`, or set to a value. Each state is tracked and can
// be checked for in handling code.
type OmittableNullable[T any] struct {
	Sent  bool
	Null  bool
	Value T
}

// UnmarshalJSON unmarshals this value from JSON input.
func (o *OmittableNullable[T]) UnmarshalJSON(b []byte) error {
	if len(b) > 0 {
		o.Sent = true
		if bytes.Equal(b, []byte("null")) {
			o.Null = true
			return nil
		}
		return json.Unmarshal(b, &o.Value)
	}
	return nil
}

// Schema returns a schema representing this value on the wire.
// It returns the schema of the contained type.
func (o OmittableNullable[T]) Schema(r huma.Registry) *huma.Schema {
	return r.Schema(reflect.TypeOf(o.Value), true, "")
}

type MyResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

func main() {
	// Create a CLI app which takes a port option.
	cli := huma.NewCLI(func(hooks huma.Hooks, options *Options) {
		// Create a new router & API
		router := chi.NewMux()
		api := humachi.New(router, huma.DefaultConfig("My API", "1.0.0"))

		huma.Register(api, huma.Operation{
			OperationID: "omittable",
			Method:      http.MethodPost,
			Path:        "/omittable",
			Summary:     "Omittable / nullable example",
		}, func(ctx context.Context, input *struct {
			// Making the body a pointer makes it optional, as it may be `nil`.
			Body *struct {
				Name OmittableNullable[string] `json:"name,omitempty" maxLength:"10"`
			}
		}) (*MyResponse, error) {
			resp := &MyResponse{}
			if input.Body == nil {
				resp.Body.Message = "Body was not sent"
			} else if !input.Body.Name.Sent {
				resp.Body.Message = "Name was omitted from the request"
			} else if input.Body.Name.Null {
				resp.Body.Message = "Name was set to null"
			} else {
				resp.Body.Message = "Name was set to: " + input.Body.Name.Value
			}
			return resp, nil
		})

		// Tell the CLI how to start your router.
		hooks.OnStart(func() {
			http.ListenAndServe(fmt.Sprintf(":%d", options.Port), router)
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()
}
