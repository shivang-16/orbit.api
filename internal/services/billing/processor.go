package billing

import (
	"context"
	"fmt"
	"log"

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

	if err := p.billing.Record(ctx, billingRepository.RecordParams{
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
	}); err != nil {
		return fmt.Errorf("record billing: %w", err)
	}
	log.Printf(
		"billing: recorded org=%s model=%s amount_micros=%d in=%d out=%d status=%s key=%s",
		job.OrganizationID, job.ModelCatalogueID, vendorAmount, job.InputTokens, job.OutputTokens, job.Status, job.IdempotencyKey,
	)
	return nil
}

func chargeMicros(inputTokens, outputTokens int, inputPerMillion, outputPerMillion int64) int64 {
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	return (int64(inputTokens)*inputPerMillion + int64(outputTokens)*outputPerMillion) / 1_000_000
}
