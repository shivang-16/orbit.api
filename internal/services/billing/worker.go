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
	jobs      chan Job
	processor *Processor
}

func NewWorker(billing *billingRepository.Repository, pricing *pricingRepository.Repository) *Worker {
	w := &Worker{
		jobs:      make(chan Job, 256),
		processor: NewProcessor(billing, pricing),
	}
	go w.loop()
	return w
}

func (w *Worker) Enqueue(job Job) {
	if w == nil {
		return
	}
	select {
	case w.jobs <- job:
	default:
		go w.process(job)
	}
}

func (w *Worker) loop() {
	for job := range w.jobs {
		w.process(job)
	}
}

func (w *Worker) process(job Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.processor.Process(ctx, job); err != nil {
		log.Printf("billing: process: %v", err)
	}
}
