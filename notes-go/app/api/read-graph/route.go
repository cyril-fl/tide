package readgraph

import (
	"net/http"

	"github.com/cyril-fl/tide/notes-go/app/api/services/respond"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/counttags"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/layoutgraph"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readgraph"
	"github.com/cyril-fl/tide/notes-go/app/shared/services/process/readneighbors"
	"github.com/uptrace/bun"
)

// Handler gives every tag with how many notes carry it, most used first: GET /api/graph.
// @Summary  Count the notes of every tag, most used first
// @Tags     graph
// @Produce  json
// @Success  200 {array} counttags.TagCount
// @Failure  500 {object} respond.Failure
// @Router   /api/graph [get]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		process := counttags.Process(r.Context(), db)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.List(w, http.StatusOK, process.GetResult())
	}
}

// Notes gives every live note with its tags, and the [[links]] between them, laid out: GET /api/graph/notes.
// The account's layout is kept by the server, computed again only when the graph changed; layout=false skips it.
// @Summary  Read every note with its tags, and the links between them, laid out
// @Tags     graph
// @Produce  json
// @Param    layout query bool false "false: no layout, the notes without x and y, sorted by id"
// @Success  200 {object} readgraph.Result
// @Failure  500 {object} respond.Failure
// @Router   /api/graph/notes [get]
func Notes(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("layout") == "false" {
			layout(w, readgraph.Process(r.Context(), db, true))
			return
		}
		layout(w, layoutgraph.Process(r.Context(), db, ""))
	}
}

// Graph gives the graph around a note, laid out: its tags, the notes sharing one, its [[links]] either way:
// GET /api/notes/{id}/graph.
// @Summary  Read the graph around a note, laid out
// @Tags     notes
// @Produce  json
// @Param    id path string true "uuid"
// @Success  200 {object} readgraph.Result
// @Failure  404 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/{id}/graph [get]
func Graph(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}
		layout(w, layoutgraph.Process(r.Context(), db, id))
	}
}

// List gives the live notes whose title or path holds q, their id, title and path, for the [[ menu: GET /api/notes?q=.
// A few dozen at most: the whole list weighs 7 MB at 50,000 notes (#150).
// @Summary  Find the notes by title or path
// @Tags     notes
// @Produce  json
// @Param    q query string false "text of the title or the path"
// @Success  200 {array} readgraph.Note
// @Failure  500 {object} respond.Failure
// @Router   /api/notes [get]
func List(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		process := readgraph.Process(r.Context(), db, false)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.List(w, http.StatusOK, readgraph.Match(process.GetResult().Notes, r.URL.Query().Get("q"), 50))
	}
}

// Neighbors gives a note and the notes sharing its tags: GET /api/notes/{id}/neighbors.
// @Summary  Read a note and the notes sharing its tags
// @Tags     notes
// @Produce  json
// @Param    id path string true "uuid"
// @Success  200 {object} readneighbors.Result
// @Failure  404 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/{id}/neighbors [get]
func Neighbors(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}

		process := readneighbors.Process(r.Context(), db, id)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		result := process.GetResult()
		result.Note.Tags = append([]string{}, result.Note.Tags...)
		result.Neighbors = append([]readneighbors.Neighbor{}, result.Neighbors...)
		respond.JSON(w, http.StatusOK, result)
	}
}

func layout(w http.ResponseWriter, process interface {
	Run()
	HasErr() bool
	GetErr() error
	GetResult() readgraph.Result
}) {
	process.Run()
	if process.HasErr() {
		respond.Error(w, process.GetErr())
		return
	}
	result := process.GetResult()
	result.Notes = append([]readgraph.Note{}, result.Notes...)
	result.Links = append([]readgraph.Link{}, result.Links...)
	respond.JSON(w, http.StatusOK, result)
}
