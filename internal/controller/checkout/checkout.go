package checkout

import (
	"encoding/json"
	"errors"
	"net/http"

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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	resp, err := c.service.CreateCheckout(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
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
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create checkout session"})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
