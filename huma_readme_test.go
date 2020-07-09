package huma_test

import (
	"net/http"
	"sync"
	"time"

	"github.com/istreamlabs/huma"
	"github.com/istreamlabs/huma/schema"
)

// NoteSummary is used to list notes. It does not include the (potentially)
// large note content.
type NoteSummary struct {
	ID      string
	Created time.Time
}

// Note records some content text for later reference.
type Note struct {
	Created time.Time `readOnly:"true"`
	Content string
}

// We'll use an in-memory DB (a goroutine-safe map). Don't do this in
// production code!
var memoryDB = sync.Map{}

func Example_readme() {
	// Create a new router and give our API a title and version.
	r := huma.NewRouter("Notes API", "1.0.0")

	notes := r.Resource("/notes")
	notes.List("Returns a list of all notes", func() []*NoteSummary {
		// Create a list of summaries from all the notes.
		summaries := make([]*NoteSummary, 0)

		memoryDB.Range(func(k, v interface{}) bool {
			summaries = append(summaries, &NoteSummary{
				ID:      k.(string),
				Created: v.(*Note).Created,
			})
			return true
		})

		return summaries
	})

	// Set up a custom schema to limit identifier values.
	idSchema := schema.Schema{Pattern: "^[a-zA-Z0-9._-]{1,32}$"}

	// Add an `id` path parameter to create a note resource.
	note := notes.With(huma.PathParam("id", "Note ID", huma.Schema(idSchema)))

	notFound := huma.ResponseError(http.StatusNotFound, "Note not found")

	note.Put("Create or update a note", func(id string, n *Note) bool {
		// Set the created time to now and then save the note in the DB.
		n.Created = time.Now()
		memoryDB.Store(id, n)

		// Empty responses don't have a body, so you can just return `true`.
		return true
	})

	note.With(notFound).Get("Get a note by its ID",
		func(id string) (*huma.ErrorModel, *Note) {
			if n, ok := memoryDB.Load(id); ok {
				// Note with that ID exists!
				return nil, n.(*Note)
			}

			return &huma.ErrorModel{
				Detail: "Note " + id + " not found",
			}, nil
		},
	)

	note.With(notFound).Delete("Delete a note by its ID",
		func(id string) (*huma.ErrorModel, bool) {
			if _, ok := memoryDB.Load(id); ok {
				// Note with that ID exists!
				memoryDB.Delete(id)
				return nil, true
			}

			return &huma.ErrorModel{
				Detail: "Note " + id + " not found",
			}, false
		},
	)

	// Run the app!
	r.Run()
}
