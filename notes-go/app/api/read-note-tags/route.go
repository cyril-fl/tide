package readnotetags

import (
	"net/http"

	"github.com/cyril-fl/notes/notes-go/app/api/services/respond"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/listnotetags"
	"github.com/uptrace/bun"
)

type Body struct {
	IDs []string `json:"ids"`
}

// Handler gives the tags of each note of ids, for the CLI filtering a page by tag: POST /api/notes/tags, a body
// because a folder's children are too many uuids for a URL.
// @Summary  Read the tags of notes
// @Tags     notes
// @Accept   json
// @Produce  json
// @Param    body body Body true "body"
// @Success  200 {object} map[string][]string "the tags of each note by id, sorted; a note without tags is missing"
// @Failure  400 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/tags [post]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input Body
		if !respond.Decode(w, r, &input) || !respond.ValidIDs(w, input.IDs...) {
			return
		}

		process := listnotetags.Process(r.Context(), db, input.IDs)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.JSON(w, http.StatusOK, process.GetResult())
	}
}
