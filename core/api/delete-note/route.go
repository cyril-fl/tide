package deletenote

import (
	"fmt"
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/deleteitems"
	"github.com/cyril-fl/tide/core/services/process/readitem"
	"github.com/uptrace/bun"
)

// Handler moves the note to the trash and answers what it moved: DELETE /api/notes/{id}.
// @Summary  Move a note to the trash
// @Tags     notes
// @Produce  json
// @Param    id path string true "uuid"
// @Success  200 {array} domain.Item
// @Failure  404 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/notes/{id} [delete]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := respond.ID(w, r)
		if !ok {
			return
		}

		process := deleteitems.Process(r.Context(), db, domain.NOTE, []string{id})
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		// deleteitems skips what it does not find: for one item, that is a 404.
		if len(process.GetResult()) == 0 {
			respond.Error(w, fmt.Errorf("%w: %q", readitem.ErrNotFound, id))
			return
		}
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
