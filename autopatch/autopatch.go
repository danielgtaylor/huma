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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/casing"
	"github.com/danielgtaylor/shorthand/v2"
	jsonpatch "github.com/evanphx/json-patch/v5"
)

const MergePatchNullabilityExtension = "x-merge-patch-nullability"

type MergePatchNullabilitySettings struct {
	Enabled                    bool
	StringRepresentationOfNull string
}

func RegisterNullabilityExtension(api huma.API, stringRepresentationOfNull string) {
	if api.OpenAPI().Extensions == nil {
		api.OpenAPI().Extensions = make(map[string]any)
	}
	api.OpenAPI().Extensions[MergePatchNullabilityExtension] = MergePatchNullabilitySettings{
		Enabled:                    true,
		StringRepresentationOfNull: stringRepresentationOfNull,
	}
}

func replaceNulls(data []byte, settings MergePatchNullabilitySettings) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var replaceNullsInValue func(v any) any
	replaceNullsInValue = func(v any) any {
		switch x := v.(type) {
		case nil:
			return settings.StringRepresentationOfNull
		case map[string]any:
			for k, v := range x {
				x[k] = replaceNullsInValue(v)
			}
			return x
		case []any:
			for i, v := range x {
				x[i] = replaceNullsInValue(v)
			}
			return x
		default:
			return v
		}
	}

	result := replaceNullsInValue(raw)
	return json.Marshal(result)
}

func restoreNulls(data []byte, settings MergePatchNullabilitySettings) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	var restoreNullsInValue func(v any) any
	restoreNullsInValue = func(v any) any {
		switch x := v.(type) {
		case string:
			if x == settings.StringRepresentationOfNull {
				return nil
			}
			return x
		case map[string]any:
			for k, v := range x {
				x[k] = restoreNullsInValue(v)
			}
			return x
		case []any:
			for i, v := range x {
				x[i] = restoreNullsInValue(v)
			}
			return x
		default:
			return v
		}
	}

	result := restoreNullsInValue(raw)
	return json.Marshal(result)
}

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
//
// If you wish to disable autopatching for a specific resource, set the
// `autopatch` operation metadata field to `false` on the GET or PUT
// operation and it will be skipped.
func AutoPatch(api huma.API) {
	oapi := api.OpenAPI()
	registry := oapi.Components.Schemas
Outer:
	for _, path := range oapi.Paths {
		if path.Get != nil && path.Put != nil && path.Patch == nil {
			for _, op := range []*huma.Operation{path.Get, path.Put} {
				if op.Metadata != nil && op.Metadata["autopatch"] != nil {
					if b, ok := op.Metadata["autopatch"].(bool); ok && !b {
						// Special case: explicitly disabled.
						continue Outer
					}
				}
			}
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
						PatchResource(api, path)
					}
				}
			}
		}
	}
}

// PatchResource is called for each resource which needs a PATCH operation to
// be added. It registers and provides a handler for this new operation. You
// may call this manually if you prefer to not use `AutoPatch` for all of
// your resources and want more fine-grained control.
func PatchResource(api huma.API, path *huma.PathItem) {
	oapi := api.OpenAPI()
	get := path.Get
	put := path.Put

	jsonPatchSchema := oapi.Components.Schemas.Schema(jsonPatchType, true, "")

	// Guess a name for this patch operation based on the GET operation.
	var name string
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

	// Get the schema from the PUT operation
	putSchema := put.RequestBody.Content["application/json"].Schema
	if putSchema.Ref != "" {
		putSchema = oapi.Components.Schemas.SchemaFromRef(putSchema.Ref)
	}

	// Create an optional version of the PUT schema
	optionalPutSchema := makeOptionalSchema(putSchema)

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
					Schema: optionalPutSchema,
				},
				"application/merge-patch+shorthand": {
					Schema: optionalPutSchema,
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

	// Manually register the handler with the router.
	adapter := api.Adapter()
	adapter.Handle(op, func(ctx huma.Context) {
		patchData, err := io.ReadAll(ctx.BodyReader())
		if err != nil {
			huma.WriteErr(api, ctx, http.StatusBadRequest, "Unable to read request body", err)
			return
		}

		resourcePath := findRelativeResourcePath(ctx.URL().Path, put.Path)

		// Perform the get!
		origReq, err := http.NewRequest(http.MethodGet, resourcePath, nil)
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
		origReq.Header.Set("Accept", "application/json")
		origReq.Header.Set("Accept-Encoding", "")

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
			// Check if we are using the merge-patch-nullability extension
			var preserveNullsInMergePatch bool
			var nullabilitySettings MergePatchNullabilitySettings
			if extension, ok := oapi.Extensions[MergePatchNullabilityExtension]; ok {
				if nullabilitySettings, ok = extension.(MergePatchNullabilitySettings); !ok {
					huma.WriteErr(api, ctx, http.StatusInternalServerError, "Unable to parse nullability settings", fmt.Errorf("invalid nullability settings type"))
					return
				} else if nullabilitySettings.Enabled {
					preserveNullsInMergePatch = true
				}
			}
			if preserveNullsInMergePatch {
				// Replace nulls with the string representation
				patchData, err = replaceNulls(patchData, nullabilitySettings)
				if err != nil {
					huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "failed to replace nulls in request", err)
					return
				}
			}
			patched, err = jsonpatch.MergePatch(origWriter.Body.Bytes(), patchData)
			if err != nil {
				huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}
			if preserveNullsInMergePatch {
				// Replace null string representations with nulls
				patched, err = restoreNulls(patched, nullabilitySettings)
				if err != nil {
					huma.WriteErr(api, ctx, http.StatusUnprocessableEntity, "failed to replace nulls in request", err)
					return
				}
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

		if jsonpatch.Equal(patched, origWriter.Body.Bytes()) {
			ctx.SetStatus(http.StatusNotModified)
			return
		}

		// Write the updated data back to the server!
		putReq, err := http.NewRequest(http.MethodPut, resourcePath, bytes.NewReader(patched))
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

func makeOptionalSchema(s *huma.Schema) *huma.Schema {
	if s == nil {
		return nil
	}

	optionalSchema := &huma.Schema{
		Type:                 s.Type,
		Title:                s.Title,
		Description:          s.Description,
		Format:               s.Format,
		ContentEncoding:      s.ContentEncoding,
		Default:              s.Default,
		Examples:             s.Examples,
		AdditionalProperties: s.AdditionalProperties,
		Enum:                 s.Enum,
		Minimum:              s.Minimum,
		ExclusiveMinimum:     s.ExclusiveMinimum,
		Maximum:              s.Maximum,
		ExclusiveMaximum:     s.ExclusiveMaximum,
		MultipleOf:           s.MultipleOf,
		MinLength:            s.MinLength,
		MaxLength:            s.MaxLength,
		Pattern:              s.Pattern,
		PatternDescription:   s.PatternDescription,
		MinItems:             s.MinItems,
		MaxItems:             s.MaxItems,
		UniqueItems:          s.UniqueItems,
		MinProperties:        s.MinProperties,
		MaxProperties:        s.MaxProperties,
		ReadOnly:             s.ReadOnly,
		WriteOnly:            s.WriteOnly,
		Deprecated:           s.Deprecated,
		Extensions:           s.Extensions,
		DependentRequired:    s.DependentRequired,
		Discriminator:        s.Discriminator,
	}

	if s.Items != nil {
		optionalSchema.Items = makeOptionalSchema(s.Items)
	}

	if s.Properties != nil {
		optionalSchema.Properties = make(map[string]*huma.Schema)
		for k, v := range s.Properties {
			optionalSchema.Properties[k] = makeOptionalSchema(v)
		}
	}

	if s.OneOf != nil {
		optionalSchema.OneOf = make([]*huma.Schema, len(s.OneOf))
		for i, schema := range s.OneOf {
			optionalSchema.OneOf[i] = makeOptionalSchema(schema)
		}
	}

	if s.AnyOf != nil {
		optionalSchema.AnyOf = make([]*huma.Schema, len(s.AnyOf))
		for i, schema := range s.AnyOf {
			optionalSchema.AnyOf[i] = makeOptionalSchema(schema)
		}
	}

	if s.AllOf != nil {
		optionalSchema.AllOf = make([]*huma.Schema, len(s.AllOf))
		for i, schema := range s.AllOf {
			optionalSchema.AllOf[i] = makeOptionalSchema(schema)
		}
	}

	if s.Not != nil {
		optionalSchema.Not = makeOptionalSchema(s.Not)
	}

	// Make all properties optional
	optionalSchema.Required = nil

	return optionalSchema
}

// This function help to find the relative path of the resource to patch
// this allow to handle potential prefix in the path
// for example if the requestPath is /api/v1/user/1 and the put path is /user/{id}
// the function will return /user/1
func findRelativeResourcePath(requestPath string, putPath string) string {
	putPathParts := strings.Split(putPath, "/")
	// if the path is not deep enough, we return the original path
	if len(putPathParts) < 2 {
		return requestPath
	}
	wantedPrefix := putPathParts[1]
	workingPath := requestPath
	for !strings.HasPrefix(workingPath, wantedPrefix) {
		// we find the next /
		slashIndex := strings.Index(workingPath, "/")
		// if we don't have a / anymore, we return the original path
		if slashIndex == -1 {
			return requestPath
		}
		// we remove till the next /
		workingPath = workingPath[slashIndex+1:]
		// if we reach the end of the path, we return the original path
		if workingPath == "" {
			return requestPath
		}
	}
	return "/" + workingPath
}
