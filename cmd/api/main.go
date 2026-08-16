package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/shivang-16/orbit.api/internal/config"
	apikeyController "github.com/shivang-16/orbit.api/internal/controller/apikey"
	catalogueController "github.com/shivang-16/orbit.api/internal/controller/catalogue"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	inferenceController "github.com/shivang-16/orbit.api/internal/controller/inference"
	organizationController "github.com/shivang-16/orbit.api/internal/controller/organization"
	userController "github.com/shivang-16/orbit.api/internal/controller/user"
	"github.com/shivang-16/orbit.api/internal/infra/clerk"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	apikeyRepository "github.com/shivang-16/orbit.api/internal/repositories/apikey"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
	"github.com/shivang-16/orbit.api/internal/routes"
	apikeyService "github.com/shivang-16/orbit.api/internal/services/apikey"
	catalogueService "github.com/shivang-16/orbit.api/internal/services/catalogue"
	healthService "github.com/shivang-16/orbit.api/internal/services/health"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
	organizationService "github.com/shivang-16/orbit.api/internal/services/organization"
	userService "github.com/shivang-16/orbit.api/internal/services/user"
)

func main() {
	cfg := config.Load()
	if cfg.ClerkSecretKey == "" {
		log.Fatal("CLERK_SECRET_KEY is required")
	}

	db, err := postgres.Open(cfg.Postgres)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.Migrate(ctx, "migrations"); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	clerkClient := clerk.New(cfg.ClerkSecretKey)
	userRepo := userRepository.NewRepository(db.DB())
	orgRepo := organizationRepository.NewRepository(db.DB())
	userSvc := userService.NewService(db.DB(), userRepo, orgRepo, clerkClient)
	userCtrl := userController.NewController(userSvc)

	catalogueRepo := catalogueRepository.NewRepository(db.DB())
	catalogueSvc := catalogueService.NewService(catalogueRepo)
	catalogueCtrl := catalogueController.NewController(catalogueSvc)

	apiKeyRepo := apikeyRepository.NewRepository(db.DB())
	apiKeySvc := apikeyService.NewService(apiKeyRepo, orgRepo)
	apiKeyCtrl := apikeyController.NewController(apiKeySvc)

	orgSvc := organizationService.NewService(orgRepo)
	orgCtrl := organizationController.NewController(orgSvc)

	inferenceSvc := inferenceService.NewService(catalogueRepo, cfg.AWSBedrockAPIKey, cfg.AWSBedrockRegion)
	inferenceCtrl := inferenceController.NewController(inferenceSvc)
	apiKeyAuth := apikeyMiddleware.New(apiKeyRepo)

	healthSvc := healthService.NewService(db)
	healthCtrl := healthController.NewController(healthSvc)
	handler := routes.New(cfg, healthCtrl, userCtrl, catalogueCtrl, apiKeyCtrl, orgCtrl, inferenceCtrl, apiKeyAuth)

	addr := ":" + cfg.Port
	log.Printf("orbit.api listening on %s (%s)", addr, cfg.Env)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
