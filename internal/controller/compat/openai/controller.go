// Package openai is the HTTP glue for the OpenAI-compatible endpoints
// (POST /v1/chat/completions, GET /v1/models): it decodes/encodes JSON
// using internal/services/compat/openai's pure translation functions and
// drives the shared inferenceService.Converse call, mirroring the native
// internal/controller/inference controller.
package openai

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	sharedController "github.com/shivang-16/orbit.api/internal/controller/shared"
	"github.com/shivang-16/orbit.api/internal/limiter"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	openaiCompat "github.com/shivang-16/orbit.api/internal/services/compat/openai"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type Controller struct {
	service   *inferenceService.Service
	catalogue *catalogueRepository.Repository
	billing   billingService.Enqueuer
}

func NewController(service *inferenceService.Service, catalogue *catalogueRepository.Repository, billing billingService.Enqueuer) *Controller {
	return &Controller{service: service, catalogue: catalogue, billing: billing}
}

func (c *Controller) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	var req openaiCompat.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		openaiCompat.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if !req.IsValid() {
		openaiCompat.WriteError(w, http.StatusBadRequest, "invalid_request_error", "model and at least one message are required")
		return
	}

	var sink inferenceService.StreamSink
	if req.Stream {
		flusher, _ := w.(http.Flusher)
		sink = openaiCompat.NewStreamSink(w, flusher, req.Model, req.WantsUsage())
	}

	result, err := c.service.Converse(r.Context(), req.Model, req.ToConverse(), w, sink)
	if err != nil {
		c.writeServiceError(w, req.Model, err)
		return
	}

	if !result.Streamed {
		if result.StatusCode != http.StatusOK {
			openaiCompat.WriteError(w, mapUpstreamStatus(result.StatusCode), "api_error", inferenceService.BedrockErrorMessage(result.Body))
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
	sharedController.RecordUsage(r.Context(), c.billing, "openai/chat.completions", result, req.Prompt(), errBody)
}

func (c *Controller) formatBufferedResponse(w http.ResponseWriter, requestedModel string, result *inferenceService.ChatResult) ([]byte, bool) {
	parsed, err := inferenceService.ParseConverseResponse(result.Body)
	if err != nil {
		log.Printf("openai/chat.completions: parse bedrock response: %v", err)
		openaiCompat.WriteError(w, http.StatusBadGateway, "api_error", "failed to parse model response")
		return nil, false
	}
	modelName := result.ModelSlug
	if modelName == "" {
		modelName = requestedModel
	}
	body, err := openaiCompat.NewChatCompletionResponse(modelName, parsed)
	if err != nil {
		log.Printf("openai/chat.completions: encode response: %v", err)
		openaiCompat.WriteError(w, http.StatusInternalServerError, "api_error", "failed to encode response")
		return nil, false
	}
	return body, true
}

func (c *Controller) ListModels(w http.ResponseWriter, r *http.Request) {
	models, err := c.catalogue.ListActive(r.Context(), "")
	if err != nil {
		log.Printf("openai/models: list: %v", err)
		openaiCompat.WriteError(w, http.StatusInternalServerError, "api_error", "failed to list models")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openaiCompat.NewModelListResponse(models))
}

func (c *Controller) writeServiceError(w http.ResponseWriter, modelID string, err error) {
	log.Printf("openai/chat.completions failed model=%s: %v", modelID, err)
	if limiter.SetHeadersFromError(w, err) {
		openaiCompat.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "rate limit exceeded")
		return
	}
	switch {
	case errors.Is(err, inferenceService.ErrInvalid):
		openaiCompat.WriteError(w, http.StatusBadRequest, "invalid_request_error", "messages are required")
	case errors.Is(err, inferenceService.ErrModelNotFound):
		openaiCompat.WriteError(w, http.StatusNotFound, "invalid_request_error", "model not found: "+modelID)
	case errors.Is(err, inferenceService.ErrUnsupportedProvider):
		openaiCompat.WriteError(w, http.StatusBadGateway, "api_error", "model provider not supported yet")
	case errors.Is(err, inferenceService.ErrLowCredits):
		openaiCompat.WriteError(w, http.StatusPaymentRequired, "insufficient_quota", "low on credits")
	default:
		openaiCompat.WriteError(w, http.StatusBadGateway, "api_error", "failed to reach model provider")
	}
}

func mapUpstreamStatus(status int) int {
	if status >= 400 && status < 600 {
		return status
	}
	return http.StatusBadGateway
}
