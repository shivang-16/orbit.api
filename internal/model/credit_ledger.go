package model

import "time"

type CreditLedgerEntry struct {
	ID                 string    `json:"id" db:"id"`
	OrganizationID     string    `json:"organization_id" db:"organization_id"`
	InferenceRequestID *string   `json:"inference_request_id" db:"inference_request_id"`
	EntryType          string    `json:"entry_type" db:"entry_type"`
	AmountMicros       int64     `json:"amount_micros" db:"amount_micros"`
	VendorAmountMicros int64     `json:"vendor_amount_micros" db:"vendor_amount_micros"`
	OrbitAmountMicros  int64     `json:"orbit_amount_micros" db:"orbit_amount_micros"`
	IdempotencyKey     string    `json:"idempotency_key" db:"idempotency_key"`
	Note               string    `json:"note" db:"note"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}
