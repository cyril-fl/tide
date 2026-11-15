package readplans

import (
	"net/http"

	"github.com/cyril-fl/tide/notes-go/app/api/services/respond"
	"github.com/cyril-fl/tide/notes-go/app/shared/domain"
)

// Plan is a plan and the features it comes with.
type Plan struct {
	ID       string   `json:"id"`
	Features []string `json:"features"`
}

// Handler answers the plans in order, each with its features, for the comparison of the settings: GET /api/plans.
// @Summary  The plans in order, each with its features
// @Tags     account
// @Produce  json
// @Success  200 {array} Plan
// @Failure  401 {object} respond.Failure
// @Router   /api/plans [get]
func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plans := make([]Plan, len(domain.Plans))
		for i, plan := range domain.Plans {
			plans[i] = Plan{ID: plan, Features: domain.PlanFeatures(plan)}
		}
		respond.List(w, http.StatusOK, plans)
	}
}
