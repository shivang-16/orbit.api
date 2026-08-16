package inference

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := c.service.Chat(r.Context(), modelID, req)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)

	orgID, _ := apikeyMiddleware.OrganizationID(r.Context())
	apiKeyID, _ := apikeyMiddleware.APIKeyID(r.Context())
	status := "success"
	errText := ""
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		status = "error"
		errText = truncate(string(result.Body), 500)
	}

	if c.billing == nil {
		return
	}

	c.billing.Enqueue(billingService.Job{
		IdempotencyKey:   billingService.NewIdempotencyKey(),
		OrganizationID:   orgID,
		APIKeyID:         apiKeyID,
		ModelCatalogueID: result.ModelCatalogueID,
		Prompt:           req.Prompt(),
		InputTokens:      result.InputTokens,
		OutputTokens:     result.OutputTokens,
		LatencyMS:        result.LatencyMS,
		Status:           status,
		Error:            errText,
	})
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
