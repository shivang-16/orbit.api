package plan

import (
	"encoding/json"
	"net/http"

	"github.com/shivang-16/orbit.api/internal/logger"
	planService "github.com/shivang-16/orbit.api/internal/services/plan"
)

type Controller struct {
	service *planService.Service
}

func NewController(service *planService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.List(r.Context())
	if err != nil {
		logger.Error(r.Context(), "plans/list failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list plans"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
