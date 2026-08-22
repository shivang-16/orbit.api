package credits

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/shivang-16/orbit.api/internal/logger"
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
		logger.Error(r.Context(), "credits/get failed", "org_id", organizationID(r), "error", err)
		writeServiceError(w, err, "failed to load credits")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) ListOrganizationCreditHistory(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.ListOrganizationCreditHistory(r.Context(), organizationID(r))
	if err != nil {
		logger.Error(r.Context(), "credits/history failed", "org_id", organizationID(r), "error", err)
		writeServiceError(w, err, "failed to list credit history")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) GetUsage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resp, err := c.service.GetUsage(r.Context(), organizationID(r), q.Get("range"), q.Get("page"), q.Get("limit"))
	if err != nil {
		logger.Error(r.Context(), "usage/get failed", "org_id", organizationID(r), "range", q.Get("range"), "page", q.Get("page"), "limit", q.Get("limit"), "error", err)
		if errors.Is(err, creditsService.ErrInvalidRange) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid range"})
			return
		}
		if errors.Is(err, creditsService.ErrInvalidPage) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid page"})
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
