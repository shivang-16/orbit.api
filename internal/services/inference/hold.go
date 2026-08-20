package inference

import "context"

type Hold struct {
	ID           string
	AmountMicros int64
	MaxTokens    int
}

type ReserveRequest struct {
	OrganizationID     string
	CatalogueID        string
	InputTokens        int
	RequestedMaxTokens int
}

type Reserver interface {
	Reserve(ctx context.Context, params ReserveRequest) (*Hold, error)
	Release(ctx context.Context, holdID string) error
}
