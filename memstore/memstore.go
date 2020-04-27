// Package memstore provides an in-memory data store for auto-resources.
package memstore

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/schema"
	"github.com/fatih/structs"
	"github.com/gosimple/slug"
	"github.com/mitchellh/copystructure"
	"github.com/mitchellh/mapstructure"
)

var (
	// ErrAutoResourceInvalid is used when the resource input is invalid.
	ErrAutoResourceInvalid = errors.New("AutoResource invalid")
)

type createHook interface {
	OnCreate()
}

func fillMap(i interface{}, m map[string]interface{}) {
	s := structs.New(i)
	s.TagName = "json"
	s.FillMap(m)
}

func etagValue(etag string) string {
	etag = strings.Trim(etag, " ")
	etag = strings.TrimPrefix(etag, "W/")
	etag = strings.Trim(etag, `"`)
	return etag
}

// etagCompare returns true if the current ETag matches any in the values set.
// values should be input from a header like `If-Match` and looks like
// `W/"abc123", "def456", ...`.
func etagCompare(values, current string) bool {
	for _, check := range strings.Split(values, ",") {
		if etagValue(check) == current {
			return true
		}
	}
	return false
}

func checkConditionals(existing map[string]interface{}, ifMatch, ifNoneMatch string, ifUnmodified, ifModified time.Time) error {
	existingETag := ""
	if existing != nil {
		existingETag = existing["etag"].(string)
	}

	if ifMatch != "" {
		if existing == nil {
			return fmt.Errorf("No existing item to compare ETags")
		}

		if ifMatch != "*" && !etagCompare(ifMatch, existingETag) {
			return fmt.Errorf("ETag %s did not match: %s", existingETag, ifMatch)
		}
	}

	if ifNoneMatch != "" {
		if existing != nil {
			if etagValue(ifNoneMatch) == "*" {
				return fmt.Errorf("Item already exists with ETag %s", existingETag)
			}

			if etagCompare(ifNoneMatch, existingETag) {
				return fmt.Errorf("ETag %s matched: %s", existingETag, ifNoneMatch)
			}
		}
	}

	// ETag checks replace date checks if present, see:
	// https://tools.ietf.org/html/rfc7232#section-3.4
	if ifMatch == "" && ifNoneMatch == "" && !ifUnmodified.IsZero() {
		if existing == nil {
			return fmt.Errorf("No existing item to compare modification date")
		}

		modified := existing["modified"].(time.Time)
		if modified.After(ifUnmodified) {
			return fmt.Errorf("Item modified after %v: last-modified at %v", ifUnmodified, modified)
		}
	}

	if ifMatch == "" && ifNoneMatch == "" && !ifModified.IsZero() {
		modified := existing["modified"].(time.Time)
		if modified.Before(ifModified) {
			return fmt.Errorf("Item not modified since %s: last-modified: %s", ifModified, modified)
		}
	}

	return nil
}

type config struct {
	single string
	plural string
}

// Option sets an option for the auto-resource.
type Option func(*config)

// Name manually sets the single and plural variants of the resource name. If
// not passed, the names are generated from the path.
func Name(single, plural string) Option {
	return func(c *config) {
		c.single = single
		c.plural = plural
	}
}

// New creates a new memory store.
func New() *MemoryStore {
	return &MemoryStore{}
}

// MemoryStore uses an in-memory data store to create automatic CRUD operation
// handlers based on a Go struct.
type MemoryStore struct {
	// db is a goroutine-safe map where the keys are the collection names and
	// the values are another map of id => stored item.
	db sync.Map

	// mu is a datastore-global lock used when atomic operations aren't available,
	// such as when you need to read, check, then write.
	mu sync.Mutex
}

// collection returns the goroutine-safe map for the given collection name.
func (m *MemoryStore) collection(name string) *sync.Map {
	c, _ := m.db.LoadOrStore(name, &sync.Map{})
	return c.(*sync.Map)
}

// validate panics if the input struct is invalid.
func (m *MemoryStore) validate(typ reflect.Type) {
	if typ.Kind() != reflect.Struct {
		panic(fmt.Errorf("must use struct but got %s: %w", typ, ErrAutoResourceInvalid))
	}

	if id, ok := typ.FieldByName("ID"); !ok {
		panic(fmt.Errorf("ID field required: %w", ErrAutoResourceInvalid))
	} else if id.Type.Kind() != reflect.String {
		panic(fmt.Errorf("ID must be string, but got %v: %w", id.Type, ErrAutoResourceInvalid))
	} else {
		if t, ok := id.Tag.Lookup("json"); !ok {
			panic(fmt.Errorf("ID missing json tag: %w", ErrAutoResourceInvalid))
		} else if t != "id" {
			panic(fmt.Errorf("ID must marshal as `id` but got %s: %w", t, ErrAutoResourceInvalid))
		}
	}
}

// AutoResource creates a new resource backed by this memory store. By default,
// this method will generate CRUD-style operations agains the given data
// structure. The data structure must have an ID string field that marshals to
// an `id` JSON property.
func (m *MemoryStore) AutoResource(r *huma.Resource, dataStructure interface{}, options ...Option) *huma.Resource {
	// Validate input structure for ID and other fields
	typ := reflect.TypeOf(dataStructure)
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	m.validate(typ)

	// Setup defaults and then process any passed options.
	parts := strings.Split(r.Path(), "/")
	name := strings.ToLower(parts[len(parts)-1])
	cfg := &config{
		single: strings.TrimRight(name, "s"),
		plural: name,
	}

	for _, option := range options {
		option(cfg)
	}

	// Figure out default summary fields
	// TODO... get from struct tags or via passed option? For now default to
	// the most useful fields to identify the object.
	summaryFields := []string{"id", "modified", "etag"}

	// Collection name is based on the current path.
	collectionName := slug.Make(r.Path())

	// Get a schema from the internal model (struct).
	rs, _ := schema.GenerateWithMode(typ, schema.ModeRead, nil)
	ws, _ := schema.GenerateWithMode(typ, schema.ModeWrite, nil)

	// Remove any existing path params from the external model. These will still
	// get set in the data store and hooks can read them, but we don't send
	// them over the wire.
	for _, p := range r.PathParams() {
		rs.RemoveProperty(p)
		ws.RemoveProperty(p)
	}

	listItem, _ := copystructure.Copy(rs)
	r.With(
		// TODO: filters?
		huma.QueryParam("fields", "List of fields to include", summaryFields),
		// TODO: pagination
		huma.ResponseJSON(http.StatusOK, "Success", huma.Schema(schema.Schema{
			Type:  "array",
			Items: listItem.(*schema.Schema),
		})),
	).List("List "+cfg.plural, huma.UnsafeHandler(func(inputs ...interface{}) []interface{} {
		i := len(r.PathParams())
		fields := inputs[i].([]string)

		items := make([]interface{}, 0)
		c := m.collection(collectionName)

		c.Range(func(key, value interface{}) bool {
			item := make(map[string]interface{})

			// TODO: JSON-path style field selection for nested objects
			for _, field := range fields {
				item[field] = value.(map[string]interface{})[field]
			}

			items = append(items, item)
			return true
		})

		return []interface{}{items}
	}))

	item := r.With(huma.PathParam(cfg.single+"Id", "Item ID"))
	rs.RemoveProperty("id")
	ws.RemoveProperty("id")

	item.With(
		huma.HeaderParam("if-none-match", "ETag-based conditional get", ""),
		huma.HeaderParam("if-modified-since", "Only return if modified", time.Time{}),
		huma.ResponseHeader("last-modified", "Last modified date/time"),
		huma.ResponseHeader("etag", "Etag content hash"),
		huma.ResponseError(http.StatusNotFound, "Not found"),
		huma.Response(http.StatusNotModified, "Not modified", huma.Headers("last-modified", "etag")),
		huma.ResponseJSON(http.StatusOK, "Success", huma.Schema(*rs), huma.Headers("last-modified", "etag")),
	).Get("Get a "+cfg.single, huma.UnsafeHandler(func(inputs ...interface{}) []interface{} {
		i := len(r.PathParams())
		id := inputs[i].(string)
		ifNoneMatch := inputs[i+1].(string)
		ifModified := inputs[i+2].(time.Time)

		var lastModified string
		var etag string
		var error404 *huma.ErrorModel
		var notModified bool
		var item interface{}

		c := m.collection(collectionName)

		// TODO: compute composite ID from any previous path params.
		loaded, ok := c.Load(id)
		if !ok {
			error404 = &huma.ErrorModel{
				Message: cfg.single + " not found with ID: " + id,
			}
			return []interface{}{lastModified, etag, error404, notModified, item}
		}

		// Copy the data to return (except id/headers)
		itemMap := loaded.(map[string]interface{})

		// Set headers
		lastModified = itemMap["modified"].(time.Time).Format(http.TimeFormat)
		etag = `W/"` + itemMap["etag"].(string) + `"`

		if err := checkConditionals(itemMap, "", ifNoneMatch, time.Time{}, ifModified); err != nil {
			notModified = true
			return []interface{}{lastModified, etag, error404, notModified, item}
		}

		// Create the response structure.
		ret := make(map[string]interface{})
		for k, v := range itemMap {
			if k == "id" || k == "modified" || k == "etag" {
				continue
			}
			ret[k] = v
		}
		item = ret

		return []interface{}{lastModified, etag, error404, notModified, item}
	}))

	item.With(
		huma.HeaderParam("if-match", "Etag-based conditional update", ""),
		huma.HeaderParam("if-none-match", "Etag-based conditional update. Use `*` to mean any possible value.", ""),
		huma.HeaderParam("if-unmodified-since", "Date-based conditional update", time.Time{}),
		huma.RequestSchema(ws),
		huma.ResponseHeader("last-modified", "Last modified date/time"),
		huma.ResponseHeader("etag", "Etag content hash"),
		huma.ResponseJSON(http.StatusPreconditionFailed, "Conditional update failed"),
		huma.Response(http.StatusNoContent, "Success", huma.Headers("last-modified", "etag")),
	).Put("Create or update "+cfg.single, huma.UnsafeHandler(func(inputs ...interface{}) []interface{} {
		i := len(r.PathParams())
		id := inputs[i].(string)
		ifMatch := inputs[i+1].(string)
		ifNoneMatch := inputs[i+2].(string)
		ifUnmodified := inputs[i+3].(time.Time)
		item := inputs[i+4].(map[string]interface{})

		var lastModified string
		var etag string
		var errorPrecondition *huma.ErrorModel
		var success bool

		if _, ok := dataStructure.(createHook); ok {
			t := reflect.TypeOf(dataStructure)
			if t.Kind() == reflect.Ptr {
				t = t.Elem()
			}
			tmp := reflect.New(t).Interface()
			mapstructure.Decode(item, &tmp)
			tmp.(createHook).OnCreate()
			fillMap(tmp, item)
		}

		c := m.collection(collectionName)

		if ifMatch != "" || ifNoneMatch != "" || !ifUnmodified.IsZero() {
			m.mu.Lock()
			defer m.mu.Unlock()
			var existing map[string]interface{}
			if e, ok := c.Load(id); ok {
				existing = e.(map[string]interface{})
			}
			if err := checkConditionals(existing, ifMatch, ifNoneMatch, ifUnmodified, time.Time{}); err != nil {
				errorPrecondition = &huma.ErrorModel{
					Message: err.Error(),
				}
				return []interface{}{lastModified, etag, errorPrecondition, success}
			}
		}

		// Set the ID from the path parameter
		item["id"] = id

		// Generate an etag hash.
		encoded, _ := json.Marshal(item)
		sum := sha1.Sum(encoded)
		newETag := base64.RawURLEncoding.EncodeToString(sum[:])

		// Update modified time & etag hash
		item["modified"] = time.Now().UTC()
		item["etag"] = newETag

		c.Store(id, item)

		lastModified = item["modified"].(time.Time).Format(http.TimeFormat)
		etag = `W/"` + newETag + `"`
		success = true
		return []interface{}{lastModified, etag, errorPrecondition, success}
	}))

	// TODO: patch

	item.With(
		huma.HeaderParam("if-match", "Etag-based conditional update header", ""),
		huma.HeaderParam("if-unmodified-since", "Date-based conditional update", time.Time{}),
		huma.ResponseError(http.StatusNotFound, cfg.single+" not found"),
		huma.ResponseError(http.StatusPreconditionFailed, "Conditional delete failed"),
		huma.Response(http.StatusNoContent, "Success"),
	).Delete("Delete a "+cfg.single, huma.UnsafeHandler(func(inputs ...interface{}) []interface{} {
		i := len(r.PathParams())
		id := inputs[i].(string)
		ifMatch := inputs[i+1].(string)
		ifUnmodified := inputs[i+2].(time.Time)

		var error404 *huma.ErrorModel
		var errorEtag *huma.ErrorModel
		var success bool

		c := m.collection(collectionName)

		m.mu.Lock()
		defer m.mu.Unlock()
		item, ok := c.Load(id)

		if !ok {
			error404 = &huma.ErrorModel{
				Message: fmt.Sprintf("%s not found: %s", cfg.single, id),
			}
			return []interface{}{error404, errorEtag, success}
		}

		if err := checkConditionals(item.(map[string]interface{}), ifMatch, "", ifUnmodified, time.Time{}); err != nil {
			errorEtag = &huma.ErrorModel{
				Message: err.Error(),
			}
			return []interface{}{error404, errorEtag, success}
		}

		c.Delete(id)
		success = true
		return []interface{}{error404, errorEtag, success}
	}))

	return item
}
