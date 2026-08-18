// Package anthropic is the HTTP glue for the Anthropic-compatible
// POST /v1/messages endpoint: it decodes/encodes JSON using
// internal/services/compat/anthropic's pure translation functions and
// drives the shared inferenceService.Converse call, mirroring the native
// internal/controller/inference controller.
package anthropic

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	sharedController "github.com/shivang-16/orbit.api/internal/controller/shared"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	anthropicCompat "github.com/shivang-16/orbit.api/internal/services/compat/anthropic"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type Controller struct {
	service *inferenceService.Service
	billing billingService.Enqueuer
}

func NewController(service *inferenceService.Service, billing billingService.Enqueuer) *Controller {
	return &Controller{service: service, billing: billing}
}

func (c *Controller) Messages(w http.ResponseWriter, r *http.Request) {
	var req anthropicCompat.MessagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		anthropicCompat.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if !req.IsValid() {
		anthropicCompat.WriteError(w, http.StatusBadRequest, "invalid_request_error", "model, max_tokens, and at least one message are required")
		return
	}

	var sink inferenceService.StreamSink
	if req.Stream {
		flusher, _ := w.(http.Flusher)
		sink = anthropicCompat.NewStreamSink(w, flusher, req.Model)
	}

	result, err := c.service.Converse(r.Context(), req.Model, req.ToConverse(), w, sink)
	if err != nil {
		c.writeServiceError(w, req.Model, err)
		return
	}

	if !result.Streamed {
		if result.StatusCode != http.StatusOK {
			anthropicCompat.WriteError(w, mapUpstreamStatus(result.StatusCode), "api_error", inferenceService.BedrockErrorMessage(result.Body))
		} else if body, ok := c.formatBufferedResponse(w, req.Model, result); ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		}
	}

	errBody := string(result.Body)
	if result.Streamed {
		errBody = "stream interrupted"
	}
	sharedController.RecordUsage(r.Context(), c.billing, "anthropic/messages", result, req.Prompt(), errBody)
}

func (c *Controller) formatBufferedResponse(w http.ResponseWriter, requestedModel string, result *inferenceService.ChatResult) ([]byte, bool) {
	parsed, err := inferenceService.ParseConverseResponse(result.Body)
	if err != nil {
		log.Printf("anthropic/messages: parse bedrock response: %v", err)
		anthropicCompat.WriteError(w, http.StatusBadGateway, "api_error", "failed to parse model response")
		return nil, false
	}
	modelName := result.ModelSlug
	if modelName == "" {
		modelName = requestedModel
	}
	body, err := anthropicCompat.NewMessageResponse(modelName, parsed)
	if err != nil {
		log.Printf("anthropic/messages: encode response: %v", err)
		anthropicCompat.WriteError(w, http.StatusInternalServerError, "api_error", "failed to encode response")
		return nil, false
	}
	return body, true
}

func (c *Controller) writeServiceError(w http.ResponseWriter, modelID string, err error) {
	log.Printf("anthropic/messages failed model=%s: %v", modelID, err)
	switch {
	case errors.Is(err, inferenceService.ErrInvalid):
		anthropicCompat.WriteError(w, http.StatusBadRequest, "invalid_request_error", "messages are required")
	case errors.Is(err, inferenceService.ErrModelNotFound):
		anthropicCompat.WriteError(w, http.StatusNotFound, "not_found_error", "model not found: "+modelID)
	case errors.Is(err, inferenceService.ErrUnsupportedProvider):
		anthropicCompat.WriteError(w, http.StatusBadGateway, "api_error", "model provider not supported yet")
	case errors.Is(err, inferenceService.ErrLowCredits):
		anthropicCompat.WriteError(w, http.StatusTooManyRequests, "rate_limit_error", "low on credits")
	default:
		anthropicCompat.WriteError(w, http.StatusBadGateway, "api_error", "failed to reach model provider")
	}
}

func mapUpstreamStatus(status int) int {
	if status >= 400 && status < 600 {
		return status
	}
	return http.StatusBadGateway
}
