package webhook

import (
	"io"
	"log"
	"net/http"

	"github.com/shivang-16/orbit.api/internal/infra/dodo"
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
		log.Printf("dodo webhook: read body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	webhookID := r.Header.Get("Webhook-Id")
	webhookTimestamp := r.Header.Get("Webhook-Timestamp")
	webhookSignature := r.Header.Get("Webhook-Signature")
	if webhookID == "" || webhookTimestamp == "" || webhookSignature == "" {
		log.Printf("dodo webhook: missing signature headers id=%q ts=%q sig=%t", webhookID, webhookTimestamp, webhookSignature != "")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if c.dodoWebhookKey == "" {
		log.Printf("dodo webhook: DODO_WEBHOOK_KEY not set — skipping signature verification")
	} else if !dodo.VerifySignature(c.dodoWebhookKey, string(body), webhookID, webhookTimestamp, webhookSignature) {
		log.Printf("dodo webhook: signature verification failed for %s", webhookID)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	if err := c.dodoService.HandleEvent(r.Context(), body); err != nil {
		log.Printf("dodo webhook: handle event: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
