package checkout

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/shivang-16/orbit.api/internal/logger"
	checkoutService "github.com/shivang-16/orbit.api/internal/services/checkout"
)

type Controller struct {
	service *checkoutService.Service
}

func NewController(service *checkoutService.Service) *Controller {
	return &Controller{service: service}
}

// Create handles POST /billing/checkout — creates a Dodo hosted checkout
// session for the requested plan and returns the URL to redirect to.
func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
	var req checkoutService.CreateCheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn(r.Context(), "checkout/create: invalid json", "error", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	if req.OrganizationID == "" {
		req.OrganizationID = strings.TrimSpace(r.Header.Get("X-Organization-Id"))
	}

	resp, err := c.service.CreateCheckout(r.Context(), req)
	if err != nil {
		logger.Error(r.Context(), "checkout/create failed", "plan", req.PlanSlug, "org_id", req.OrganizationID, "error", err)
		writeServiceError(w, err)
		return
	}
	logger.Info(r.Context(), "checkout/create ok", "plan", req.PlanSlug, "org_id", req.OrganizationID)
	writeJSON(w, http.StatusOK, resp)
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, checkoutService.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_slug is required"})
	case errors.Is(err, checkoutService.ErrPlanNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "plan not found"})
	case errors.Is(err, checkoutService.ErrNoOrganization):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no organization"})
	case errors.Is(err, checkoutService.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
	case errors.Is(err, checkoutService.ErrSamePlan):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "You are already on this plan.", "code": "same_plan"})
	case errors.Is(err, checkoutService.ErrDowngrade):
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Downgrades are not supported. Contact support if you need to change plans.",
			"code":  "downgrade_not_supported",
		})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create checkout session"})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
