package billing

import (
	"context"
	"log"
	"time"

	"github.com/shivang-16/orbit.api/internal/infra/sqs"
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
			log.Printf("billing: receive: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, msg := range messages {
			job, err := DecodeJob(msg.Body)
			if err != nil {
				// Leave the message on the queue so SQS redrive can move it
				// to the DLQ after maxReceiveCount. Deleting here would
				// drop unbillable jobs forever.
				log.Printf("billing: invalid message receives=%d: %v", msg.ReceiveCount, err)
				continue
			}

			if err := processor.Process(ctx, job); err != nil {
				log.Printf("billing: process receives=%d key=%s: %v", msg.ReceiveCount, job.IdempotencyKey, err)
				continue
			}

			if err := client.Delete(ctx, msg.ReceiptHandle); err != nil {
				log.Printf("billing: delete key=%s: %v", job.IdempotencyKey, err)
			}
		}
	}
}
