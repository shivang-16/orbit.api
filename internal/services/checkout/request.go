package checkout

import "strings"

type CreateCheckoutRequest struct {
	PlanSlug       string `json:"plan_slug"`
	OrganizationID string `json:"organization_id"`
}

func (r CreateCheckoutRequest) isValid() bool {
	return strings.TrimSpace(r.PlanSlug) != ""
}
