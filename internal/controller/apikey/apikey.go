package apikey

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/shivang-16/orbit.api/internal/logger"

	apikeyService "github.com/shivang-16/orbit.api/internal/services/apikey"
)

type Controller struct {
	service *apikeyService.Service
}

func NewController(service *apikeyService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.List(r.Context(), organizationID(r))
	if err != nil {
		logger.Error(r.Context(), "api-keys/list failed", "org_id", organizationID(r), "error", err)
		writeServiceError(w, err, "failed to list api keys")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
	var req apikeyService.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if req.OrganizationID == "" {
		req.OrganizationID = organizationID(r)
	}

	resp, err := c.service.Create(r.Context(), req)
	if err != nil {
		logger.Error(r.Context(), "api-keys/create failed", "org_id", req.OrganizationID, "error", err)
		writeServiceError(w, err, "failed to create api key")
		return
	}
	logger.Info(r.Context(), "api-keys/create ok", "org_id", req.OrganizationID)
	writeJSON(w, http.StatusCreated, resp)
}

func (c *Controller) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := c.service.Delete(r.Context(), id, organizationID(r)); err != nil {
		logger.Error(r.Context(), "api-keys/delete failed", "id", id, "org_id", organizationID(r), "error", err)
		writeServiceError(w, err, "failed to delete api key")
		return
	}
	// Soft-delete only: the row stays, status becomes inactive. 200 JSON
	// (not 204) so the Next.js proxy can forward a body without throwing.
	writeJSON(w, http.StatusOK, map[string]string{
		"ok":      "true",
		"message": "key removed successfully",
	})
}

func writeServiceError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, apikeyService.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and expiration are required"})
	case errors.Is(err, apikeyService.ErrNoOrganization):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no organization"})
	case errors.Is(err, apikeyService.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of this organization"})
	case errors.Is(err, apikeyService.ErrAdminRequired):
		writeJSON(w, http.StatusForbidden, map[string]string{
			"error": "only organization admins can delete API keys",
			"code":  "admin_required",
		})
	case errors.Is(err, apikeyService.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "api key not found"})
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
