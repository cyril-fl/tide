package deletefolder

import (
	"fmt"
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/uptrace/bun"
)

// Handler moves the folder and its content to the trash and answers every item moved: DELETE /api/folders/{id}.
// @Summary  Move a folder and its content to the trash
// @Tags     folders
// @Produce  json
// @Param    id path string true "uuid"
// @Success  200 {array} domain.Item
// @Failure  404 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/folders/{id} [delete]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}

		process := deleteitems.Process(r.Context(), db, domain.FOLDER, []string{id})
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		// deleteitems skips what it does not find: for one item, that is a 404.
		if len(process.GetResult()) == 0 {
			respond.Error(w, r, fmt.Errorf("%w: %q", readitem.ErrNotFound, id))
			return
		}
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
