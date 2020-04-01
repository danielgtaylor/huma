package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma"
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

// We'll use an in-memory DB (a map) and protect it with a lock. Don't do
// this in production code!
var memoryDB = make(map[string]*Note, 0)
var dbLock = sync.Mutex{}

func main() {
	// Create a new router and give our API a title and version.
	r := huma.NewRouter(&huma.OpenAPI{
		Title:   "Notes API",
		Version: "1.0.0",
	})

	notes := r.Resource("/notes")
	notes.ListJSON(http.StatusOK, "Returns a list of all notes",
		func() []*NoteSummary {
			dbLock.Lock()
			defer dbLock.Unlock()

			// Create a list of summaries from all the notes.
			summaries := make([]*NoteSummary, 0, len(memoryDB))

			for k, v := range memoryDB {
				summaries = append(summaries, &NoteSummary{
					ID:      k,
					Created: v.Created,
				})
			}

			return summaries
		},
	)

	note := notes.With(huma.PathParam("id", "Note ID", &huma.Schema{
		Pattern: "^[a-zA-Z0-9._-]{1,32}$",
	}))

	notFound := huma.ResponseError(http.StatusNotFound, "Note not found")

	note.PutNoContent(http.StatusNoContent, "Create or update a note",
		func(id string, note *Note) bool {
			dbLock.Lock()
			defer dbLock.Unlock()

			// Set the created time to now and then save the note in the DB.
			note.Created = time.Now()
			memoryDB[id] = note

			// Empty responses don't have a body, so you can just return `true`.
			return true
		},
	)

	note.With(notFound).GetJSON(http.StatusOK, "Get a note by its ID",
		func(id string) (*huma.ErrorModel, *Note) {
			dbLock.Lock()
			defer dbLock.Unlock()

			if note, ok := memoryDB[id]; ok {
				// Note with that ID exists!
				return nil, note
			}

			return &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}, nil
		},
	)

	note.With(notFound).DeleteNoContent(http.StatusNoContent, "Successfully deleted note",
		func(id string) (*huma.ErrorModel, bool) {
			dbLock.Lock()
			defer dbLock.Unlock()

			if _, ok := memoryDB[id]; ok {
				// Note with that ID exists!
				delete(memoryDB, id)
				return nil, true
			}

			return &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}, false
		},
	)

	// Run the app!
	r.Run()
}
