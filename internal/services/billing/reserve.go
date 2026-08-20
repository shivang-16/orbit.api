package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shivang-16/orbit.api/internal/config"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type Reserver struct {
	billing    *billingRepository.Repository
	pricing    *pricingRepository.Repository
	threshold  int64
	defaultMax int
	holdTTL    time.Duration
}

func NewReserver(
	billing *billingRepository.Repository,
	pricing *pricingRepository.Repository,
	cfg config.Config,
) *Reserver {
	threshold := cfg.Credits.LowBalanceThresholdMicros
	if threshold < 1 {
		threshold = 10_000
	}
	defaultMax := cfg.Credits.DefaultOutputTokens
	if defaultMax < 1 {
		defaultMax = 4096
	}
	ttl := time.Duration(cfg.Server.InferenceTimeoutSeconds)*time.Second + time.Minute
	if ttl < 2*time.Minute {
		ttl = 6 * time.Minute
	}
	return &Reserver{
		billing:    billing,
		pricing:    pricing,
		threshold:  threshold,
		defaultMax: defaultMax,
		holdTTL:    ttl,
	}
}

func (r *Reserver) Reserve(ctx context.Context, params inferenceService.ReserveRequest) (*inferenceService.Hold, error) {
	if r == nil || r.billing == nil || r.pricing == nil {
		return nil, inferenceService.ErrLowCredits
	}
	if params.OrganizationID == "" {
		return nil, inferenceService.ErrLowCredits
	}

	price, err := r.pricing.GetByCatalogueID(ctx, params.CatalogueID)
	if err != nil {
		return nil, fmt.Errorf("load price: %w", err)
	}
	if price == nil {
		return nil, fmt.Errorf("no pricing row for model=%s", params.CatalogueID)
	}

	inputPerMillion := price.VendorInputPerMillionMicros
	outputPerMillion := price.VendorOutputPerMillionMicros
	inputTokens := params.InputTokens
	requestedMax := params.RequestedMaxTokens
	defaultMax := r.defaultMax
	threshold := r.threshold

	placed, err := r.billing.PlaceHold(ctx, params.OrganizationID, threshold, r.holdTTL, func(available int64) (int64, int, error) {
		plan, err := ComputeHold(
			inputTokens,
			requestedMax,
			defaultMax,
			inputPerMillion,
			outputPerMillion,
			available,
			threshold,
		)
		if err != nil {
			return 0, 0, err
		}
		return plan.AmountMicros, plan.MaxTokens, nil
	})
	if err != nil {
		if errors.Is(err, ErrInsufficientCredits) || errors.Is(err, billingRepository.ErrInsufficientCredits) {
			return nil, inferenceService.ErrLowCredits
		}
		return nil, err
	}

	return &inferenceService.Hold{
		ID:           placed.ID,
		AmountMicros: placed.AmountMicros,
		MaxTokens:    placed.MaxTokens,
	}, nil
}

func (r *Reserver) Release(ctx context.Context, holdID string) error {
	if r == nil || r.billing == nil || holdID == "" {
		return nil
	}
	return r.billing.ReleaseHold(ctx, holdID)
}
