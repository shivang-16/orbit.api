package dodo

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Payment struct {
	PaymentID      string         `json:"payment_id"`
	InvoiceID      string         `json:"invoice_id"`
	InvoiceURL     string         `json:"invoice_url"`
	TotalAmount    int            `json:"total_amount"`
	Currency       string         `json:"currency"`
	Status         string         `json:"status"`
	RefundStatus   string         `json:"refund_status"`
	SubscriptionID string         `json:"subscription_id"`
	CreatedAt      time.Time      `json:"created_at"`
	Metadata       map[string]any `json:"metadata"`
}

func (c *Client) GetPayment(ctx context.Context, paymentID string) (*Payment, error) {
	if paymentID == "" {
		return nil, fmt.Errorf("payment id is required")
	}
	var item Payment
	if err := c.doJSON(ctx, http.MethodGet, "/payments/"+url.PathEscape(paymentID), nil, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (c *Client) GetInvoicePDF(ctx context.Context, paymentID string) ([]byte, string, error) {
	if paymentID == "" {
		return nil, "", fmt.Errorf("payment id is required")
	}
	return c.do(ctx, http.MethodGet, "/invoices/payments/"+url.PathEscape(paymentID), nil, "application/pdf")
}
