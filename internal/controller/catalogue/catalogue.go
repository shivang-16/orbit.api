package catalogue

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	catalogueService "github.com/shivang-16/orbit.api/internal/services/catalogue"
)

type Controller struct {
	service *catalogueService.Service
}

func NewController(service *catalogueService.Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	resp, err := c.service.List(r.Context(), tag)
	if err != nil {
		log.Printf("catalogue/list tag=%q: %v", tag, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list models"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	resp, err := c.service.Get(r.Context(), id)
	if err != nil {
		log.Printf("catalogue/get id=%s: %v", id, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load model"})
		return
	}
	if resp == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) Overview(w http.ResponseWriter, r *http.Request) {
	resp, err := c.service.Overview(r.Context())
	if err != nil {
		log.Printf("catalogue/overview: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load overview"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
