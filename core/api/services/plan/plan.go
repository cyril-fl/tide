// Package plan refuses the routes of a paid feature to the accounts whose plan lacks it.
package plan

import (
	"net/http"

	"github.com/cyril-fl/tide/core/api/services/respond"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/uptrace/bun"
)

// Require answers 403 to an account without feature (domain.Features), next otherwise.
// TODO: each account picks its own plan in PATCH /api/me, so this only follows the app; restrict that field to the admins once a plan is paid for.
func Require(db bun.IDB, feature string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var plan string
		err := db.NewSelect().Model((*domain.User)(nil)).Column("plan").Where("id = ?", domain.UserID(r.Context())).Scan(r.Context(), &plan)
		if err != nil {
			respond.Error(w, r, err)
			return
		}
		if !domain.Unlocked(feature, plan) {
			respond.JSON(w, http.StatusForbidden, respond.Failure{Error: "the plan of this account does not include " + feature})
			return
		}
		next.ServeHTTP(w, r)
	})
}
