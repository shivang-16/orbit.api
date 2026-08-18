package inference

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	sharedController "github.com/shivang-16/orbit.api/internal/controller/shared"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type Controller struct {
	service *inferenceService.Service
	billing billingService.Enqueuer
}

func NewController(service *inferenceService.Service, billing billingService.Enqueuer) *Controller {
	return &Controller{service: service, billing: billing}
}

func (c *Controller) Chat(w http.ResponseWriter, r *http.Request) {
	modelID := chi.URLParam(r, "id")

	var req inferenceService.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("inference/chat: invalid body model=%s: %v", modelID, err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := c.service.Chat(r.Context(), modelID, req, w)
	if err != nil {
		orgID, _ := apikeyMiddleware.OrganizationID(r.Context())
		log.Printf("inference/chat failed model=%s org=%s: %v", modelID, orgID, err)
		switch {
		case errors.Is(err, inferenceService.ErrInvalid):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "messages are required"})
		case errors.Is(err, inferenceService.ErrModelNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		case errors.Is(err, inferenceService.ErrUnsupportedProvider):
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "model provider not supported yet"})
		case errors.Is(err, inferenceService.ErrLowCredits):
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "low on credits"})
		default:
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to reach model provider"})
		}
		return
	}

	// When result.Streamed is true, the service already wrote the status
	// line, headers, and every SSE chunk directly to w — nothing left to
	// write here. Otherwise this is the buffered ("stream": false) path.
	if !result.Streamed {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(result.StatusCode)
		_, _ = w.Write(result.Body)
	}

	errBody := string(result.Body)
	if result.Streamed {
		errBody = "stream interrupted"
	}
	sharedController.RecordUsage(r.Context(), c.billing, "inference/chat", result, req.Prompt(), errBody)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
