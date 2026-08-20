package billing

import (
	"context"
	"log"
	"time"

	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
)

// Worker processes jobs in-process. Tests use this so they do not need SQS.
type Worker struct {
	processor *Processor
}

func NewWorker(billing *billingRepository.Repository, pricing *pricingRepository.Repository) *Worker {
	return &Worker{processor: NewProcessor(billing, pricing)}
}

func (w *Worker) Enqueue(job Job) {
	if w == nil {
		return
	}
	w.process(job)
}

func (w *Worker) process(job Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.processor.Process(ctx, job); err != nil {
		log.Printf("billing: process: %v", err)
	}
}
