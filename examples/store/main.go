package main

import (
	"time"

	"github.com/istreamlabs/huma"
	"github.com/istreamlabs/huma/memstore"
)

// Note represents a sticky note.
type Note struct {
	ID      string    `json:"id" doc:"Note ID"`
	Created time.Time `json:"created" readOnly:"true" doc:"Created date/time as ISO8601"`
	Content string    `json:"content" doc:"Note contents as Markdown"`
}

// OnCreate is called before a new Note gets created and stored.
func (n *Note) OnCreate() {
	n.Created = time.Now().UTC()
}

func main() {
	r := huma.NewRouter("Adapter API", "1.0.0")

	store := memstore.New()
	store.AutoResource(r.Resource("/notes"), &Note{})

	r.Run()
}
