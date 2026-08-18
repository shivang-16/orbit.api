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
	ID           string    `json:"id"`
	EntryType    string    `json:"entry_type"`
	TypeLabel    string    `json:"type_label"`
	ModelName    string    `json:"model_name"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	LatencyMS    int       `json:"latency_ms"`
	AmountMicros int64     `json:"amount_micros"`
	CreatedAt    time.Time `json:"created_at"`
}

type HistoryResponse struct {
	Entries []HistoryEntry `json:"entries"`
	Total   int            `json:"total"`
}

type UsageModelPoint struct {
	ModelID      string `json:"model_id"`
	ModelName    string `json:"model_name"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
}

type UsageDay struct {
	Date   string            `json:"date"`
	Models []UsageModelPoint `json:"models"`
}

type UsageRequest struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	ModelName    string    `json:"model_name"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	LatencyMS    int       `json:"latency_ms"`
	AmountMicros int64     `json:"amount_micros"`
	Status       string    `json:"status"`
}

type UsageResponse struct {
	Range        string         `json:"range"`
	From         time.Time      `json:"from"`
	To           time.Time      `json:"to"`
	InputTokens  int64          `json:"input_tokens"`
	OutputTokens int64          `json:"output_tokens"`
	TotalTokens  int64          `json:"total_tokens"`
	CostMicros   int64          `json:"cost_micros"`
	Series       []UsageDay     `json:"series"`
	Requests     []UsageRequest `json:"requests"`
}
