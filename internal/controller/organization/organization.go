package organization

import (
	"encoding/json"
	"errors"
	"net/http"

	organizationService "github.com/shivang-16/orbit.api/internal/services/organization"
)

type Controller struct {
	service *organizationService.Service
}

func NewController(service *organizationService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list organizations"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
	var req organizationService.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	resp, err := c.service.Create(r.Context(), req)
	if err != nil {
		if errors.Is(err, organizationService.ErrInvalid) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create organization"})
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
