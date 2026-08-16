// Package webhook processes verified payment-provider webhook payloads.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	planRepository "github.com/shivang-16/orbit.api/internal/repositories/plan"
)

type DodoService struct {
	billing *billingRepository.Repository
	plans   *planRepository.Repository
}

func NewDodoService(billing *billingRepository.Repository, plans *planRepository.Repository) *DodoService {
	return &DodoService{billing: billing, plans: plans}
}

type dodoEvent struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
}

type dodoObject struct {
	PaymentID      string          `json:"payment_id"`
	SubscriptionID string          `json:"subscription_id"`
	ID             string          `json:"id"`
	ProductID      string          `json:"product_id"`
	Metadata       json.RawMessage `json:"metadata"`
	Customer       *dodoCustomer   `json:"customer"`
	Items          []dodoItem      `json:"items"`
}

type dodoCustomer struct {
	Metadata json.RawMessage `json:"metadata"`
}

type dodoItem struct {
	Price *struct {
		Product string `json:"product"`
	} `json:"price"`
}

// HandleEvent routes an already-signature-verified Dodo webhook payload.
func (s *DodoService) HandleEvent(ctx context.Context, rawBody []byte) error {
	var event dodoEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		return fmt.Errorf("decode dodo event: %w", err)
	}

	eventType := event.Type
	if eventType == "" {
		eventType = event.EventType
	}

	payload := event.Data
	if len(payload) == 0 {
		payload = rawBody
	}

	switch eventType {
	case "subscription.active", "subscription.renewed":
		return s.handleSubscriptionGrant(ctx, eventType, payload)
	case "payment.succeeded":
		return s.handlePaymentSucceeded(ctx, payload)
	default:
		log.Printf("dodo webhook: ignoring event type %q", eventType)
		return nil
	}
}

// handleSubscriptionGrant credits the org for a new or renewed subscription.
// Dodo fires both subscription.active and subscription.renewed on the first
// charge, so the ledger key is scoped to the billing month and whichever
// event arrives first wins.
func (s *DodoService) handleSubscriptionGrant(ctx context.Context, eventType string, raw json.RawMessage) error {
	obj, err := decodeObject(raw)
	if err != nil {
		return err
	}

	orgID, planSlug, creditsMicros, err := s.resolveGrant(ctx, obj)
	if err != nil {
		return err
	}
	if orgID == "" || creditsMicros <= 0 {
		log.Printf("dodo webhook: %s missing organization/credits org=%q credits=%d product=%q — skipping", eventType, orgID, creditsMicros, firstNonEmpty(obj.ProductID))
		return nil
	}

	subscriptionID := firstNonEmpty(obj.SubscriptionID, obj.ID)
	period := time.Now().UTC().Format("2006-01")
	idempotencyKey := "dodo_period:" + subscriptionID + ":" + period
	if subscriptionID == "" {
		idempotencyKey = "dodo_event:" + eventType + ":" + period + ":" + orgID
	}

	note := "plan_purchase"
	if planSlug != "" {
		note = "plan:" + planSlug
	}

	if err := s.billing.GrantCredits(ctx, billingRepository.GrantParams{
		OrganizationID: orgID,
		AmountMicros:   creditsMicros,
		IdempotencyKey: idempotencyKey,
		Note:           note,
	}); err != nil {
		return fmt.Errorf("grant credits: %w", err)
	}

	log.Printf("dodo webhook: %s granted %d micros to org %s (plan=%s, key=%s)", eventType, creditsMicros, orgID, planSlug, idempotencyKey)
	return nil
}

// handlePaymentSucceeded is for one-time charges only. Subscription payments
// are covered by subscription.active / renewed so we do not double-grant.
func (s *DodoService) handlePaymentSucceeded(ctx context.Context, raw json.RawMessage) error {
	obj, err := decodeObject(raw)
	if err != nil {
		return err
	}
	if firstNonEmpty(obj.SubscriptionID) != "" {
		log.Printf("dodo webhook: payment.succeeded is a subscription charge — skipping (handled by subscription events)")
		return nil
	}

	orgID, planSlug, creditsMicros, err := s.resolveGrant(ctx, obj)
	if err != nil {
		return err
	}
	if orgID == "" || creditsMicros <= 0 {
		log.Printf("dodo webhook: payment.succeeded missing organization/credits — skipping")
		return nil
	}

	paymentID := firstNonEmpty(obj.PaymentID, obj.ID)
	if paymentID == "" {
		return fmt.Errorf("payment.succeeded missing payment id")
	}

	note := "plan_purchase"
	if planSlug != "" {
		note = "plan:" + planSlug
	}

	if err := s.billing.GrantCredits(ctx, billingRepository.GrantParams{
		OrganizationID: orgID,
		AmountMicros:   creditsMicros,
		IdempotencyKey: "dodo_payment:" + paymentID,
		Note:           note,
	}); err != nil {
		return fmt.Errorf("grant credits: %w", err)
	}

	log.Printf("dodo webhook: payment.succeeded granted %d micros to org %s (plan=%s, payment=%s)", creditsMicros, orgID, planSlug, paymentID)
	return nil
}

func (s *DodoService) resolveGrant(ctx context.Context, obj dodoObject) (orgID, planSlug string, creditsMicros int64, err error) {
	meta := mergeMetadata(obj.Metadata)
	if obj.Customer != nil {
		for key, value := range mergeMetadata(obj.Customer.Metadata) {
			if meta[key] == "" {
				meta[key] = value
			}
		}
	}

	orgID = firstNonEmpty(meta["organization_id"], meta["organizationId"])
	planSlug = firstNonEmpty(meta["plan_slug"], meta["planId"], meta["plan_id"])
	creditsMicros = parseInt64(meta["credits_micros"])

	productID := firstNonEmpty(obj.ProductID)
	if productID == "" && len(obj.Items) > 0 && obj.Items[0].Price != nil {
		productID = obj.Items[0].Price.Product
	}

	if productID != "" && (creditsMicros <= 0 || planSlug == "") {
		found, lookupErr := s.plans.GetByDodoProductID(ctx, productID)
		if lookupErr != nil {
			return "", "", 0, fmt.Errorf("load plan by product: %w", lookupErr)
		}
		if found != nil {
			if planSlug == "" {
				planSlug = found.Slug
			}
			if creditsMicros <= 0 {
				creditsMicros = found.CreditsMicros
			}
		}
	}
	if creditsMicros <= 0 && planSlug != "" {
		found, lookupErr := s.plans.GetBySlug(ctx, planSlug)
		if lookupErr != nil {
			return "", "", 0, fmt.Errorf("load plan by slug: %w", lookupErr)
		}
		if found != nil {
			creditsMicros = found.CreditsMicros
		}
	}

	return orgID, planSlug, creditsMicros, nil
}

func decodeObject(raw json.RawMessage) (dodoObject, error) {
	var obj dodoObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		return dodoObject{}, fmt.Errorf("decode dodo payload: %w", err)
	}
	return obj, nil
}

func mergeMetadata(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}

	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return out
	}
	for key, value := range asMap {
		switch typed := value.(type) {
		case string:
			out[key] = typed
		case float64:
			out[key] = strconv.FormatInt(int64(typed), 10)
		case json.Number:
			out[key] = typed.String()
		default:
			if value != nil {
				out[key] = fmt.Sprint(value)
			}
		}
	}
	return out
}

func parseInt64(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
