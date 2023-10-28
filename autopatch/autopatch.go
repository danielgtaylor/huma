// Package autopatch provides a way to automatically generate PATCH operations
// for resources which have a GET & PUT but no PATCH. This is useful for
// resources which are large and have many fields, but where the majority of
// updates are only to a few fields. This allows clients to send a partial
// update to the server without having to send the entire resource.
//
// JSON Merge Patch, JSON Patch, and Shorthand Merge Patch are supported as
// input formats.
package autopatch

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/casing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/shorthand/v2"
	jsonpatch "github.com/evanphx/json-patch/v5"
)

// jsonPatchOp describes an RFC 6902 JSON Patch operation. See also:
// https://www.rfc-editor.org/rfc/rfc6902
type jsonPatchOp struct {
	Op    string      `json:"op" enum:"add,remove,replace,move,copy,test" doc:"Operation name"`
	From  string      `json:"from,omitempty" doc:"JSON Pointer for the source of a move or copy"`
	Path  string      `json:"path" doc:"JSON Pointer to the field being operated on, or the destination of a move/copy operation"`
	Value interface{} `json:"value,omitempty" doc:"The value to set"`
}

var jsonPatchType = reflect.TypeOf([]jsonPatchOp{})

// AutoPatch generates HTTP PATCH operations for any resource which has a GET &
// PUT but no pre-existing PATCH operation. Generated PATCH operations will call
// GET, apply either `application/merge-patch+json`,
// `application/json-patch+json`, or `application/merge-patch+shorthand`
// patches, then call PUT with the updated resource. This method may be safely
// called multiple times.
func AutoPatch(api huma.API) {
	oapi := api.OpenAPI()
	registry := oapi.Components.Schemas
	for _, path := range oapi.Paths {
		if path.Get != nil && path.Put != nil && path.Patch == nil {
			body := path.Put.RequestBody
			if body != nil && body.Content != nil && body.Content["application/json"] != nil {
				ct := body.Content["application/json"]
				if ct.Schema != nil {
					s := ct.Schema
					if s.Ref != "" {
						// Dereference if needed so we can find the underlying type.
						s = registry.SchemaFromRef(s.Ref)
					}
					// Only objects can be patched automatically. No arrays or
					// primitives so skip those.
					if s.Type == "object" {
						generatePatch(api, path)
					}
				}
			}
		}
	}
}

// generatePatch is called for each resource which needs a PATCH operation to
// be added. it registers and provides a handler for this new operation.
func generatePatch(api huma.API, path *huma.PathItem) {
	oapi := api.OpenAPI()
	get := path.Get
	put := path.Put

	jsonPatchSchema := oapi.Components.Schemas.Schema(jsonPatchType, true, "")

	// Guess a name for this patch operation based on the GET operation.
	name := ""
	parts := casing.Split(get.OperationID)
	if len(parts) > 1 && (strings.ToLower(parts[0]) == "get" || strings.ToLower(parts[0]) == "fetch") {
		parts = parts[1:]
	}
	name = casing.Join(parts, "-")

	// Augment the response list with ones we may return from the PATCH.
	responses := make(map[string]*huma.Response, len(put.Responses))
	for k, v := range put.Responses {
		responses[k] = v
	}
	statuses := append([]int{}, put.Errors...)
	if responses["default"] == nil {
		for _, code := range []int{
			http.StatusNotModified,
			http.StatusBadRequest,
			http.StatusUnprocessableEntity,
			http.StatusUnsupportedMediaType,
		} {
			found := false
			for statusStr := range put.Responses {
				if statusStr == strconv.Itoa(code) {
					found = true
					break
				}
			}
			for status := range put.Errors {
				if status == code {
					found = true
					break
				}
			}
			if !found {
				statuses = append(statuses, code)
			}
		}
	}

	// Manually register the operation so it shows up in the generated OpenAPI.
	op := &huma.Operation{
		OperationID:  "patch-" + name,
		Method:       http.MethodPatch,
		Path:         put.Path,
		Summary:      "Patch " + name,
		Description:  "Partial update operation supporting both JSON Merge Patch & JSON Patch updates.",
		Tags:         put.Tags,
		Deprecated:   put.Deprecated,
		MaxBodyBytes: put.MaxBodyBytes,
		Parameters:   put.Parameters,
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"application/merge-patch+json": {
					Schema: &huma.Schema{
						Type:                 huma.TypeObject,
						Description:          "JSON merge patch object, see PUT operation for schema. All fields are optional.",
						AdditionalProperties: true,
					},
				},
				"application/merge-patch+shorthand": {
					Schema: &huma.Schema{
						Type:                 huma.TypeObject,
						Description:          "Shorthand merge patch object, see PUT operation for schema. All fields are optional.",
						AdditionalProperties: true,
					},
				},
				"application/json-patch+json": {
					Schema: jsonPatchSchema,
				},
			},
		},
		Responses: responses,
		Errors:    statuses,
		Callbacks: put.Callbacks,
		Security:  put.Security,
		Servers:   put.Servers,
	}
	oapi.AddOperation(op)

	// Manually register the handler with the router. This bypasses the normal
	// Huma API since this is easier and we are just calling the other pre-existing
	// operations.
	adapter := api.Adapter()
	adapter.Handle(op, func(ctx huma.Context) {
		patchData, err := io.ReadAll(ctx.BodyReader())
		if err != nil {
			huma.WriteErr(api, ctx, http.StatusBadRequest, "Unable to read request body", err)
			return
		}

		// Perform the get!
		origReq, err := http.NewRequest(http.MethodGet, ctx.URL().Path, nil)
		if err != nil {
			huma.WriteErr(api, ctx, http.StatusInternalServerError, "Unable to get resource", err)
			return
		}

		// Copy incoming headers.
		ctx.EachHeader(func(k, v string) {
			if k == "Accept" || k == "Accept-Encoding" {
				// We will force these to be JSON for easier handling.
				return
			}
			if k == "If-Match" || k == "If-None-Match" || k == "If-Modified-Since" || k == "If-Unmodified-Since" {
				// Conditional request headers will be used on the write side, so
				// ignore them here.
				return
			}
			if k == "Content-Type" || k == "Content-Length" {
				// GET will be empty.
				return
			}
			origReq.Header.Add(k, v)
		})

		// Accept JSON for the patches.
		// TODO: could we accept other stuff here...?
		ctx.SetHeader("Accept", "application/json")
		ctx.SetHeader("Accept-Encoding", "")

		origWriter := httptest.NewRecorder()
		adapter.ServeHTTP(origWriter, origReq)

		if origWriter.Code >= 300 {
			// This represents an error on the GET side.
			for key, values := range origWriter.Header() {
				for _, value := range values {
					ctx.SetHeader(key, value)
				}
			}
			ctx.SetStatus(origWriter.Code)
			io.Copy(ctx.BodyWriter(), origWriter.Body)
			return
		}

		// Patch the data!
		var patched []byte
		switch strings.Split(ctx.Header("Content-Type"), ";")[0] {
		case "application/json-patch+json":
			patch, err := jsonpatch.DecodePatch(patchData)
			if err != nil {
				huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "Unable to decode JSON Patch", err)
				return
			}
			patched, err = patch.Apply(origWriter.Body.Bytes())
			if err != nil {
				huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}
		case "application/merge-patch+json", "application/json", "":
			// Assume most cases are merge-patch.
			patched, err = jsonpatch.MergePatch(origWriter.Body.Bytes(), patchData)
			if err != nil {
				huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}
		case "application/merge-patch+shorthand":
			// Load the original data so it can be used as a base.
			var tmp any
			if err := json.Unmarshal(origWriter.Body.Bytes(), &tmp); err != nil {
				huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}

			// Unmarshal the shorthand over the existing data.
			tmp, err = shorthand.Unmarshal(string(patchData), shorthand.ParseOptions{
				ForceStringKeys: true,
			}, tmp)
			if err != nil {
				huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}

			// Marshal the updated data back for the request to PUT.
			patched, err = json.Marshal(tmp)
			if err != nil {
				huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}
		default:
			// A content type we explicitly do not support was passed.
			huma.WriteErr(api, ctx, http.StatusUnsupportedMediaType, "Content type should be one of application/merge-patch+json or application/json-patch+json")
			return
		}

		if bytes.Equal(bytes.TrimSpace(patched), bytes.TrimSpace(origWriter.Body.Bytes())) {
			ctx.SetStatus(http.StatusNotModified)
			return
		}

		// Write the updated data back to the server!
		putReq, err := http.NewRequest(http.MethodPut, ctx.URL().Path, bytes.NewReader(patched))
		if err != nil {
			huma.WriteErr(api, ctx, http.StatusInternalServerError, "Unable to put modified resource", err)
			return
		}
		ctx.EachHeader(func(k, v string) {
			if k == "Content-Type" || k == "Content-Length" {
				return
			}
			putReq.Header.Add(k, v)
		})

		putReq.Header.Set("Content-Type", "application/json")

		h := putReq.Header
		if h.Get("If-Match") == "" && h.Get("If-None-Match") == "" && h.Get("If-Unmodified-Since") == "" && h.Get("If-Modified-Since") == "" {
			// No conditional headers have been set on the request. Can we set one?
			// If we have an ETag or last modified time then we can set a corresponding
			// conditional request header to prevent overwriting someone else's
			// changes between when we did our GET and are doing our PUT.
			// Distributed write failures will result in a 412 Precondition Failed.
			oh := origWriter.Header()
			if etag := oh.Get("ETag"); etag != "" {
				h.Set("If-Match", etag)
			} else if modified := oh.Get("Last-Modified"); modified != "" {
				h.Set("If-Unmodified-Since", modified)
			}
		}

		putWriter := httptest.NewRecorder()
		adapter.ServeHTTP(putWriter, putReq)
		for key, values := range putWriter.Header() {
			for _, value := range values {
				ctx.SetHeader(key, value)
			}
		}
		ctx.SetStatus(putWriter.Code)
		io.Copy(ctx.BodyWriter(), putWriter.Body)
	})
}
