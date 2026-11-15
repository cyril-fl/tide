// Package readsearch is the API route running a saved search.
package readsearch

import (
	"net/http"

	"github.com/cyril-fl/notes/notes-go/app/api/services/respond"
	"github.com/cyril-fl/notes/notes-go/app/shared/domain"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/edituser"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/readsavedsearch"
	"github.com/cyril-fl/notes/notes-go/app/shared/services/process/runsavedsearch"
	"github.com/uptrace/bun"
)

// Handler runs a saved search and lists the notes it matches: GET /api/searches/{id}?limit=&offset=, X-Total-Count
// counting the matches.
// @Summary  Run a saved search
// @Tags     searches
// @Produce  json
// @Param    id     path  string  true  "saved search id"
// @Param    limit  query integer false "matches in the page, all when left out or 0"
// @Param    offset query integer false "matches skipped before the page"
// @Success  200 {array} runsavedsearch.Match
// @Header   200 {integer} X-Total-Count "the matches, not only the page: those read, and the notes after them that may match"
// @Failure  404 {object} respond.Failure
// @Failure  403 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/searches/{id} [get]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset, ok := respond.Paging(w, r)
		if !ok {
			return
		}
		process := readsavedsearch.Process(r.Context(), db, r.PathValue("id")).Page(limit, offset)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.Total(w, process.GetTotal())
		respond.List(w, http.StatusOK, process.GetResult())
	}
}

// Preview runs criteria not saved yet, for the smart folder dialog to show what they match: POST /api/searches?limit=,
// X-Total-Count counting the matches. The name and id of the body are ignored.
// @Summary  Run criteria not saved yet
// @Tags     searches
// @Accept   json
// @Produce  json
// @Param    body   body  domain.SavedSearch true  "criteria"
// @Param    limit  query integer false "matches in the page, all when left out or 0"
// @Param    offset query integer false "matches skipped before the page"
// @Success  200 {array} runsavedsearch.Match
// @Header   200 {integer} X-Total-Count "the matches, not only the page: those read, and the notes after them that may match"
// @Failure  400 {object} respond.Failure
// @Failure  403 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/searches [post]
func Preview(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit, offset, ok := respond.Paging(w, r)
		if !ok {
			return
		}
		var search domain.SavedSearch
		if !respond.Decode(w, r, &search) {
			return
		}
		if len(search.TagIDs) > edituser.MaxSavedSearchIDs || len(search.FolderIDs) > edituser.MaxSavedSearchIDs {
			respond.Error(w, edituser.ErrInvalidSavedSearch)
			return
		}

		process := runsavedsearch.Process(r.Context(), db, search).Page(limit, offset)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.Total(w, process.GetTotal())
		respond.List(w, http.StatusOK, process.GetResult())
	}
}
