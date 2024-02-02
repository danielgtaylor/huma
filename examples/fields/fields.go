// Example field selection transform enabling a GraphQL-like behavior.
package fields

import (
	"encoding/json"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/shorthand/v2"
)

// FieldSelectTransform is an example of a transform that can use an input
// header value to modify the response on the server, providing a GraphQL-like
// way to send only the fields that the client wants over the wire.
func FieldSelectTransform(ctx huma.Context, status string, v any) (any, error) {
	if fields := ctx.Header("Fields"); fields != "" {
		// Ugh this is inefficient... consider other ways of doing this :-(
		var tmp any
		b, _ := json.Marshal(v)
		json.Unmarshal(b, &tmp)
		result, _, err := shorthand.GetPath(fields, tmp, shorthand.GetOptions{})
		return result, err
	}
	return v, nil
}
