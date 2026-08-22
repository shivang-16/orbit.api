// Package webhook processes verified payment-provider webhook payloads.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shivang-16/orbit.api/internal/infra/dodo"
	"github.com/shivang-16/orbit.api/internal/logger"
	"github.com/shivang-16/orbit.api/internal/model"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	invoiceRepository "github.com/shivang-16/orbit.api/internal/repositories/invoice"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	planRepository "github.com/shivang-16/orbit.api/internal/repositories/plan"
)

type DodoService struct {
	billing  *billingRepository.Repository
	invoices *invoiceRepository.Repository
	dodo     *dodo.Client
	plans    *planRepository.Repository
	orgs     *organizationRepository.Repository
}

func NewDodoService(
	billing *billingRepository.Repository,
	invoices *invoiceRepository.Repository,
	dodoClient *dodo.Client,
	plans *planRepository.Repository,
	orgs *organizationRepository.Repository,
) *DodoService {
	return &DodoService{billing: billing, invoices: invoices, dodo: dodoClient, plans: plans, orgs: orgs}
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
	InvoiceID      string          `json:"invoice_id"`
	TotalAmount    int             `json:"total_amount"`
	Currency       string          `json:"currency"`
	Status         string          `json:"status"`
	RefundStatus   string          `json:"refund_status"`
	CreatedAt      time.Time       `json:"created_at"`
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
	case "refund.succeeded":
		return s.handleRefundSucceeded(ctx, payload)
	default:
		logger.Infof(ctx, "dodo webhook: ignoring event type %q", eventType)
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
		logger.Infof(ctx, "dodo webhook: %s missing organization/credits org=%q credits=%d product=%q — skipping", eventType, orgID, creditsMicros, firstNonEmpty(obj.ProductID))
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

	if err := s.attachPlan(ctx, orgID, planSlug); err != nil {
		return err
	}

	logger.Infof(ctx, "dodo webhook: %s granted %d micros to org %s (plan=%s, key=%s)", eventType, creditsMicros, orgID, planSlug, idempotencyKey)
	return nil
}

// handlePaymentSucceeded is for one-time charges only. Subscription payments
// are covered by subscription.active / renewed so we do not double-grant.
func (s *DodoService) handlePaymentSucceeded(ctx context.Context, raw json.RawMessage) error {
	obj, err := decodeObject(raw)
	if err != nil {
		return err
	}

	orgID, planSlug, creditsMicros, err := s.resolveGrant(ctx, obj)
	if err != nil {
		return err
	}

	if firstNonEmpty(obj.SubscriptionID) != "" {
		if err := s.recordInvoice(ctx, orgID, planSlug, obj); err != nil {
			return err
		}
		logger.Infof(ctx, "dodo webhook: payment.succeeded is a subscription charge — invoice saved, credits handled by subscription events")
		return nil
	}

	if orgID == "" || creditsMicros <= 0 {
		logger.Infof(ctx, "dodo webhook: payment.succeeded missing organization/credits — skipping grant")
		return s.recordInvoice(ctx, orgID, planSlug, obj)
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

	// Invoice persist is independent of attachPlan. Credits already landed;
	// a missing invoice is worse than a delayed plan slug. Both errors still
	// fail the webhook so Dodo retries — grant and upsert are idempotent.
	invoiceErr := s.recordInvoice(ctx, orgID, planSlug, obj)
	attachErr := s.attachPlan(ctx, orgID, planSlug)
	if invoiceErr == nil && attachErr == nil {
		logger.Infof(ctx, "dodo webhook: payment.succeeded granted %d micros to org %s (plan=%s, payment=%s)", creditsMicros, orgID, planSlug, paymentID)
		return nil
	}
	if invoiceErr != nil && attachErr != nil {
		return fmt.Errorf("%w (also attach plan: %v)", invoiceErr, attachErr)
	}
	if invoiceErr != nil {
		return invoiceErr
	}
	return attachErr
}

func (s *DodoService) handleRefundSucceeded(ctx context.Context, raw json.RawMessage) error {
	if s.invoices == nil {
		return nil
	}

	var refund struct {
		PaymentID  string `json:"payment_id"`
		IsPartial  bool   `json:"is_partial"`
		RefundType string `json:"refund_type"`
	}
	if err := json.Unmarshal(raw, &refund); err != nil {
		return fmt.Errorf("decode refund payload: %w", err)
	}

	paymentID := strings.TrimSpace(refund.PaymentID)
	if paymentID == "" {
		logger.Infof(ctx, "dodo webhook: refund.succeeded missing payment_id — skipping")
		return nil
	}

	refundStatus := normalizeRefundStatus("", refund.IsPartial || strings.EqualFold(refund.RefundType, "partial"))

	var payment *dodo.Payment
	if s.dodo != nil {
		fetched, err := s.dodo.GetPayment(ctx, paymentID)
		if err != nil {
			logger.Infof(ctx, "dodo webhook: refund enrich payment %s: %v", paymentID, err)
		} else {
			payment = fetched
			if payment != nil {
				// GetPayment can lag the webhook. A stale "partial" must not
				// overwrite a full refund from this event; full always wins.
				refundStatus = mergeRefundStatus(refundStatus, payment.RefundStatus)
			}
		}
	}

	existing, err := s.invoices.GetByPayment(ctx, paymentID)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.RefundStatus == "full" {
			return nil
		}
		if _, err := s.invoices.UpdateRefundStatus(ctx, paymentID, refundStatus); err != nil {
			return err
		}
		logger.Infof(ctx, "dodo webhook: refund.succeeded payment=%s status=%s", paymentID, refundStatus)
		return nil
	}

	if payment == nil {
		return fmt.Errorf("invoice not found for refunded payment %s", paymentID)
	}

	metaBytes, _ := json.Marshal(payment.Metadata)
	obj := dodoObject{
		PaymentID:      firstNonEmpty(payment.PaymentID, paymentID),
		InvoiceID:      payment.InvoiceID,
		TotalAmount:    payment.TotalAmount,
		Currency:       payment.Currency,
		Status:         firstNonEmpty(payment.Status, "succeeded"),
		RefundStatus:   refundStatus,
		SubscriptionID: payment.SubscriptionID,
		CreatedAt:      payment.CreatedAt,
		Metadata:       metaBytes,
	}
	orgID, planSlug, _, resolveErr := s.resolveGrant(ctx, obj)
	if resolveErr != nil {
		return resolveErr
	}
	if orgID == "" {
		return fmt.Errorf("invoice not found for refunded payment %s", paymentID)
	}
	if err := s.recordInvoice(ctx, orgID, planSlug, obj); err != nil {
		return err
	}
	logger.Infof(ctx, "dodo webhook: refund.succeeded created invoice payment=%s status=%s", paymentID, refundStatus)
	return nil
}

// resolveGrant figures out which org gets credited, for which plan tier, and
// how much. The plan is always resolved against our own `plans` table (by
// id, then slug, then Dodo product id) rather than trusted from raw webhook
// metadata, so a mangled or stale metadata value can never produce a plan
// slug that doesn't exist — that used to make attachPlan fail after credits
// had already been granted.
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

	plan, lookupErr := s.resolvePlan(ctx, meta, obj)
	if lookupErr != nil {
		return "", "", 0, lookupErr
	}
	if plan != nil {
		return orgID, plan.Slug, plan.CreditsMicros, nil
	}

	// No plan record matched — fall back to whatever credits amount Dodo
	// echoed back so we still grant something, but we cannot attach a tier.
	return orgID, "", parseInt64(meta["credits_micros"]), nil
}

// resolvePlan looks up the plan a checkout was for, preferring identifiers
// we control over ones a payment provider merely echoes back. plan_id is
// our plans.id UUID (set by CreateCheckout); plan_slug is the human slug;
// the Dodo product id is the last resort for older checkouts that predate
// metadata tagging.
func (s *DodoService) resolvePlan(ctx context.Context, meta map[string]string, obj dodoObject) (*model.Plan, error) {
	if planID := firstNonEmpty(meta["plan_id"]); planID != "" {
		found, err := s.plans.GetByID(ctx, planID)
		if err != nil {
			return nil, fmt.Errorf("load plan by id: %w", err)
		}
		if found != nil {
			return found, nil
		}
	}

	if planSlug := firstNonEmpty(meta["plan_slug"]); planSlug != "" {
		found, err := s.plans.GetBySlug(ctx, planSlug)
		if err != nil {
			return nil, fmt.Errorf("load plan by slug: %w", err)
		}
		if found != nil {
			return found, nil
		}
	}

	productID := firstNonEmpty(obj.ProductID)
	if productID == "" && len(obj.Items) > 0 && obj.Items[0].Price != nil {
		productID = obj.Items[0].Price.Product
	}
	if productID != "" {
		found, err := s.plans.GetByDodoProductID(ctx, productID)
		if err != nil {
			return nil, fmt.Errorf("load plan by product: %w", err)
		}
		if found != nil {
			return found, nil
		}
	}

	return nil, nil
}

func (s *DodoService) recordInvoice(ctx context.Context, orgID, planSlug string, obj dodoObject) error {
	if s.invoices == nil {
		return nil
	}
	paymentID := firstNonEmpty(obj.PaymentID, obj.ID)
	if orgID == "" || paymentID == "" {
		logger.Infof(ctx, "dodo webhook: skip invoice persist org=%q payment=%q", orgID, paymentID)
		return nil
	}

	invoiceID := strings.TrimSpace(obj.InvoiceID)
	amount := obj.TotalAmount
	currency := strings.TrimSpace(obj.Currency)
	status := firstNonEmpty(obj.Status, "succeeded")
	refundStatus := strings.TrimSpace(obj.RefundStatus)
	subscriptionID := firstNonEmpty(obj.SubscriptionID)
	paidAt := obj.CreatedAt

	if amount <= 0 || invoiceID == "" || currency == "" {
		if s.dodo == nil {
			return fmt.Errorf("payment %s missing amount/invoice fields and no dodo client", paymentID)
		}
		payment, err := s.dodo.GetPayment(ctx, paymentID)
		if err != nil {
			return fmt.Errorf("enrich payment %s: %w", paymentID, err)
		}
		if payment == nil {
			return fmt.Errorf("enrich payment %s: empty response", paymentID)
		}
		if invoiceID == "" {
			invoiceID = strings.TrimSpace(payment.InvoiceID)
		}
		if amount <= 0 {
			amount = payment.TotalAmount
		}
		if currency == "" {
			currency = payment.Currency
		}
		if refundStatus == "" {
			refundStatus = payment.RefundStatus
		}
		if subscriptionID == "" {
			subscriptionID = payment.SubscriptionID
		}
		if paidAt.IsZero() && !payment.CreatedAt.IsZero() {
			paidAt = payment.CreatedAt
		}
		if payment.Status != "" {
			status = payment.Status
		}
	}
	if amount <= 0 {
		return fmt.Errorf("payment %s has no amount after enrich", paymentID)
	}

	if err := s.invoices.Upsert(ctx, invoiceRepository.UpsertParams{
		OrganizationID: orgID,
		PaymentID:      paymentID,
		InvoiceID:      invoiceID,
		PlanSlug:       planSlug,
		Amount:         amount,
		Currency:       currency,
		Status:         status,
		RefundStatus:   refundStatus,
		SubscriptionID: subscriptionID,
		PaidAt:         paidAt,
	}); err != nil {
		return fmt.Errorf("save invoice: %w", err)
	}
	logger.Infof(ctx, "dodo webhook: saved invoice payment=%s org=%s amount=%d %s", paymentID, orgID, amount, currency)
	return nil
}

func (s *DodoService) attachPlan(ctx context.Context, orgID, planSlug string) error {
	if s.orgs == nil || orgID == "" || planSlug == "" {
		return nil
	}
	if err := s.orgs.SetPlanSlugIfHigher(ctx, orgID, planSlug); err != nil {
		return fmt.Errorf("attach plan %s to org %s: %w", planSlug, orgID, err)
	}
	return nil
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

func knownRefundStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "full", "refunded":
		return "full"
	case "partial", "partially_refunded":
		return "partial"
	default:
		return ""
	}
}

func normalizeRefundStatus(value string, isPartial bool) string {
	if known := knownRefundStatus(value); known != "" {
		return known
	}
	if isPartial {
		return "partial"
	}
	return "full"
}

func mergeRefundStatus(webhookStatus, paymentStatus string) string {
	if webhookStatus == "full" || knownRefundStatus(paymentStatus) == "full" {
		return "full"
	}
	if webhookStatus == "partial" || knownRefundStatus(paymentStatus) == "partial" {
		return "partial"
	}
	if webhookStatus != "" {
		return webhookStatus
	}
	return "full"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
