package deleteitems

import (
	"net/http"

	"github.com/cyril-fl/notes/notes-go/app/api/services/respond"
	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/deleteitems"
	"github.com/uptrace/bun"
)

type Body struct {
	IDs []string `json:"ids"`
}

// Handler moves the notes or the folders of the body to the trash in one transaction, a folder with its content, and
// answers every item moved; unknown ids are skipped: DELETE /api/notes, DELETE /api/folders.
// @Summary  Move notes or folders to the trash, a folder with its content
// @Tags     items
// @Accept   json
// @Produce  json
// @Param    body body Body true "body"
// @Success  200 {array} domain.Item
// @Failure  400 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes [delete]
// @Router   /api/folders [delete]
func Handler(db bun.IDB, kind domain.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input Body
		if !respond.Decode(w, r, &input) || !respond.ValidIDs(w, input.IDs...) {
			return
		}

		process := deleteitems.Process(r.Context(), db, kind, input.IDs)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
