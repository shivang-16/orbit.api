package model

import "time"

type InferenceRequest struct {
	ID               string    `json:"id" db:"id"`
	OrganizationID   string    `json:"organization_id" db:"organization_id"`
	APIKeyID         *string   `json:"api_key_id" db:"api_key_id"`
	ModelCatalogueID *string   `json:"model_catalogue_id" db:"model_catalogue_id"`
	Prompt           string    `json:"prompt" db:"prompt"`
	InputTokens      int       `json:"input_tokens" db:"input_tokens"`
	OutputTokens     int       `json:"output_tokens" db:"output_tokens"`
	LatencyMS        int       `json:"latency_ms" db:"latency_ms"`
	Status           string    `json:"status" db:"status"`
	Error            string    `json:"error" db:"error"`
	IdempotencyKey   string    `json:"idempotency_key" db:"idempotency_key"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}
