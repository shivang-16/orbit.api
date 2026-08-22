package billing

import (
	"context"
	"time"

	"github.com/shivang-16/orbit.api/internal/infra/sqs"
	"github.com/shivang-16/orbit.api/internal/logger"
)

func RunConsumer(ctx context.Context, client *sqs.Client, processor *Processor) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		messages, err := client.Receive(ctx, 10)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Error(logger.SetTag(ctx, logger.TagBilling), "billing: receive failed", "error", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, msg := range messages {
			job, err := DecodeJob(msg.Body)
			if err != nil {
				// Leave the message on the queue so SQS redrive can move it
				// to the DLQ after maxReceiveCount. Deleting here would
				// drop unbillable jobs forever.
				logger.Error(logger.SetTag(ctx, logger.TagBilling), "billing: invalid message", "receives", msg.ReceiveCount, "error", err)
				continue
			}

			if err := processor.Process(ctx, job); err != nil {
				logger.Error(logger.SetTag(ctx, logger.TagBilling), "billing: process failed", "receives", msg.ReceiveCount, "idempotency_key", job.IdempotencyKey, "error", err)
				continue
			}

			if err := client.Delete(ctx, msg.ReceiptHandle); err != nil {
				logger.Error(logger.SetTag(ctx, logger.TagBilling), "billing: delete failed", "idempotency_key", job.IdempotencyKey, "error", err)
			}
		}
	}
}
