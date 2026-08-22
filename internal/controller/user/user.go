package user

import (
	"encoding/json"
	"net/http"

	"github.com/shivang-16/orbit.api/internal/logger"
	userService "github.com/shivang-16/orbit.api/internal/services/user"
)

type Controller struct {
	service *userService.Service
}

func NewController(service *userService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) Sync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, created, err := c.service.Sync(ctx)
	if err != nil {
		logger.Error(ctx, "users/sync failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to sync user"})
		return
	}

	if user != nil {
		logger.SetUser(ctx, user.ID, user.Email)
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
		logger.Info(ctx, "users/sync created", "user_id", user.ID, "email", user.Email)
	} else {
		logger.Info(ctx, "users/sync ok", "user_id", user.ID)
	}
	writeJSON(w, status, user)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
