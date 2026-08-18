package sqs

import (
	"context"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	appconfig "github.com/shivang-16/orbit.api/internal/config"
)

type Message struct {
	Body          string
	ReceiptHandle string
	ReceiveCount  int
}

type Client struct {
	api      *awssqs.Client
	queueURL string
}

func New(ctx context.Context, cfg appconfig.Config) (*Client, error) {
	if cfg.SQS.BillingQueueURL == "" {
		return nil, fmt.Errorf("AWS_SQS_BILLING_QUEUE_URL is required")
	}
	if cfg.SQS.Region == "" {
		return nil, fmt.Errorf("AWS_QUEUE_REGION is required")
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.SQS.Region),
	}
	if cfg.AWS.AccessKeyID != "" && cfg.AWS.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWS.AccessKeyID, cfg.AWS.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &Client{
		api:      awssqs.NewFromConfig(awsCfg),
		queueURL: cfg.SQS.BillingQueueURL,
	}, nil
}

func (c *Client) Publish(ctx context.Context, body string) error {
	_, err := c.api.SendMessage(ctx, &awssqs.SendMessageInput{
		QueueUrl:    aws.String(c.queueURL),
		MessageBody: aws.String(body),
	})
	if err != nil {
		return fmt.Errorf("sqs send: %w", err)
	}
	return nil
}

func (c *Client) Receive(ctx context.Context, max int32) ([]Message, error) {
	out, err := c.api.ReceiveMessage(ctx, &awssqs.ReceiveMessageInput{
		QueueUrl:            aws.String(c.queueURL),
		MaxNumberOfMessages: max,
		WaitTimeSeconds:     20,
		VisibilityTimeout:   90,
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{
			types.MessageSystemAttributeNameApproximateReceiveCount,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("sqs receive: %w", err)
	}

	messages := make([]Message, 0, len(out.Messages))
	for _, msg := range out.Messages {
		messages = append(messages, Message{
			Body:          aws.ToString(msg.Body),
			ReceiptHandle: aws.ToString(msg.ReceiptHandle),
			ReceiveCount:  receiveCount(msg.Attributes),
		})
	}
	return messages, nil
}

func receiveCount(attrs map[string]string) int {
	raw, ok := attrs["ApproximateReceiveCount"]
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func (c *Client) Delete(ctx context.Context, receiptHandle string) error {
	_, err := c.api.DeleteMessage(ctx, &awssqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("sqs delete: %w", err)
	}
	return nil
}
