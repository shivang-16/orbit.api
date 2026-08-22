package model

import "time"

type Invoice struct {
	ID             string    `json:"id" db:"id"`
	OrganizationID string    `json:"organization_id" db:"organization_id"`
	PaymentID      string    `json:"payment_id" db:"payment_id"`
	InvoiceID      string    `json:"invoice_id" db:"invoice_id"`
	PlanSlug       string    `json:"plan_slug" db:"plan_slug"`
	Amount         int       `json:"amount" db:"amount"`
	Currency       string    `json:"currency" db:"currency"`
	Status         string    `json:"status" db:"status"`
	RefundStatus   string    `json:"refund_status" db:"refund_status"`
	SubscriptionID string    `json:"subscription_id" db:"subscription_id"`
	PaidAt         time.Time `json:"paid_at" db:"paid_at"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}
