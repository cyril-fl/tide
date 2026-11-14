package search

import (
	"net/http"

	"github.com/cyril-fl/notes/notes-go/app/api/services/respond"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/searchnotes"
	"github.com/uptrace/bun"
)

// Handler finds notes by title or content: GET /api/search?q=&limit=&offset=, X-Total-Count counting the matches.
// @Summary  Find notes by title or content
// @Tags     search
// @Produce  json
// @Param    q      query string  true  "text to find"
// @Param    limit  query integer false "matches in the page, all when left out or 0"
// @Param    offset query integer false "matches skipped before the page"
// @Success  200 {array} searchnotes.Match
// @Header   200 {integer} X-Total-Count "the matches, not only the page: those read, and the notes after them that may match"
// @Failure  400 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/search [get]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset, ok := respond.Paging(w, r)
		if !ok {
			return
		}
		process := searchnotes.Process(r.Context(), db, r.URL.Query().Get("q")).Page(limit, offset)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.Total(w, process.GetTotal())
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
