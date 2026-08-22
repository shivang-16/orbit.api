package billing

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	"github.com/shivang-16/orbit.api/internal/infra/sqs"
	"github.com/shivang-16/orbit.api/internal/logger"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
)

func Start(ctx context.Context, cfg config.Config) {
	logger.Init(cfg)
	ctx = logger.SetTag(ctx, logger.TagBilling)
	db, err := postgres.OpenAndMigrate(cfg.Postgres, "migrations")
	if err != nil {
		logger.Fatal(ctx, "postgres open failed", "error", err)
	}
	defer db.Close()

	sqsClient, err := sqs.New(ctx, cfg)
	if err != nil {
		logger.Fatal(ctx, "sqs init failed", "error", err)
	}

	billingRepo := billingRepository.NewRepository(db.DB())
	processor := billingService.NewProcessor(
		billingRepo,
		pricingRepository.NewRepository(db.DB()),
	)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	billingService.StartHoldReclaimer(ctx, billingRepo)

	logger.Info(ctx, "orbit.api billing worker listening", "queue", cfg.SQS.BillingQueueURL)
	if err := billingService.RunConsumer(ctx, sqsClient, processor); err != nil && err != context.Canceled {
		logger.Fatal(ctx, "billing consumer failed", "error", err)
	}
	logger.Info(ctx, "orbit.api billing worker stopped")
}
