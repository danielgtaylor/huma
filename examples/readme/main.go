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

	// Create the "list-notes" operation via `GET /notes`.
	r.Register(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/notes",
		Description: "Returns a list of all notes",
		Responses: []*huma.Response{
			huma.ResponseJSON(http.StatusOK, "Successful hello response"),
		},
		Handler: func() []*NoteSummary {
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
	})

	// idParam defines the note's ID as part of the URL path.
	idParam := huma.PathParam("id", "Note ID", &huma.Schema{
		Pattern: "^[a-zA-Z0-9._-]{1,32}$",
	})

	r.Register(&huma.Operation{
		Method:      http.MethodGet,
		Path:        "/notes/{id}",
		Description: "Get a single note by its ID",
		Params:      []*huma.Param{idParam},
		Responses: []*huma.Response{
			huma.ResponseJSON(200, "Success"),
			huma.ResponseError(404, "Note was not found"),
		},
		Handler: func(id string) (*Note, *huma.ErrorModel) {
			dbLock.Lock()
			defer dbLock.Unlock()

			if note, ok := memoryDB[id]; ok {
				// Note with that ID exists!
				return note, nil
			}

			return nil, &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}
		},
	})

	r.Register(&huma.Operation{
		Method:      http.MethodPut,
		Path:        "/notes/{id}",
		Description: "Creates or updates a note",
		Params:      []*huma.Param{idParam},
		Responses: []*huma.Response{
			huma.ResponseEmpty(204, "Successfully created or updated the note"),
		},
		Handler: func(id string, note *Note) bool {
			dbLock.Lock()
			defer dbLock.Unlock()

			// Set the created time to now and then save the note in the DB.
			note.Created = time.Now()
			memoryDB[id] = note

			// Empty responses don't have a body, so you can just return `true`.
			return true
		},
	})

	r.Register(&huma.Operation{
		Method:      http.MethodDelete,
		Path:        "/notes/{id}",
		Description: "Deletes a note",
		Params:      []*huma.Param{idParam},
		Responses: []*huma.Response{
			huma.ResponseEmpty(204, "Successfuly deleted note"),
			huma.ResponseError(404, "Note was not found"),
		},
		Handler: func(id string) (bool, *huma.ErrorModel) {
			dbLock.Lock()
			defer dbLock.Unlock()

			if _, ok := memoryDB[id]; ok {
				// Note with that ID exists!
				delete(memoryDB, id)
				return true, nil
			}

			return false, &huma.ErrorModel{
				Message: "Note " + id + " not found",
			}
		},
	})

	// Run the app!
	r.Run()
}
