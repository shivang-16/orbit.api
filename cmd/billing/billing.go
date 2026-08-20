package billing

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	"github.com/shivang-16/orbit.api/internal/infra/sqs"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
)

func Start(ctx context.Context, cfg config.Config) {
	db, err := postgres.OpenAndMigrate(cfg.Postgres, "migrations")
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	sqsClient, err := sqs.New(ctx, cfg)
	if err != nil {
		log.Fatalf("sqs: %v", err)
	}

	billingRepo := billingRepository.NewRepository(db.DB())
	processor := billingService.NewProcessor(
		billingRepo,
		pricingRepository.NewRepository(db.DB()),
	)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	billingService.StartHoldReclaimer(ctx, billingRepo)

	log.Printf("orbit.api billing worker listening on %s", cfg.SQS.BillingQueueURL)
	if err := billingService.RunConsumer(ctx, sqsClient, processor); err != nil && err != context.Canceled {
		log.Fatalf("billing consumer: %v", err)
	}
	log.Print("orbit.api billing worker stopped")
}
