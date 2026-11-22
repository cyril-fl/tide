package readpath

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/process/resolveitem"
	"github.com/uptrace/bun"
)

// Resolved is the item at a path.
type Resolved struct {
	ID string `json:"id"`
}

// Handler finds the item at a path, for the CLI that takes paths where the API takes ids: GET /api/resolve?path=&kind=.
// @Summary  Find the item at a path
// @Tags     items
// @Produce  json
// @Param    path query string true  "path from the root, /Projects/Ideas"
// @Param    kind query string false "folder or note, both when left out"
// @Success  200 {object} Resolved
// @Failure  404 {object} respond.Failure
// @Failure  409 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/resolve [get]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		// resolveitem hands back a ref without / as a uuid: here only paths make sense.
		if !strings.HasPrefix(path, "/") {
			respond.Error(w, r, fmt.Errorf("%w: %q", resolveitem.ErrNotFound, path))
			return
		}
		process := resolveitem.Process(r.Context(), db, domain.Kind(r.URL.Query().Get("kind")), []string{path})
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		respond.JSON(w, http.StatusOK, Resolved{ID: process.GetResult()[0]})
	}
}
