package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/shivang-16/orbit.api/internal/config"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	userController "github.com/shivang-16/orbit.api/internal/controller/user"
	"github.com/shivang-16/orbit.api/internal/infra/clerk"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	userRepository "github.com/shivang-16/orbit.api/internal/repositories/user"
	"github.com/shivang-16/orbit.api/internal/routes"
	healthService "github.com/shivang-16/orbit.api/internal/services/health"
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

	healthSvc := healthService.NewService(db)
	healthCtrl := healthController.NewController(healthSvc)
	handler := routes.New(cfg, healthCtrl, userCtrl)

	addr := ":" + cfg.Port
	log.Printf("orbit.api listening on %s (%s)", addr, cfg.Env)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
