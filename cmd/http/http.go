package httpserver

import (
	"context"
	"net/http"

	"github.com/shivang-16/orbit.api/internal/config"
	apikeyController "github.com/shivang-16/orbit.api/internal/controller/apikey"
	catalogueController "github.com/shivang-16/orbit.api/internal/controller/catalogue"
	checkoutController "github.com/shivang-16/orbit.api/internal/controller/checkout"
	anthropicController "github.com/shivang-16/orbit.api/internal/controller/compat/anthropic"
	openaiController "github.com/shivang-16/orbit.api/internal/controller/compat/openai"
	creditsController "github.com/shivang-16/orbit.api/internal/controller/credits"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	inferenceController "github.com/shivang-16/orbit.api/internal/controller/inference"
	invoicesController "github.com/shivang-16/orbit.api/internal/controller/invoices"
	organizationController "github.com/shivang-16/orbit.api/internal/controller/organization"
	planController "github.com/shivang-16/orbit.api/internal/controller/plan"
	userController "github.com/shivang-16/orbit.api/internal/controller/user"
	webhookController "github.com/shivang-16/orbit.api/internal/controller/webhook"
	"github.com/shivang-16/orbit.api/internal/infra/clerk"
	"github.com/shivang-16/orbit.api/internal/infra/dodo"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	"github.com/shivang-16/orbit.api/internal/infra/resend"
	"github.com/shivang-16/orbit.api/internal/infra/sqs"
	"github.com/shivang-16/orbit.api/internal/logger"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	apikeyRepository "github.com/shivang-16/orbit.api/internal/repositories/apikey"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
	invoiceRepository "github.com/shivang-16/orbit.api/internal/repositories/invoice"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	planRepository "github.com/shivang-16/orbit.api/internal/repositories/plan"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
	"github.com/shivang-16/orbit.api/internal/routes"
	apikeyService "github.com/shivang-16/orbit.api/internal/services/apikey"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	catalogueService "github.com/shivang-16/orbit.api/internal/services/catalogue"
	checkoutService "github.com/shivang-16/orbit.api/internal/services/checkout"
	creditsService "github.com/shivang-16/orbit.api/internal/services/credits"
	healthService "github.com/shivang-16/orbit.api/internal/services/health"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
	invoicesService "github.com/shivang-16/orbit.api/internal/services/invoices"
	mailService "github.com/shivang-16/orbit.api/internal/services/mail"
	organizationService "github.com/shivang-16/orbit.api/internal/services/organization"
	planService "github.com/shivang-16/orbit.api/internal/services/plan"
	userService "github.com/shivang-16/orbit.api/internal/services/user"
	webhookService "github.com/shivang-16/orbit.api/internal/services/webhook"
)

func Start(ctx context.Context, cfg config.Config) {
	logger.Init(cfg)
	if cfg.ClerkSecretKey == "" {
		logger.Fatal(ctx, "CLERK_SECRET_KEY is required")
	}

	db, err := postgres.OpenAndMigrate(cfg.Postgres, "migrations")
	if err != nil {
		logger.Fatal(ctx, "postgres open failed", "error", err)
	}
	defer db.Close()

	sqsClient, err := sqs.New(context.Background(), cfg)
	if err != nil {
		logger.Fatal(ctx, "sqs init failed", "error", err)
	}
	billingPublisher := billingService.NewPublisher(sqsClient)

	clerkClient := clerk.New(cfg.ClerkSecretKey)
	mailSvc := mailService.NewService(resend.New(cfg))
	userRepo := userRepository.NewRepository(db.DB())
	orgRepo := organizationRepository.NewRepository(db.DB())
	userSvc := userService.NewService(db.DB(), userRepo, orgRepo, clerkClient, mailSvc, cfg.Credits.SignupMicros)
	userCtrl := userController.NewController(userSvc)

	catalogueRepo := catalogueRepository.NewRepository(db.DB())
	pricingRepo := pricingRepository.NewRepository(db.DB())
	catalogueSvc := catalogueService.NewService(catalogueRepo, pricingRepo)
	catalogueCtrl := catalogueController.NewController(catalogueSvc)

	apiKeyRepo := apikeyRepository.NewRepository(db.DB())
	apiKeySvc := apikeyService.NewService(apiKeyRepo, orgRepo)
	apiKeyCtrl := apikeyController.NewController(apiKeySvc)

	orgSvc := organizationService.NewService(db.DB(), orgRepo, userRepo)
	orgCtrl := organizationController.NewController(orgSvc)

	billingRepo := billingRepository.NewRepository(db.DB())
	reserver := billingService.NewReserver(billingRepo, pricingRepo, cfg)
	inferenceSvc := inferenceService.NewService(catalogueRepo, reserver, cfg)
	processor := billingService.NewProcessor(billingRepo, pricingRepo)
	billingEnqueuer := billingService.NewReliableEnqueuer(processor, billingPublisher)
	inferenceCtrl := inferenceController.NewController(inferenceSvc, billingEnqueuer, orgRepo)
	openaiCompatCtrl := openaiController.NewController(inferenceSvc, catalogueRepo, billingEnqueuer)
	anthropicCompatCtrl := anthropicController.NewController(inferenceSvc, billingEnqueuer)
	apiKeyAuth := apikeyMiddleware.New(apiKeyRepo, userRepo)
	clerkAuth := authMiddleware.New(userRepo)

	planRepo := planRepository.NewRepository(db.DB())
	planSvc := planService.NewService(planRepo)
	planCtrl := planController.NewController(planSvc)

	dodoClient := dodo.New(cfg)
	checkoutSvc := checkoutService.NewService(dodoClient, clerkClient, planRepo, orgRepo, cfg)
	checkoutCtrl := checkoutController.NewController(checkoutSvc)

	invoiceRepo := invoiceRepository.NewRepository(db.DB())
	dodoWebhookSvc := webhookService.NewDodoService(billingRepo, invoiceRepo, dodoClient, planRepo, orgRepo)
	webhookCtrl := webhookController.NewController(cfg.Dodo.WebhookKey, dodoWebhookSvc)

	creditsSvc := creditsService.NewService(billingRepo, orgRepo)
	creditsCtrl := creditsController.NewController(creditsSvc)

	invoicesSvc := invoicesService.NewService(dodoClient, invoiceRepo, orgRepo, planRepo)
	invoicesCtrl := invoicesController.NewController(invoicesSvc)

	billingService.StartHoldReclaimer(ctx, billingRepo)

	healthSvc := healthService.NewService(db)
	healthCtrl := healthController.NewController(healthSvc)
	handler := routes.New(cfg, healthCtrl, userCtrl, catalogueCtrl, apiKeyCtrl, orgCtrl, inferenceCtrl, openaiCompatCtrl, anthropicCompatCtrl, planCtrl, checkoutCtrl, creditsCtrl, invoicesCtrl, webhookCtrl, apiKeyAuth, clerkAuth)

	addr := ":" + cfg.Port
	logger.Info(ctx, "orbit.api http listening", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Fatal(ctx, "http listen failed", "error", err)
	}
}
