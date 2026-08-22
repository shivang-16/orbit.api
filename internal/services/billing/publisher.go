package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shivang-16/orbit.api/internal/infra/sqs"
	"github.com/shivang-16/orbit.api/internal/logger"
)

type Enqueuer interface {
	Enqueue(job Job)
}

type Publisher struct {
	sqs *sqs.Client
}

func NewPublisher(client *sqs.Client) *Publisher {
	return &Publisher{sqs: client}
}

func (p *Publisher) Enqueue(job Job) {
	if p == nil || p.sqs == nil {
		return
	}

	body, err := json.Marshal(job)
	if err != nil {
		logger.Error(logger.SetTag(context.Background(), logger.TagBilling), "billing: encode job failed", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = logger.SetTag(ctx, logger.TagBilling)
	ctx = logger.SetOrg(ctx, job.OrganizationID)
	if err := p.sqs.Publish(ctx, string(body)); err != nil {
		logger.Error(ctx, "billing: publish sqs failed", "model", job.ModelCatalogueID, "error", err)
		return
	}
	logger.Info(ctx, "billing: published sqs", "model", job.ModelCatalogueID, "idempotency_key", job.IdempotencyKey)
}

func DecodeJob(body string) (Job, error) {
	var job Job
	if err := json.Unmarshal([]byte(body), &job); err != nil {
		return Job{}, fmt.Errorf("decode billing job: %w", err)
	}
	if job.OrganizationID == "" {
		return Job{}, fmt.Errorf("billing job missing organization_id")
	}
	if job.IdempotencyKey == "" {
		return Job{}, fmt.Errorf("billing job missing idempotency_key")
	}
	return job, nil
}
