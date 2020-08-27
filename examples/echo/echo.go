package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/cli"
	"github.com/danielgtaylor/huma/responses"
)

// Standard middleware is supported, works with streaming. Useful for stuff
// like compression, request IDs, panic recovery/logging, etc.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Request-ID", "abc123")
		next.ServeHTTP(w, r)
	})
}

type EchoRequest struct {
	Word  string `path:"word" doc:"The word to echo back. Cannot be 'test'."`
	Greet bool   `query:"greet" doc:"Return a greeting" default:"false"`
	Foo   int    `query:"foo" enum:"1,3,5,7" doc:"..."`
}

func (e *EchoRequest) Resolve(ctx huma.Context, r *http.Request) {
	// Extra validation for returning exhaustive errors. Huma handles returning
	// the actual errors after resolving all request dependencies.
	if e.Word == "test" {
		ctx.AddError(&huma.ErrorDetail{
			Message:  "disallowed word value",
			Location: "path.word",
			Value:    e.Word,
		})
	}

	// Post-processing of fields. You can also access the raw request for
	// anything you can't model via tags.
	e.Word = strings.ToLower(e.Word)
}

type EtagHeader struct {
	Etag string `header:"etag" doc:"some docs..."`
}

// EchoResponse message which echoes a value.
type EchoResponse struct {
	EtagHeader
	Body struct {
		Value string `json:"value"`
		Foo   int    `json:"foo,omitempty"`
	}
}

func main() {
	app := cli.NewRouter("My API", "1.0.0")

	app.Middleware(requestIDMiddleware)

	app.Resource("/echo", "word").
		//WithTags("echo-tag").
		Get("echo", "Echo back an input word",
			responses.OK().Model(&EchoResponse{}),
			responses.BadRequest(),
		).
		//WithDeadline(30 * time.Second).
		Run(func(ctx huma.Context, input EchoRequest) {
			v := input.Word
			if input.Greet {
				v = "Hello, " + v
			}

			resp := &EchoResponse{}
			resp.Etag = `W/"foo"`
			resp.Body.Value = v
			resp.Body.Foo = input.Foo

			panic(fmt.Errorf("some error"))

			ctx.WriteModel(http.StatusOK, resp)
		})

	app.Run()
}
