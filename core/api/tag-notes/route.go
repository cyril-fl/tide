package tagnotes

import (
	"net/http"
	"time"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/tagnotes"
	"github.com/uptrace/bun"
)

// Handler adds and removes #tags on several notes in one transaction, the rest of their text kept, a note missing
// or locked answered with its error and the others tagged: PATCH /api/notes/tags.
// @Summary  Tag several notes
// @Tags     notes
// @Accept   json
// @Produce  json
// @Param    body body tagnotes.Input true "body"
// @Success  200 {array} tagnotes.Tagged "in the order asked"
// @Failure  400 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/tags [patch]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input tagnotes.Input
		if !respond.Decode(w, r, &input) {
			return
		}

		// Hundreds of notes go through patchnote one by one: past the 30 s the server gives a request.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		process := tagnotes.Process(r.Context(), db, input)
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
