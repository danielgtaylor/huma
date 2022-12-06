package huma

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"

	"github.com/danielgtaylor/casing"
	"github.com/danielgtaylor/huma/schema"
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
var jsonPatchSchema, _ = schema.Generate(jsonPatchType)

// allResources recursively collects and returns all resources/sub-resources
// attached to a router.
func (r *Router) allResources() []*Resource {
	resources := []*Resource{}
	resources = append(resources, r.resources...)

	for i := 0; i < len(resources); i++ {
		if len(resources[i].subResources) > 0 {
			resources = append(resources, resources[i].subResources...)
		}
	}

	return resources
}

// makeAllOptional recursively makes all fields in a schema optional, useful
// for allowing PATCH operation on just some fields.
func makeAllOptional(s *schema.Schema) {
	if s.Required != nil {
		s.Required = []string{}
	}

	if s.Items != nil {
		makeAllOptional(s.Items)
	}

	for _, props := range []map[string]*schema.Schema{
		s.Properties,
		s.PatternProperties,
	} {
		for _, v := range props {
			makeAllOptional(v)
		}
	}
}

// AutoPatch generates HTTP PATCH operations for any resource which has a
// GET & PUT but no pre-existing PATCH operation. Generated PATCH operations
// will call GET, apply either `application/merge-patch+json` or
// `application/json-patch+json` patches, then call PUT with the updated
// resource. This method is called automatically on server start-up but can
// be called manually (e.g. for tests) and is idempotent.
func (r *Router) AutoPatch() {
	for _, resource := range r.allResources() {
		var get *Operation
		var put *Operation
		hasPatch := false
		var kind reflect.Kind = 0

		for _, op := range resource.operations {
			switch op.method {
			case http.MethodGet:
				get = op
			case http.MethodPut:
				put = op
				_, reqDef := put.requestForContentType("application/json")
				if reqDef != nil && reqDef.model != nil {
					kind = reqDef.model.Kind()
					if kind == reflect.Ptr {
						kind = reqDef.model.Elem().Kind()
					}
				}
			case http.MethodPatch:
				hasPatch = true
			}
		}

		// We need a GET and PUT, but also an object (not array) to patch.
		if get != nil && put != nil && !hasPatch && kind == reflect.Struct {
			generatePatch(resource, get, put)
		}
	}
}

// copyHeaders copies all headers from one header object into another, useful
// for creating a new request with headers that match an existing request.
func copyHeaders(from, to http.Header) {
	for k, values := range from {
		for _, v := range values {
			to.Add(k, v)
		}
	}
}

// generatePatch is called for each resource which needs a PATCH operation to
// be added. it registers and provides a handler for this new operation.
func generatePatch(resource *Resource, get *Operation, put *Operation) {
	_, reqDef := put.requestForContentType("application/json")

	s, _ := schema.Generate(reqDef.model)
	makeAllOptional(s)

	// Guess a name for this patch operation based on the GET operation.
	name := ""
	parts := casing.Split(get.id)
	if len(parts) > 1 && (strings.ToLower(parts[0]) == "get" || strings.ToLower(parts[0]) == "fetch") {
		parts = parts[1:]
	}
	name = casing.Join(parts, "-")

	// Augment the response list with ones we may return from the PATCH.
	responses := append([]Response{}, put.responses...)
	for _, code := range []int{
		http.StatusNotModified,
		http.StatusBadRequest,
		http.StatusUnprocessableEntity,
		http.StatusUnsupportedMediaType,
	} {
		found := false
		for _, resp := range responses {
			if resp.status == code {
				found = true
				break
			}
		}
		if !found {
			responses = append(responses, NewResponse(code, http.StatusText(code)).
				ContentType("application/problem+json").
				Model(&ErrorModel{}),
			)
		}
	}

	// Manually register the operation so it shows up in the generated OpenAPI.
	resource.operations = append(resource.operations, &Operation{
		resource:    resource,
		method:      http.MethodPatch,
		id:          "patch-" + name,
		summary:     "Patch " + name,
		description: "Partial update operation supporting both JSON Merge Patch & JSON Patch updates.",
		params:      put.params,
		paramsOrder: put.paramsOrder,
		requests: map[string]*request{
			"application/merge-patch+json": {
				override: true,
				schema:   s,
				model:    reqDef.model,
			},
			"application/json-patch+json": {
				override: true,
				schema:   jsonPatchSchema,
				model:    jsonPatchType,
			},
		},
		responses:  responses,
		deprecated: put.deprecated,
	})

	// Manually register the handler with the router. This bypasses the normal
	// Huma API since this is easier and we are just calling the other pre-existing
	// operations.
	resource.router.mux.Patch(resource.path, func(w http.ResponseWriter, r *http.Request) {
		ctx := ContextFromRequest(w, r)

		patchData, err := ioutil.ReadAll(r.Body)
		if err != nil {
			ctx.WriteError(http.StatusBadRequest, "Unable to read request body", err)
			return
		}

		// Perform the get!
		origReq, err := http.NewRequest(http.MethodGet, r.URL.Path, nil)
		if err != nil {
			ctx.WriteError(http.StatusBadRequest, "Unable to get resource", err)
			return
		}
		copyHeaders(r.Header, origReq.Header)
		origReq.Header.Set("Accept", "application/json")
		origReq.Header.Set("Accept-Encoding", "")

		// Conditional request headers will be used on the write side, so ignore
		// them on the read.
		origReq.Header.Del("If-Match")
		origReq.Header.Del("If-None-Match")
		origReq.Header.Del("If-Modified-Since")
		origReq.Header.Del("If-Unmodified-Since")

		origWriter := httptest.NewRecorder()
		resource.router.ServeHTTP(origWriter, origReq)

		if origWriter.Code >= 300 {
			// This represents an error on the GET side.
			copyHeaders(origWriter.Header(), w.Header())
			w.WriteHeader(origWriter.Code)
			w.Write(origWriter.Body.Bytes())
			return
		}

		// Patch the data!
		var patched []byte
		switch strings.Split(r.Header.Get("Content-Type"), ";")[0] {
		case "application/json-patch+json":
			patch, err := jsonpatch.DecodePatch(patchData)
			if err != nil {
				ctx.WriteError(http.StatusUnprocessableEntity, "Unable to decode patch", err)
				return
			}
			patched, err = patch.Apply(origWriter.Body.Bytes())
			if err != nil {
				ctx.WriteError(http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}
		case "application/merge-patch+json", "application/json", "":
			// Assume most cases are merge-patch.
			patched, err = jsonpatch.MergePatch(origWriter.Body.Bytes(), patchData)
			if err != nil {
				ctx.WriteError(http.StatusUnprocessableEntity, "Unable to apply patch", err)
				return
			}
		default:
			// A content type we explicitly do not support was passed.
			ctx.WriteError(http.StatusUnsupportedMediaType, "Content type should be one of application/merge-patch+json or application/json-patch+json")
			return
		}

		if bytes.Compare(patched, origWriter.Body.Bytes()) == 0 {
			ctx.WriteHeader(http.StatusNotModified)
			return
		}

		// Write the updated data back to the server!
		putReq, err := http.NewRequest(http.MethodPut, r.URL.Path, bytes.NewReader(patched))
		if err != nil {
			ctx.WriteError(http.StatusInternalServerError, "Unable to put modified resource", err)
		}
		copyHeaders(r.Header, putReq.Header)

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

		resource.router.ServeHTTP(w, putReq)
	})
}
