package readnotes

import (
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/readnotes"
	"github.com/uptrace/bun"
)

type Body struct {
	Notes    []string `json:"notes"`                                  // uuids, or paths from the root (/Folder/Note); 50 at most
	MaxChars int      `json:"max_chars,omitempty" binding:"optional"` // only the start of each content, with its length and truncated
}

// Handler reads several notes in one request, for an agent reading a batch over a slow network: POST /api/notes/read,
// a body because 50 paths may not fit a URL.
// @Summary  Read several notes
// @Tags     notes
// @Accept   json
// @Produce  json
// @Param    body body Body true "body"
// @Success  200 {array} readnotes.Note "in the order asked"
// @Failure  400 {object} respond.Failure
// @Failure  404 {object} respond.Failure "a note missing, named in the error"
// @Failure  409 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/read [post]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input Body
		if !respond.Decode(w, r, &input) {
			return
		}

		process := readnotes.Process(r.Context(), db, input.Notes, input.MaxChars)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
