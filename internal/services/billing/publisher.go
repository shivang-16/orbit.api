package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/shivang-16/orbit.api/internal/infra/sqs"
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
		log.Printf("billing: encode job: %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.sqs.Publish(ctx, string(body)); err != nil {
		log.Printf("billing: publish sqs org=%s model=%s: %v", job.OrganizationID, job.ModelCatalogueID, err)
		return
	}
	log.Printf("billing: published sqs org=%s model=%s key=%s", job.OrganizationID, job.ModelCatalogueID, job.IdempotencyKey)
}

func DecodeJob(body string) (Job, error) {
	var job Job
	if err := json.Unmarshal([]byte(body), &job); err != nil {
		return Job{}, fmt.Errorf("decode billing job: %w", err)
	}
	if job.OrganizationID == "" {
		return Job{}, fmt.Errorf("billing job missing organization_id")
	}
	return job, nil
}
