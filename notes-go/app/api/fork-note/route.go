package forknote

import (
	"net/http"
	"time"

	"github.com/cyril-fl/notes/notes-go/app/api/services/respond"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/forknote"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/mergefork"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readfork"
	"github.com/uptrace/bun"
)

// Body is the text of the fork.
type Body struct {
	Content string `json:"content"`
}

// Handler creates a fork of a note beside it: POST /api/notes/{id}/forks.
// @Summary  Fork a note
// @Tags     notes
// @Accept   json
// @Produce  json
// @Param    id path string true "uuid of the note forked"
// @Param    body body Body true "body"
// @Success  201 {object} domain.Item
// @Failure  400 {object} respond.Failure
// @Failure  403 {object} respond.Failure
// @Failure  404 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/{id}/forks [post]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}
		var body Body
		if !respond.Decode(w, r, &body) {
			return
		}

		process := forknote.Process(r.Context(), db, forknote.Input{ID: id, Content: body.Content})
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.JSON(w, http.StatusCreated, process.GetResult())
	}
}

// Read answers what a merge of the fork starts from: GET /api/notes/{id}/fork, 404 for a note that is no fork.
// @Summary  Read the origin of a fork
// @Tags     notes
// @Produce  json
// @Param    id path string true "uuid of the fork"
// @Success  200 {object} readfork.Result
// @Failure  403 {object} respond.Failure
// @Failure  404 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/{id}/fork [get]
func Read(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}

		process := readfork.Process(r.Context(), db, id)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.JSON(w, http.StatusOK, process.GetResult())
	}
}

// MergeBody is a merge made by the app.
type MergeBody struct {
	// Content is the merged text, saved into the original.
	Content string `json:"content"`
	// Base is the fork's text the merge started from, the base of the next merge.
	Base string `json:"base"`
	// UpdatedAt is updated_at of the original as read, or its created_at: a 409 if it changed since.
	UpdatedAt *time.Time `json:"updated_at" binding:"optional"`
}

// Merge saves a merge of the fork into its original: POST /api/notes/{id}/merge. The text replaced is kept in the
// original's history. 409 when the original is locked, changed since, or in the trash.
// @Summary  Merge a fork into its original
// @Tags     notes
// @Accept   json
// @Produce  json
// @Param    id path string true "uuid of the fork"
// @Param    body body MergeBody true "body"
// @Success  200 {object} readitem.Result
// @Failure  400 {object} respond.Failure
// @Failure  403 {object} respond.Failure
// @Failure  404 {object} respond.Failure
// @Failure  409 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/{id}/merge [post]
func Merge(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}
		var body MergeBody
		if !respond.Decode(w, r, &body) {
			return
		}

		process := mergefork.Process(r.Context(), db, mergefork.Input{ID: id, Content: body.Content, Base: body.Base, UpdatedAt: body.UpdatedAt})
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		note := process.GetResult()
		note.Tags = append([]string{}, note.Tags...)
		respond.JSON(w, http.StatusOK, note)
	}
}
