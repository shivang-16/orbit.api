package dodo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type ChangePlanParams struct {
	ProductID            string            `json:"product_id"`
	Quantity             int               `json:"quantity"`
	ProrationBillingMode string            `json:"proration_billing_mode"`
	EffectiveAt          string            `json:"effective_at,omitempty"`
	OnPaymentFailure     string            `json:"on_payment_failure,omitempty"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

// ChangePlan upgrades or downgrades an existing Dodo subscription in place
// using the customer's saved payment method. See
// https://docs.dodopayments.com/api-reference/subscriptions/change-plan
func (c *Client) ChangePlan(ctx context.Context, subscriptionID string, params ChangePlanParams) error {
	subscriptionID = strings.TrimSpace(subscriptionID)
	if subscriptionID == "" {
		return fmt.Errorf("subscription id is required")
	}
	return c.doJSON(ctx, http.MethodPost, "/subscriptions/"+url.PathEscape(subscriptionID)+"/change-plan", params, nil)
}

// CancelSubscription ends a Dodo subscription immediately so a replacement
// checkout cannot leave two active mandates billing the same org.
func (c *Client) CancelSubscription(ctx context.Context, subscriptionID string) error {
	subscriptionID = strings.TrimSpace(subscriptionID)
	if subscriptionID == "" {
		return fmt.Errorf("subscription id is required")
	}
	err := c.doJSON(ctx, http.MethodPatch, "/subscriptions/"+url.PathEscape(subscriptionID), map[string]string{
		"status": "cancelled",
	}, nil)
	if err != nil && isAlreadyCancelled(err) {
		return nil
	}
	return err
}

func isAlreadyCancelled(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "(404)") || strings.Contains(msg, "not found") {
		return true
	}
	cancelled := strings.Contains(msg, "cancelled") || strings.Contains(msg, "canceled")
	inactive := strings.Contains(msg, "inactive")
	already := strings.Contains(msg, "already")
	// 422 is only "already done" when Dodo says the subscription itself is
	// cancelled or inactive. Bare "already" matches unrelated rejects
	// ("already has a pending plan change") and would leave mandates live.
	if strings.Contains(msg, "(422)") {
		return cancelled || inactive
	}
	return cancelled && (already || inactive)
}
