package editnotetext

import (
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/editnotetext"
	"github.com/uptrace/bun"
)

// Handler changes a part of a note's text, its title, a text or a section, through the path of a full edit:
// PATCH /api/notes/{id}/text. It answers the note as GET does and how many times the text was replaced.
// @Summary  Rename a note, or replace a text or a section of it
// @Tags     notes
// @Accept   json
// @Produce  json
// @Param    id path string true "uuid"
// @Param    body body editnotetext.Input true "body"
// @Success  200 {object} editnotetext.Result
// @Failure  400 {object} respond.Failure
// @Failure  404 {object} respond.Failure
// @Failure  409 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/{id}/text [patch]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}

		var input editnotetext.Input
		if !respond.Decode(w, r, &input) {
			return
		}
		input.ID = id

		process := editnotetext.Process(r.Context(), db, input)
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		result := process.GetResult()
		result.Note.Tags = append([]string{}, result.Note.Tags...)
		respond.JSON(w, http.StatusOK, result)
	}
}
