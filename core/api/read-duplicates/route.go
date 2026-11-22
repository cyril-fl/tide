package readduplicates

import (
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/listduplicates"
	"github.com/uptrace/bun"
)

// Handler lists the groups of live notes with the same title, or with the same path when same_path=true: GET /api/notes/duplicates.
// @Summary  List the notes with the same title or the same path
// @Tags     search
// @Produce  json
// @Param    same_path query bool false "only the notes of the same folder, which a path cannot tell apart"
// @Success  200 {array} listduplicates.Group
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/duplicates [get]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		process := listduplicates.Process(r.Context(), db, r.URL.Query().Get("same_path") == "true")
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
