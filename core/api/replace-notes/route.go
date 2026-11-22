package replacenotes

import (
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/replacenotes"
	"github.com/uptrace/bun"
)

// Handler replaces a plain text in every live note in one transaction, the locked ones left as they are, and
// answers the notes and occurrences replaced; dry_run only counts and lists them, notes and occurrence narrow it to
// a note or one of its occurrences: POST /api/replace.
// @Summary  Replace a text in every note, or count it with dry_run
// @Tags     notes
// @Accept   json
// @Produce  json
// @Param    body body replacenotes.Input true "body"
// @Success  200 {object} replacenotes.Result
// @Failure  400 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  403 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/replace [post]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input replacenotes.Input
		if !respond.Decode(w, r, &input) || !respond.ValidIDs(w, append([]string{input.Folder}, input.Notes...)...) {
			return
		}

		process := replacenotes.Process(r.Context(), db, input)
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		respond.JSON(w, http.StatusOK, process.GetResult())
	}
}
