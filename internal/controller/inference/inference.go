package inference

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type Controller struct {
	service *inferenceService.Service
}

func NewController(service *inferenceService.Service) *Controller {
	return &Controller{service: service}
}

// Chat validates the request, forwards it to the model provider, and
// streams the upstream response body straight back to the caller.
func (c *Controller) Chat(w http.ResponseWriter, r *http.Request) {
	modelID := chi.URLParam(r, "id")

	var req inferenceService.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	upstream, err := c.service.Chat(r.Context(), modelID, req)
	if err != nil {
		switch {
		case errors.Is(err, inferenceService.ErrInvalid):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages are required"})
		case errors.Is(err, inferenceService.ErrModelNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		case errors.Is(err, inferenceService.ErrUnsupportedProvider):
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "model provider not supported yet"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reach model provider"})
		}
		return
	}
	defer upstream.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(upstream.StatusCode)
	_, _ = io.Copy(w, upstream.Body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
