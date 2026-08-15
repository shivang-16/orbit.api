package main

import (
	"log"
	"net/http"

	"github.com/shivang-16/orbit.api/internal/config"
	healthController "github.com/shivang-16/orbit.api/internal/controller/health"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	"github.com/shivang-16/orbit.api/internal/routes"
	healthService "github.com/shivang-16/orbit.api/internal/services/health"
)

func main() {
	cfg := config.Load()

	db, err := postgres.Open(cfg.Postgres)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	healthSvc := healthService.NewService(db)
	healthCtrl := healthController.NewController(healthSvc)
	handler := routes.New(cfg, healthCtrl)

	addr := ":" + cfg.Port
	log.Printf("orbit.api listening on %s (%s)", addr, cfg.Env)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
