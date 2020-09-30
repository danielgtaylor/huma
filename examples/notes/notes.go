package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/istreamlabs/huma"
	"github.com/istreamlabs/huma/cli"
	"github.com/istreamlabs/huma/middleware"
	"github.com/istreamlabs/huma/responses"
)

// NoteSummary is used to list notes. It does not include the (potentially)
// large note content.
type NoteSummary struct {
	ID      string    `json:"id" doc:"Note ID"`
	Created time.Time `json:"created" doc:"Created date/time as ISO8601"`
}

// NoteIDParam gets the note ID from the URI path.
type NoteIDParam struct {
	NoteID string `path:"note-id" pattern:"^[a-zA-Z0-9._-]{1,32}$"`
}

// Note records some content text for later reference.
type Note struct {
	Created time.Time `json:"created" readOnly:"true" doc:"Created date/time as ISO8601"`
	Content string    `json:"content" doc:"Note content"`
}

// We'll use an in-memory DB (a goroutine-safe map). Don't do this in
// production code!
var memoryDB = sync.Map{}

func main() {
	// Create a new router and give our API a title and version.
	app := cli.NewRouter("Notes API", "1.0.0")
	app.ServerLink("Development server", "http://localhost:8888")

	notes := app.Resource("/v1/notes")
	notes.Get("list-notes", "Returns a list of all notes",
		responses.OK().Model([]*NoteSummary{}),
	).Run(func(ctx huma.Context) {
		// Create a list of summaries from all the notes.
		summaries := make([]*NoteSummary, 0)

		memoryDB.Range(func(k, v interface{}) bool {
			summaries = append(summaries, &NoteSummary{
				ID:      k.(string),
				Created: v.(Note).Created,
			})
			return true
		})

		ctx.WriteModel(http.StatusOK, summaries)
	})

	// Add an `id` path parameter to create a note resource.
	note := notes.SubResource("/{note-id}")

	note.Put("put-note", "Create or update a note",
		responses.NoContent(),
	).Run(func(ctx huma.Context, input struct {
		NoteIDParam
		Body Note
	}) {
		// Set the created time to now and then save the note in the DB.
		input.Body.Created = time.Now()
		middleware.GetLogger(ctx).Info("Creating a new note")
		memoryDB.Store(input.NoteID, input.Body)
	})

	note.Get("get-note", "Get a note by its ID",
		responses.OK().Model(Note{}),
		responses.NotFound(),
	).Run(func(ctx huma.Context, input NoteIDParam) {
		if n, ok := memoryDB.Load(input.NoteID); ok {
			// Note with that ID exists!
			ctx.WriteModel(http.StatusOK, n.(Note))
			return
		}

		ctx.WriteError(http.StatusNotFound, "Note "+input.NoteID+" not found")
	})

	note.Delete("delete-note", "Delete a note by its ID",
		responses.NoContent(),
		responses.NotFound(),
	).Run(func(ctx huma.Context, input NoteIDParam) {
		if _, ok := memoryDB.Load(input.NoteID); ok {
			// Note with that ID exists!
			memoryDB.Delete(input.NoteID)
			ctx.WriteHeader(http.StatusNoContent)
			return
		}

		ctx.WriteError(http.StatusNotFound, "Note "+input.NoteID+" not found")
	})

	// Run the app!
	app.Run()
}
