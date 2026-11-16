package edittag

import (
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/services/process/edittag"
	"github.com/uptrace/bun"
)

type Body struct {
	Value string `json:"value"`
	// Merge folds the tag into value when value is another tag already, instead of a 409.
	Merge bool `json:"merge,omitempty" binding:"optional"`
}

// Handler renames a tag: PATCH /api/tags/{value}.
// @Summary  Rename a tag, in the text of its notes too, or merge it into another with merge
// @Tags     tags
// @Accept   json
// @Produce  json
// @Param    value path string true "tag"
// @Param    body body Body true "body"
// @Success  200 {object} domain.Tag
// @Failure  400 {object} respond.Failure
// @Failure  404 {object} respond.Failure
// @Failure  409 {object} respond.Failure
// @Failure  413 {object} respond.Failure
// @Failure  500 {object} respond.Failure
// @Router   /api/tags/{value} [patch]
func Handler(db bun.IDB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input Body
		if !respond.Decode(w, r, &input) {
			return
		}

		process := edittag.Process(r.Context(), db, r.PathValue("value"), input.Value, input.Merge)
		process.Run()
		if process.HasErr() {
			respond.Error(w, process.GetErr())
			return
		}
		respond.JSON(w, http.StatusOK, process.GetResult())
	}
}
