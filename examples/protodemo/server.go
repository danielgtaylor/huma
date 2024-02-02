package main

import (
	"context"
	"io"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/danielgtaylor/huma/v2/examples/protodemo/protodemo"
	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

/*
Create:

restish put :3000/users/daniel -H "Accept:application/protobuf" name: Daniel Taylor, email: dt@example.com, roles[]: admin >user.pb

Convert to JSON:

cat user.pb | restish put :3000/users/daniel -H "Content-Type: application/protobuf" -H "Accept: application/json"
*/

//go:generate protoc --go_out=. model.proto

type PutUserInput struct {
	ID   string `path:"id"`
	Body *protodemo.User
}

type PutUserOutput struct {
	Body *protodemo.User
}

func main() {
	r := chi.NewMux()

	// Define our custom formats.
	jsonFormat := huma.Format{
		Marshal: func(w io.Writer, v any) error {
			b, err := protojson.Marshal(v.(proto.Message))
			if err != nil {
				panic(err)
			}
			_, err = w.Write(b)
			return err
		},
		Unmarshal: func(data []byte, v any) error {
			// v = any as **proto.Message, we need to create a new instance and
			// then let the unmarshaler do its thing.
			vv := reflect.New(reflect.TypeOf(v).Elem().Elem())
			reflect.ValueOf(v).Elem().Set(vv)
			return protojson.Unmarshal(data, vv.Interface().(proto.Message))
		},
	}

	protoFormat := huma.Format{
		Marshal: func(w io.Writer, v any) error {
			b, err := proto.Marshal(v.(proto.Message))
			if err != nil {
				panic(err)
			}
			_, err = w.Write(b)
			return err
		},
		Unmarshal: func(data []byte, v any) error {
			// v = any as **proto.Message, we need to create a new instance and
			// then let the unmarshaler do its thing.
			vv := reflect.New(reflect.TypeOf(v).Elem().Elem())
			reflect.ValueOf(v).Elem().Set(vv)
			return proto.Unmarshal(data, vv.Interface().(proto.Message))
		},
	}

	// Create the API with those formats.
	api := humachi.New(r, huma.Config{
		OpenAPI: &huma.OpenAPI{
			Info: &huma.Info{
				Title:   "My API",
				Version: "1.0.0",
			},
		},
		Formats: map[string]huma.Format{
			"application/json":     jsonFormat,
			"json":                 jsonFormat,
			"application/protobuf": protoFormat,
			"proto":                protoFormat,
		},
	})

	huma.Register(api, huma.Operation{
		OperationID:      "put-user",
		Method:           http.MethodPut,
		Path:             "/users/{id}",
		SkipValidateBody: true,
	}, func(ctx context.Context, input *PutUserInput) (*PutUserOutput, error) {
		return &PutUserOutput{
			Body: &protodemo.User{
				Id:      input.ID,
				Name:    input.Body.Name,
				Email:   input.Body.Email,
				Roles:   input.Body.Roles,
				Updated: timestamppb.Now(),
			},
		}, nil
	})

	http.ListenAndServe(":3000", r)
}
