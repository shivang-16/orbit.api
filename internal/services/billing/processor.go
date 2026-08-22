package billing

import (
	"context"
	"fmt"

	"github.com/shivang-16/orbit.api/internal/logger"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
)

type Processor struct {
	billing *billingRepository.Repository
	pricing *pricingRepository.Repository
}

func NewProcessor(billing *billingRepository.Repository, pricing *pricingRepository.Repository) *Processor {
	return &Processor{billing: billing, pricing: pricing}
}

func (p *Processor) Process(ctx context.Context, job Job) error {
	if job.IdempotencyKey == "" {
		return fmt.Errorf("idempotency key is required")
	}

	var vendorAmount int64
	if job.Status == "success" && job.ModelCatalogueID != "" {
		price, err := p.pricing.GetByCatalogueID(ctx, job.ModelCatalogueID)
		if err != nil {
			return fmt.Errorf("load price: %w", err)
		}
		if price == nil {
			return fmt.Errorf("no pricing row for model=%s", job.ModelCatalogueID)
		}
		vendorAmount = chargeMicros(job.InputTokens, job.OutputTokens, price.VendorInputPerMillionMicros, price.VendorOutputPerMillionMicros)
	}

	settled, err := p.billing.Record(ctx, billingRepository.RecordParams{
		IdempotencyKey:     job.IdempotencyKey,
		OrganizationID:     job.OrganizationID,
		APIKeyID:           job.APIKeyID,
		ModelCatalogueID:   job.ModelCatalogueID,
		Prompt:             job.Prompt,
		InputTokens:        job.InputTokens,
		OutputTokens:       job.OutputTokens,
		LatencyMS:          job.LatencyMS,
		Status:             job.Status,
		Error:              job.Error,
		AmountMicros:       vendorAmount,
		VendorAmountMicros: vendorAmount,
		HoldID:             job.HoldID,
	})
	if err != nil {
		return fmt.Errorf("record billing: %w", err)
	}
	ctx = logger.SetTag(ctx, logger.TagBilling)
	ctx = logger.SetOrg(ctx, job.OrganizationID)
	msg := "billing: recorded"
	if settled.Applied {
		msg = fmt.Sprintf(
			"billing: recorded hold=%d actual=%d refund=%d remaining %d -> %d",
			settled.AmountMicros, settled.ActualMicros, settled.RefundMicros, settled.RemainingBeforeMicros, settled.RemainingAfterMicros,
		)
	}
	logger.Info(ctx, msg,
		"model", job.ModelCatalogueID,
		"amount_micros", vendorAmount,
		"hold_micros", settled.AmountMicros,
		"actual_micros", settled.ActualMicros,
		"refund_micros", settled.RefundMicros,
		"remaining_before_micros", settled.RemainingBeforeMicros,
		"remaining_after_micros", settled.RemainingAfterMicros,
		"input_tokens", job.InputTokens,
		"output_tokens", job.OutputTokens,
		"status", job.Status,
		"idempotency_key", job.IdempotencyKey,
		"hold_id", job.HoldID,
	)
	return nil
}
