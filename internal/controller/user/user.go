package user

import (
	"encoding/json"
	"net/http"

	userService "github.com/shivang-16/orbit.api/internal/services/user"
)

type Controller struct {
	service *userService.Service
}

func NewController(service *userService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) Sync(w http.ResponseWriter, r *http.Request) {
	user, created, err := c.service.Sync(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to sync user"})
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, user)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
