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
	PayloadType    string          `json:"payload_type"`
	PaymentID      string          `json:"payment_id"`
	SubscriptionID string          `json:"subscription_id"`
	ID             string          `json:"id"`
	ProductID      string          `json:"product_id"`
	InvoiceID      string          `json:"invoice_id"`
	TotalAmount    int             `json:"total_amount"`
	Amount         int             `json:"amount"`
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
	case "subscription.active", "subscription.renewed", "subscription.plan_changed":
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

	subscriptionID := firstNonEmpty(obj.SubscriptionID, obj.ID)
	if orgID == "" {
		orgID = s.orgIDFromPayment(ctx, obj)
	}
	if orgID == "" {
		orgID = s.orgIDFromSubscription(ctx, subscriptionID)
	}
	if planSlug == "" {
		if plan, planErr := s.enrichPlanFromSubscription(ctx, subscriptionID); planErr != nil {
			return planErr
		} else if plan != nil {
			planSlug = plan.Slug
			creditsMicros = plan.CreditsMicros
		}
	}
	if orgID == "" {
		logger.Infof(ctx, "dodo webhook: %s missing organization org=%q product=%q — skipping", eventType, orgID, firstNonEmpty(obj.ProductID))
		return nil
	}

	period := time.Now().UTC().Format("2006-01")
	previousPlanSlug := ""
	if s.orgs != nil {
		if org, orgErr := s.orgs.GetByID(ctx, orgID); orgErr == nil && org != nil {
			previousPlanSlug = org.PlanSlug
		}
	}
	currentPlanSlug := previousPlanSlug
	action, err := s.subscriptionAction(ctx, orgID, subscriptionID, planSlug, previousPlanSlug)
	if err != nil {
		return err
	}
	if action == persistIgnoreStale || action == persistDefer {
		if err := s.recordSubscriptionInvoice(ctx, orgID, planSlug, obj); err != nil {
			return err
		}
		if err := s.persistSubscription(ctx, orgID, subscriptionID, planSlug, previousPlanSlug); err != nil {
			return err
		}
		if action == persistDefer {
			logger.Infof(ctx, "dodo webhook: %s deferred subscription %s for org %s — plan unknown, no credits granted", eventType, subscriptionID, orgID)
			return nil
		}
		logger.Infof(ctx, "dodo webhook: %s ignored stale subscription %s for org %s (plan=%s) — no credits granted", eventType, subscriptionID, orgID, planSlug)
		return nil
	}

	if creditsMicros > 0 && planSlug != "" && s.billing != nil {
		legacyGranted := false
		if subscriptionID != "" {
			exists, keyErr := s.billing.HasIdempotencyKey(ctx, "dodo_period:"+subscriptionID+":"+period)
			if keyErr != nil {
				return fmt.Errorf("check grant key: %w", keyErr)
			}
			legacyGranted = exists
		}
		idempotencyKey := subscriptionGrantKey(subscriptionID, planSlug, currentPlanSlug, eventType, period, orgID, legacyGranted)

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
		if err := s.recordSubscriptionInvoice(ctx, orgID, planSlug, obj); err != nil {
			return err
		}
		if err := s.persistSubscription(ctx, orgID, subscriptionID, planSlug, previousPlanSlug); err != nil {
			return err
		}
		logger.Infof(ctx, "dodo webhook: %s granted %d micros to org %s (plan=%s, key=%s)", eventType, creditsMicros, orgID, planSlug, idempotencyKey)
		return nil
	}

	if err := s.recordSubscriptionInvoice(ctx, orgID, planSlug, obj); err != nil {
		return err
	}
	if err := s.persistSubscription(ctx, orgID, subscriptionID, planSlug, previousPlanSlug); err != nil {
		return err
	}
	logger.Infof(ctx, "dodo webhook: %s persisted subscription %s for org %s (plan=%s) — no credits to grant", eventType, subscriptionID, orgID, planSlug)
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
		if orgID == "" {
			orgID = s.orgIDFromPayment(ctx, obj)
		}
		if err := s.recordInvoice(ctx, orgID, planSlug, obj); err != nil {
			return err
		}
		previousPlanSlug := ""
		if s.orgs != nil && orgID != "" {
			if org, orgErr := s.orgs.GetByID(ctx, orgID); orgErr == nil && org != nil {
				previousPlanSlug = org.PlanSlug
			}
		}
		if err := s.persistSubscription(ctx, orgID, firstNonEmpty(obj.SubscriptionID), planSlug, previousPlanSlug); err != nil {
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
	if plan == nil {
		plan, lookupErr = s.enrichPlanFromPayment(ctx, obj)
		if lookupErr != nil {
			return "", "", 0, lookupErr
		}
	}
	if plan != nil {
		return orgID, plan.Slug, plan.CreditsMicros, nil
	}

	// No plan record matched — fall back to whatever credits amount Dodo
	// echoed back so we still grant something, but we cannot attach a tier.
	return orgID, "", parseInt64(meta["credits_micros"]), nil
}

func (s *DodoService) enrichPlanFromPayment(ctx context.Context, obj dodoObject) (*model.Plan, error) {
	paymentID := obj.paymentID()
	if s.dodo == nil || paymentID == "" || s.plans == nil {
		return nil, nil
	}
	payment, err := s.dodo.GetPayment(ctx, paymentID)
	if err != nil || payment == nil {
		return nil, nil
	}
	return s.resolvePlanFromPayment(ctx, obj, payment)
}

func (s *DodoService) enrichPlanFromSubscription(ctx context.Context, subscriptionID string) (*model.Plan, error) {
	if s.dodo == nil || s.plans == nil || strings.TrimSpace(subscriptionID) == "" {
		return nil, nil
	}
	payment, err := s.dodo.LatestSubscriptionPayment(ctx, subscriptionID)
	if err != nil || payment == nil {
		return nil, nil
	}
	return s.resolvePlanFromPayment(ctx, dodoObject{}, payment)
}

func (s *DodoService) resolvePlanFromPayment(ctx context.Context, obj dodoObject, payment *dodo.Payment) (*model.Plan, error) {
	if payment == nil {
		return nil, nil
	}
	payMeta := map[string]string{}
	for key, value := range payment.Metadata {
		if typed := strings.TrimSpace(fmt.Sprint(value)); typed != "" && typed != "<nil>" {
			payMeta[key] = typed
		}
	}
	if obj.ProductID == "" {
		obj.ProductID = strings.TrimSpace(payment.ProductID)
	}
	return s.resolvePlan(ctx, payMeta, obj)
}

func (s *DodoService) orgIDFromSubscription(ctx context.Context, subscriptionID string) string {
	if s.dodo == nil || strings.TrimSpace(subscriptionID) == "" {
		return ""
	}
	payment, err := s.dodo.LatestSubscriptionPayment(ctx, subscriptionID)
	if err != nil || payment == nil {
		return ""
	}
	return firstNonEmpty(stringMapValue(payment.Metadata, "organization_id"), stringMapValue(payment.Metadata, "organizationId"))
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

func (s *DodoService) recordSubscriptionInvoice(ctx context.Context, orgID, planSlug string, obj dodoObject) error {
	if paymentID := obj.paymentID(); paymentID != "" {
		return s.recordInvoice(ctx, orgID, planSlug, obj)
	}

	subscriptionID := firstNonEmpty(obj.SubscriptionID, obj.ID)
	if s.dodo == nil || subscriptionID == "" {
		logger.Infof(ctx, "dodo webhook: skip invoice persist org=%q subscription=%q — no payment id", orgID, subscriptionID)
		return nil
	}

	payment, err := s.dodo.LatestSubscriptionPayment(ctx, subscriptionID)
	if err != nil {
		return fmt.Errorf("load subscription payment %s: %w", subscriptionID, err)
	}
	if payment == nil {
		logger.Infof(ctx, "dodo webhook: no succeeded payment for subscription %s yet", subscriptionID)
		return nil
	}

	if orgID == "" {
		orgID = firstNonEmpty(stringMapValue(payment.Metadata, "organization_id"), stringMapValue(payment.Metadata, "organizationId"))
	}
	return s.recordInvoice(ctx, orgID, planSlug, dodoObject{
		PayloadType:    "Payment",
		PaymentID:      payment.PaymentID,
		InvoiceID:      payment.InvoiceID,
		TotalAmount:    payment.TotalAmount,
		Currency:       payment.Currency,
		Status:         firstNonEmpty(payment.Status, "succeeded"),
		RefundStatus:   payment.RefundStatus,
		SubscriptionID: firstNonEmpty(payment.SubscriptionID, subscriptionID),
		CreatedAt:      payment.CreatedAt,
	})
}

func (s *DodoService) orgIDFromPayment(ctx context.Context, obj dodoObject) string {
	paymentID := obj.paymentID()
	if s.dodo == nil || paymentID == "" {
		return ""
	}
	payment, err := s.dodo.GetPayment(ctx, paymentID)
	if err != nil || payment == nil {
		return ""
	}
	return firstNonEmpty(stringMapValue(payment.Metadata, "organization_id"), stringMapValue(payment.Metadata, "organizationId"))
}

func (s *DodoService) recordInvoice(ctx context.Context, orgID, planSlug string, obj dodoObject) error {
	if s.invoices == nil {
		return nil
	}
	paymentID := obj.paymentID()
	if orgID == "" {
		orgID = s.orgIDFromPayment(ctx, obj)
	}
	if orgID == "" || paymentID == "" {
		logger.Infof(ctx, "dodo webhook: skip invoice persist org=%q payment=%q", orgID, paymentID)
		return nil
	}

	invoiceID := strings.TrimSpace(obj.InvoiceID)
	amount := obj.amount()
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

func (s *DodoService) subscriptionAction(ctx context.Context, orgID, subscriptionID, incomingPlan, previousPlan string) (string, error) {
	if s.orgs == nil || orgID == "" || subscriptionID == "" {
		return persistAdopt, nil
	}
	stored, err := s.orgs.GetDodoSubscriptionID(ctx, orgID)
	if err != nil {
		return "", fmt.Errorf("load stored subscription: %w", err)
	}
	return s.persistDecision(ctx, orgID, stored, subscriptionID, incomingPlan, previousPlan)
}

func (s *DodoService) persistDecision(ctx context.Context, orgID, stored, incoming, incomingPlan, previousPlan string) (string, error) {
	incomingOrder, incomingKnown, err := s.planSortOrder(ctx, incomingPlan)
	if err != nil {
		return "", err
	}
	previousOrder, _, err := s.planSortOrder(ctx, previousPlan)
	if err != nil {
		return "", err
	}
	storedOrder, storedKnown, err := s.storedPlanOrder(ctx, orgID, stored)
	if err != nil {
		return "", err
	}
	return subscriptionPersistAction(stored, incoming, incomingKnown, incomingOrder, previousOrder, storedKnown, storedOrder), nil
}

func (s *DodoService) storedPlanOrder(ctx context.Context, orgID, stored string) (int, bool, error) {
	stored = strings.TrimSpace(stored)
	if stored == "" {
		return 0, false, nil
	}
	refs, err := s.invoiceSubscriptionRefs(ctx, orgID)
	if err != nil {
		return 0, false, err
	}
	for _, ref := range refs {
		if strings.TrimSpace(ref.ID) == stored {
			return s.planSortOrder(ctx, ref.PlanSlug)
		}
	}
	return 0, false, nil
}

func (s *DodoService) persistSubscription(ctx context.Context, orgID, subscriptionID, incomingPlan, previousPlan string) error {
	if s.orgs == nil || orgID == "" || subscriptionID == "" {
		return nil
	}

	stored, err := s.orgs.GetDodoSubscriptionID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("load stored subscription: %w", err)
	}

	action, err := s.persistDecision(ctx, orgID, stored, subscriptionID, incomingPlan, previousPlan)
	if err != nil {
		return err
	}

	switch action {
	case persistKeep:
		// Same stored id. Compare leftovers to the higher of the org's
		// pre-webhook plan and the incoming plan — attachPlan may already
		// have raised the org in this request (subscription.plan_changed).
		keepPlan, err := s.higherPlan(ctx, incomingPlan, previousPlan)
		if err != nil {
			return err
		}
		return s.cancelLowerPlanSubscriptions(ctx, orgID, subscriptionID, keepPlan)
	case persistDefer:
		logger.Infof(ctx, "dodo webhook: deferred subscription persist %s for org %s — incoming plan unknown (kept %s)", subscriptionID, orgID, stored)
		return nil
	case persistIgnoreStale:
		if s.dodo != nil {
			if err := s.dodo.CancelSubscription(ctx, subscriptionID); err != nil {
				return fmt.Errorf("cancel stale subscription %s: %w", subscriptionID, err)
			}
			logger.Infof(ctx, "dodo webhook: ignored stale subscription %s for org %s (kept %s)", subscriptionID, orgID, stored)
		}
		return nil
	}

	if err := s.cancelPriorSubscriptions(ctx, orgID, stored, subscriptionID); err != nil {
		return err
	}

	if err := s.orgs.SetDodoSubscriptionID(ctx, orgID, subscriptionID); err != nil {
		return fmt.Errorf("store subscription %s on org %s: %w", subscriptionID, orgID, err)
	}
	return nil
}

func (s *DodoService) cancelPriorSubscriptions(ctx context.Context, orgID, stored, incoming string) error {
	if s.dodo == nil {
		return nil
	}
	priors, err := s.priorSubscriptionIDs(ctx, orgID, stored, incoming)
	if err != nil {
		return err
	}
	return s.cancelSubscriptionIDs(ctx, orgID, priors)
}

func (s *DodoService) cancelLowerPlanSubscriptions(ctx context.Context, orgID, incoming, currentPlan string) error {
	if s.dodo == nil {
		return nil
	}
	currentOrder, currentKnown, err := s.planSortOrder(ctx, currentPlan)
	if err != nil {
		return err
	}
	refs, err := s.invoiceSubscriptionRefs(ctx, orgID)
	if err != nil {
		return err
	}
	resolved := make([]invoiceSubRef, 0, len(refs))
	for _, ref := range refs {
		order, known, planErr := s.planSortOrder(ctx, ref.PlanSlug)
		if planErr != nil {
			return planErr
		}
		resolved = append(resolved, invoiceSubRef{ID: ref.ID, Order: order, Known: known})
	}
	return s.cancelSubscriptionIDs(ctx, orgID, staleInvoiceIDs(incoming, currentOrder, currentKnown, resolved))
}

func (s *DodoService) cancelSubscriptionIDs(ctx context.Context, orgID string, ids []string) error {
	for _, priorID := range ids {
		if err := s.dodo.CancelSubscription(ctx, priorID); err != nil {
			return fmt.Errorf("cancel previous subscription %s: %w", priorID, err)
		}
		logger.Infof(ctx, "dodo webhook: cancelled previous subscription %s for org %s", priorID, orgID)
	}
	return nil
}

func (s *DodoService) priorSubscriptionIDs(ctx context.Context, orgID, stored, incoming string) ([]string, error) {
	refs, err := s.invoiceSubscriptionRefs(ctx, orgID)
	if err != nil {
		return nil, err
	}
	invoiceIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		invoiceIDs = append(invoiceIDs, ref.ID)
	}
	return mergePriorSubscriptionIDs(stored, incoming, invoiceIDs), nil
}

func (s *DodoService) invoiceSubscriptionRefs(ctx context.Context, orgID string) ([]invoiceRepository.SubscriptionRef, error) {
	if s.invoices == nil || strings.TrimSpace(orgID) == "" {
		return nil, nil
	}
	refs, err := s.invoices.SubscriptionsForOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list prior subscriptions for org %s: %w", orgID, err)
	}
	return refs, nil
}

type invoiceSubRef struct {
	ID    string
	Order int
	Known bool
}

// staleInvoiceIDs are invoice mandates that are safe to cancel on persistKeep.
// Only subscriptions whose plan is known and strictly below the org's current
// plan are cancelled, so a deferred upgrade invoice cannot be swept by a
// later webhook for the still-stored older subscription.
func staleInvoiceIDs(incoming string, currentOrder int, currentKnown bool, refs []invoiceSubRef) []string {
	if !currentKnown {
		return nil
	}
	incoming = strings.TrimSpace(incoming)
	out := make([]string, 0)
	seen := map[string]struct{}{}
	if incoming != "" {
		seen[incoming] = struct{}{}
	}
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if !ref.Known || ref.Order >= currentOrder {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// mergePriorSubscriptionIDs is the set of Dodo mandates to cancel when a
// subscription is adopted. The incoming id is never cancelled. Stored and
// every distinct invoice subscription_id are included so an orphaned older
// mandate is cancelled even after dodo_subscription_id is set.
func mergePriorSubscriptionIDs(stored, incoming string, invoiceIDs []string) []string {
	seen := map[string]struct{}{}
	if id := strings.TrimSpace(incoming); id != "" {
		seen[id] = struct{}{}
	}
	out := make([]string, 0)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	add(stored)
	for _, id := range invoiceIDs {
		add(id)
	}
	return out
}

func (s *DodoService) higherPlan(ctx context.Context, incomingPlan, previousPlan string) (string, error) {
	incomingOrder, incomingKnown, err := s.planSortOrder(ctx, incomingPlan)
	if err != nil {
		return "", err
	}
	previousOrder, previousKnown, err := s.planSortOrder(ctx, previousPlan)
	if err != nil {
		return "", err
	}
	return pickHigherPlan(incomingPlan, previousPlan, incomingKnown, incomingOrder, previousKnown, previousOrder), nil
}

func pickHigherPlan(incomingPlan, previousPlan string, incomingKnown bool, incomingOrder int, previousKnown bool, previousOrder int) string {
	if incomingKnown && (!previousKnown || incomingOrder > previousOrder) {
		return incomingPlan
	}
	return previousPlan
}

func (s *DodoService) planSortOrder(ctx context.Context, slug string) (int, bool, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" || s.plans == nil {
		return 0, false, nil
	}
	plan, err := s.plans.GetBySlug(ctx, slug)
	if err != nil {
		return 0, false, fmt.Errorf("load plan %s: %w", slug, err)
	}
	if plan == nil {
		return 0, false, nil
	}
	return plan.SortOrder, true, nil
}

const (
	persistKeep        = "keep"
	persistIgnoreStale = "stale"
	persistAdopt       = "adopt"
	persistDefer       = "defer"
)

func subscriptionPersistAction(stored, incoming string, incomingKnown bool, incomingOrder, previousOrder int, storedKnown bool, storedOrder int) string {
	if incoming == "" {
		return persistKeep
	}
	if stored == incoming {
		// Org already moved to a higher plan (attachPlan succeeded) but this
		// leftover mandate is still the stored id. Cancel it; do not grant.
		if incomingKnown && incomingOrder < previousOrder {
			return persistIgnoreStale
		}
		return persistKeep
	}
	if !incomingKnown {
		// payment.succeeded often has no plan slug. Cancelling would kill a
		// just-paid upgrade; adopting would overwrite a newer stored id with
		// a leftover. Wait for a later event that can resolve the plan.
		if stored == "" && previousOrder == 0 {
			return persistAdopt
		}
		return persistDefer
	}
	// Stored mandate is an older plan — adopt even if attachPlan already
	// raised the org to the incoming tier (incomingOrder == previousOrder).
	if storedKnown && incomingOrder > storedOrder {
		return persistAdopt
	}
	isUpgrade := incomingOrder > previousOrder
	isFirst := stored == "" && previousOrder == 0
	if isUpgrade || isFirst || (stored == "" && incomingOrder >= previousOrder) {
		return persistAdopt
	}
	return persistIgnoreStale
}

func subscriptionGrantKey(subscriptionID, incomingPlan, currentPlan, eventType, period, orgID string, legacyGranted bool) string {
	if subscriptionID == "" {
		return "dodo_event:" + eventType + ":" + period + ":" + orgID
	}
	legacy := "dodo_period:" + subscriptionID + ":" + period
	if incomingPlan == "" || (incomingPlan == currentPlan && legacyGranted) {
		return legacy
	}
	return "dodo_period:" + subscriptionID + ":" + incomingPlan + ":" + period
}

func (obj dodoObject) paymentID() string {
	if id := strings.TrimSpace(obj.PaymentID); id != "" {
		return id
	}
	if strings.EqualFold(obj.PayloadType, "Payment") {
		return strings.TrimSpace(obj.ID)
	}
	return ""
}

func (obj dodoObject) amount() int {
	if obj.TotalAmount > 0 {
		return obj.TotalAmount
	}
	return obj.Amount
}

func stringMapValue(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return strings.TrimSpace(fmt.Sprint(value))
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
