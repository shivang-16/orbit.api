package credits

import "time"

type OrganizationCreditsResponse struct {
	OrganizationID         string `json:"organization_id"`
	OrganizationName       string `json:"organization_name"`
	CreditsGrantedMicros   int64  `json:"credits_granted_micros"`
	CreditsUsedMicros      int64  `json:"credits_used_micros"`
	CreditsRemainingMicros int64  `json:"credits_remaining_micros"`
}

type HistoryEntry struct {
	ID             string    `json:"id"`
	EntryType      string    `json:"entry_type"`
	TypeLabel      string    `json:"type_label"`
	Description    string    `json:"description"`
	AmountMicros   int64     `json:"amount_micros"`
	IdempotencyKey string    `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

type HistoryResponse struct {
	Entries []HistoryEntry `json:"entries"`
	Total   int            `json:"total"`
}
