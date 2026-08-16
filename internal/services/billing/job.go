package billing

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Job struct {
	IdempotencyKey   string `json:"idempotency_key"`
	OrganizationID   string `json:"organization_id"`
	APIKeyID         string `json:"api_key_id"`
	ModelCatalogueID string `json:"model_catalogue_id"`
	Prompt           string `json:"prompt"`
	InputTokens      int    `json:"input_tokens"`
	OutputTokens     int    `json:"output_tokens"`
	LatencyMS        int    `json:"latency_ms"`
	Status           string `json:"status"`
	Error            string `json:"error"`
}

func NewIdempotencyKey() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}
