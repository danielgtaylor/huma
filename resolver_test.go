package huma_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// Step 1: Create your input struct where you want to do additional validation.
// This struct must implement the `huma.Resolver` interface.
type ExampleInputBody struct {
	Count int `json:"count" minimum:"0"`
}

func (b *ExampleInputBody) Resolve(ctx huma.Context, prefix *huma.PathBuffer) []error {
	// Return an error if some arbitrary rule is broken. In this case, if it's
	// a multiple of 30 we return an error.
	if b.Count%30 == 0 {
		return []error{&huma.ErrorDetail{
			Location: prefix.With("count"),
			Message:  "multiples of 30 are not allowed",
			Value:    b.Count,
		}}
	}

	return nil
}

func ExampleResolver() {
	// Create the API.
	r := http.NewServeMux()
	api := NewExampleAPI(r, huma.DefaultConfig("Example API", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "resolver-example",
		Method:      http.MethodPut,
		Path:        "/resolver",
	}, func(ctx context.Context, input *struct {
		// Step 2: Use your custom struct with the resolver as a field in the
		// request input. Here we use it as the body of the request.
		Body ExampleInputBody
	}) (*struct{}, error) {
		// Do nothing. Validation should catch the error!
		return nil, nil
	})

	// Make an example request showing the validation error response.
	req, _ := http.NewRequest(http.MethodPut, "/resolver", strings.NewReader(`{"count": 30}`))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	out := bytes.NewBuffer(nil)
	json.Indent(out, w.Body.Bytes(), "", "  ")
	fmt.Println(out.String())
	// Output: {
	//   "$schema": "https://example.com/schemas/ErrorModel.json",
	//   "title": "Unprocessable Entity",
	//   "status": 422,
	//   "detail": "validation failed",
	//   "errors": [
	//     {
	//       "message": "multiples of 30 are not allowed",
	//       "location": "body.count",
	//       "value": 30
	//     }
	//   ]
	// }
}
