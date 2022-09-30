package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma"
	"github.com/danielgtaylor/huma/cli"
	"github.com/danielgtaylor/huma/middleware"
	"github.com/danielgtaylor/huma/responses"
)

// GenreSummary is used to list genres. It does not include the (potentially)
// large genre content.
type GenreSummary struct {
	ID          string    `json:"id" doc:"Genre ID"`
	Description string    `json:"description" doc:"Description"`
	Created     time.Time `json:"created" doc:"Created date/time as ISO8601"`
}

type GenrePutRequest struct {
	Description string `json:"description" doc:"Description"`
}

// GenreIDParam gets the genre ID from the URI path.
type GenreIDParam struct {
	GenreID string `path:"genre-id" pattern:"^[a-zA-Z0-9._-]{1,32}$"`
}

// Genre records some content text for later reference.
type Genre struct {
	ID          string    `json:"id" doc:"Genre ID"`
	Books       []Book    `json:"books" doc:"Books"`
	Description string    `json:"description" doc:"Description"`
	Created     time.Time `json:"created" readOnly:"true" doc:"Created date/time as ISO8601"`
}

type Book struct {
	ID        string    `json:"id" doc:"Book ID"`
	Title     string    `json:"title" doc:"Title"`
	Author    string    `json:"author" doc:"Author"`
	Published time.Time `json:"published" doc:"Created date/time as ISO8601"`
}

type BookPutRequest struct {
	Title     string    `json:"title" doc:"Title"`
	Author    string    `json:"author" doc:"Author"`
	Published time.Time `json:"published" doc:"Created date/time as ISO8601"`
}

type BookIDParam struct {
	BookID string `path:"book-id" pattern:"^[a-zA-Z0-9._-]{1,32}$"`
}

// We'll use an in-memory DB (a goroutine-safe map). Don't do this in
// production code!
var memoryDB = sync.Map{}

func main() {
	// Create a new router and give our API a title and version.
	app := cli.NewRouter("BookStore API", "1.0.0")
	app.ServerLink("Development server", "http://localhost:8888")

	genres := app.Resource("/v1/genres")
	genres.Get("list-genres", "Returns a list of all genres",
		responses.OK().Model([]*GenreSummary{}),
	).Run(func(ctx huma.Context) {
		// Create a list of summaries from all the genres.
		summaries := make([]*GenreSummary, 0)

		memoryDB.Range(func(k, v interface{}) bool {
			summaries = append(summaries, &GenreSummary{
				ID:          k.(string),
				Description: v.(Genre).Description,
				Created:     v.(Genre).Created,
			})
			return true
		})

		ctx.WriteModel(http.StatusOK, summaries)
	})

	// Add an `id` path parameter to create a genre resource.
	genre := genres.SubResource("/{genre-id}")

	genre.Put("put-genre", "Create or update a genre",
		responses.NoContent(),
	).Run(func(ctx huma.Context, input struct {
		GenreIDParam
		Body GenrePutRequest
	}) {
		middleware.GetLogger(ctx).Info("Creating a new genre")

		// Set the created time to now and then save the genre in the DB.
		new := Genre{
			ID:          input.GenreID,
			Description: input.Body.Description,
			Created:     time.Now(),
			Books:       []Book{},
		}
		memoryDB.Store(input.GenreID, new)
	})

	genre.Get("get-genre", "Get a genre by its ID",
		responses.OK().Model(Genre{}),
		responses.NotFound(),
	).Run(func(ctx huma.Context, input GenreIDParam) {
		if g, ok := memoryDB.Load(input.GenreID); ok {
			// Genre with that ID exists!
			ctx.WriteModel(http.StatusOK, g.(Genre))
			return
		}

		ctx.WriteError(http.StatusNotFound, "Genre "+input.GenreID+" not found")
	})

	genre.Delete("delete-genre", "Delete a genre by its ID",
		responses.NoContent(),
		responses.NotFound(),
	).Run(func(ctx huma.Context, input GenreIDParam) {
		if _, ok := memoryDB.Load(input.GenreID); ok {
			// Genre with that ID exists!
			memoryDB.Delete(input.GenreID)
			ctx.WriteHeader(http.StatusNoContent)
			return
		}

		ctx.WriteError(http.StatusNotFound, "Genre "+input.GenreID+" not found")
	})

	books := genre.SubResource("/books")
	books.Tags("Books by Genre")

	books.Get("list-books", "Returns a list of all books for a genre",
		[]huma.Response{
			responses.OK().Model([]Book{}),
			responses.NotFound(),
		}...,
	).Run(func(ctx huma.Context, input struct {
		GenreIDParam
	}) {

		if g, ok := memoryDB.Load(input.GenreID); ok {
			ctx.WriteModel(http.StatusOK, g.(Genre).Books)
			return
		}

		ctx.WriteError(http.StatusNotFound, "Genre "+input.GenreID+" not found")
	})

	book := books.SubResource("/{book-id}")
	book.Put("put-book", "Create or update a book",
		responses.NoContent(),
	).Run(func(ctx huma.Context, input struct {
		GenreIDParam
		BookIDParam
		Body BookPutRequest
	}) {
		middleware.GetLogger(ctx).Info("Creating a new book")

		if g, ok := memoryDB.Load(input.GenreID); !ok {
			// Genre with that ID doesn't exists!
			ctx.WriteError(http.StatusNotFound, "Genre "+input.GenreID+" not found")
			return
		} else {
			genre := g.(Genre)
			genre.Books = append(genre.Books, Book{
				Title:     input.Body.Title,
				Author:    input.Body.Author,
				ID:        input.BookID,
				Published: input.Body.Published,
			})

			memoryDB.Store(input.GenreID, genre)
		}

	})

	book.Get("get-book", "Get a book by its ID",
		responses.OK().Model(Book{}),
		responses.NotFound(),
	).Run(func(ctx huma.Context, input struct {
		GenreIDParam
		BookIDParam
	}) {
		if g, ok := memoryDB.Load(input.GenreID); !ok {
			// Genre with that ID exists!
			ctx.WriteError(http.StatusNotFound, "Genre "+input.GenreID+" not found")
			return
		} else {
			for _, book := range g.(Genre).Books {
				if book.ID == input.BookID {
					ctx.WriteModel(http.StatusOK, book)
					return
				}
			}
		}

		ctx.WriteError(http.StatusNotFound, "Book "+input.BookID+" not found")
	})

	book.Delete("delete-book", "Delete a book by its ID",
		responses.NoContent(),
		responses.NotFound(),
	).Run(func(ctx huma.Context, input struct {
		GenreIDParam
		BookIDParam
	}) {
		if g, ok := memoryDB.Load(input.GenreID); !ok {
			// Genre with that ID exists!
			ctx.WriteError(http.StatusNotFound, "Genre "+input.GenreID+" not found")
			return
		} else {
			for _, book := range g.(Genre).Books {
				if book.ID == input.BookID {
					ctx.WriteHeader(http.StatusNoContent)
					return
				}
			}
		}

		ctx.WriteError(http.StatusNotFound, "Book "+input.BookID+" not found")
	})

	// Run the app!
	app.Run()
}
