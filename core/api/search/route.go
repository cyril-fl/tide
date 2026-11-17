package search

import (
	"net/http"
	"strconv"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/searchnotes"
	"github.com/uptrace/bun"
)

// Handler finds notes by title or content: GET /api/search?q=&max_length=&untagged=&limit=&offset=, X-Total-Count
// counting the matches.
// @Summary  Find notes by title or content
// @Description The text in the title first, then in a heading, then the notes with the most backlinks, the latest edited, by path.
// @Tags     search
// @Produce  json
// @Param    q          query string  false "text to find, may be left out with max_length or untagged"
// @Param    max_length query integer false "only the notes whose content (title line included) has at most that many characters"
// @Param    untagged   query boolean false "only the notes with no tag"
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
		query := r.URL.Query()
		filter := searchnotes.Filter{Untagged: query.Get("untagged") == "true"}
		if value := query.Get("max_length"); value != "" {
			var err error
			if filter.MaxLength, err = strconv.Atoi(value); err != nil || filter.MaxLength < 0 {
				respond.JSON(w, http.StatusBadRequest, respond.Failure{Error: "invalid max_length"})
				return
			}
		}
		process := searchnotes.Process(r.Context(), db, query.Get("q")).Filter(filter).Page(limit, offset)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.Total(w, process.GetTotal())
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
