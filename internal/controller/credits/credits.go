package credits

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	creditsService "github.com/shivang-16/orbit.api/internal/services/credits"
)

type Controller struct {
	service *creditsService.Service
}

func NewController(service *creditsService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) GetOrganizationCredits(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.GetOrganizationCredits(r.Context(), organizationID(r))
	if err != nil {
		log.Printf("credits/get org=%s: %v", organizationID(r), err)
		writeServiceError(w, err, "failed to load credits")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) ListOrganizationCreditHistory(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.ListOrganizationCreditHistory(r.Context(), organizationID(r))
	if err != nil {
		log.Printf("credits/history org=%s: %v", organizationID(r), err)
		writeServiceError(w, err, "failed to list credit history")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) GetUsage(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.GetUsage(r.Context(), organizationID(r), r.URL.Query().Get("range"))
	if err != nil {
		log.Printf("usage/get org=%s range=%s: %v", organizationID(r), r.URL.Query().Get("range"), err)
		if errors.Is(err, creditsService.ErrInvalidRange) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid range"})
			return
		}
		writeServiceError(w, err, "failed to load usage")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeServiceError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, creditsService.ErrNoOrganization):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no organization"})
	case errors.Is(err, creditsService.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fallback})
	}
}

func organizationID(r *http.Request) string {
	if value := strings.TrimSpace(r.URL.Query().Get("organization_id")); value != "" {
		return value
	}
	return strings.TrimSpace(r.Header.Get("X-Organization-Id"))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
