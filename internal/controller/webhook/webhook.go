package webhook

import (
	"io"
	"net/http"

	"github.com/shivang-16/orbit.api/internal/infra/dodo"
	"github.com/shivang-16/orbit.api/internal/logger"
	webhookService "github.com/shivang-16/orbit.api/internal/services/webhook"
)

type Controller struct {
	dodoWebhookKey string
	dodoService    *webhookService.DodoService
}

func NewController(dodoWebhookKey string, dodoService *webhookService.DodoService) *Controller {
	return &Controller{dodoWebhookKey: dodoWebhookKey, dodoService: dodoService}
}

// Dodo handles POST /webhooks/dodo. It must read the raw body for signature
// verification before anything else touches the request.
func (c *Controller) Dodo(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error(r.Context(), "dodo webhook: read body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	webhookID := r.Header.Get("Webhook-Id")
	webhookTimestamp := r.Header.Get("Webhook-Timestamp")
	webhookSignature := r.Header.Get("Webhook-Signature")
	if webhookID == "" || webhookTimestamp == "" || webhookSignature == "" {
		logger.Warn(r.Context(), "dodo webhook: missing signature headers", "webhook_id", webhookID, "has_timestamp", webhookTimestamp != "", "has_signature", webhookSignature != "")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if c.dodoWebhookKey == "" {
		logger.Warn(r.Context(), "dodo webhook: DODO_WEBHOOK_KEY not set — skipping signature verification")
	} else if !dodo.VerifySignature(c.dodoWebhookKey, string(body), webhookID, webhookTimestamp, webhookSignature) {
		logger.Warn(r.Context(), "dodo webhook: signature verification failed", "webhook_id", webhookID)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := c.dodoService.HandleEvent(r.Context(), body); err != nil {
		logger.Error(r.Context(), "dodo webhook: handle event failed", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
