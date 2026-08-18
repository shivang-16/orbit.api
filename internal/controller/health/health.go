package health

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	healthService "github.com/shivang-16/orbit.api/internal/services/health"
)

type Controller struct {
	service *healthService.Service
}

func NewController(service *healthService.Service) *Controller {
	return &Controller{service: service}
}

type liveResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
}

type readyResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Database  string `json:"database"`
	Timestamp string `json:"timestamp"`
}

func (c *Controller) Check(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, liveResponse{
		Status:    "ok",
		Service:   "orbit.api",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func (c *Controller) Ready(w http.ResponseWriter, r *http.Request) {
	if err := c.service.Ready(r.Context()); err != nil {
		log.Printf("health/ready: database ping failed: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, readyResponse{
			Status:    "unavailable",
			Service:   "orbit.api",
			Database:  "down",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	writeJSON(w, http.StatusOK, readyResponse{
		Status:    "ok",
		Service:   "orbit.api",
		Database:  "up",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
