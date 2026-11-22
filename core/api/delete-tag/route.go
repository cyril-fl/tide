package deletetag

import (
	"fmt"
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/deletetag"
	"github.com/cyril-fl/tide/core/services/process/readtag"
	"github.com/uptrace/bun"
)

type Body struct {
	Values []string `json:"values"`
	// DryRun answers what the deletion would do and writes nothing.
	DryRun bool `json:"dry_run,omitempty" binding:"optional"`
	// KeepEmpty leaves a note holding nothing but the tags empty instead of trashing it.
	KeepEmpty bool `json:"keep_empty,omitempty" binding:"optional"`
}

// Many deletes the tags of the body in one transaction, not the notes carrying them, and answers the tags deleted,
// the count of notes changed and the notes trashed; unknown ones are skipped: DELETE /api/tags.
// @Summary  Delete tags and erase them from the text of their notes
// @Tags     tags
// @Accept   json
// @Produce  json
// @Param    body body Body true "body"
// @Success  200 {object} deletetag.Result
// @Failure  400 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/tags [delete]
func Many(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input Body
		if !respond.Decode(w, r, &input) {
			return
		}

		process := deletetag.Process(r.Context(), db, input.Values)
		if input.DryRun {
			process.DryRun()
		}
		if input.KeepEmpty {
			process.KeepEmpty()
		}
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		respond.JSON(w, http.StatusOK, process.GetResult())
	}
}

// Handler deletes a tag, not the notes carrying it, and answers like Many: DELETE /api/tags/{value}.
// @Summary  Delete a tag and erase it from the text of its notes
// @Tags     tags
// @Produce  json
// @Param    value      path  string true  "tag"
// @Param    dry_run    query bool   false "answer what it would do, write nothing"
// @Param    keep_empty query bool   false "leave a note holding nothing but the tag empty instead of trashing it"
// @Success  200 {object} deletetag.Result
// @Failure  404 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/tags/{value} [delete]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := r.PathValue("value")
		process := deletetag.Process(r.Context(), db, []string{value})
		if r.URL.Query().Get("dry_run") == "true" {
			process.DryRun()
		}
		if r.URL.Query().Get("keep_empty") == "true" {
			process.KeepEmpty()
		}
		process.Run()
		if process.HasErr() {
			respond.Error(w, r, process.GetErr())
			return
		}
		// deletetag skips what it does not find: for one tag, that is a 404.
		if len(process.GetResult().Tags) == 0 {
			respond.Error(w, r, fmt.Errorf("%w: %q", readtag.ErrNotFound, value))
			return
		}
		respond.JSON(w, http.StatusOK, process.GetResult())
	}
}
