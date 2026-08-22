package billing

import (
	"context"
	"time"

	"github.com/shivang-16/orbit.api/internal/logger"
)

type ReliableEnqueuer struct {
	processor *Processor
	fallback  Enqueuer
}

func NewReliableEnqueuer(processor *Processor, fallback Enqueuer) *ReliableEnqueuer {
	return &ReliableEnqueuer{processor: processor, fallback: fallback}
}

func (e *ReliableEnqueuer) Enqueue(job Job) {
	if e == nil {
		return
	}
	if e.processor != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := e.processor.Process(ctx, job); err != nil {
			logger.Error(logger.SetTag(ctx, logger.TagBilling), "billing: sync settle failed", "idempotency_key", job.IdempotencyKey, "hold_id", job.HoldID, "error", err)
			if e.fallback != nil {
				e.fallback.Enqueue(job)
			}
			return
		}
		return
	}
	if e.fallback != nil {
		e.fallback.Enqueue(job)
	}
}
