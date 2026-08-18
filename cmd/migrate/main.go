// Command migrate applies every migrations/*.up.sql file against the
// configured Postgres database and exits. It is meant to run as an
// explicit, fail-fast deploy step (see .github/workflows/deploy.yml) so a
// bad migration stops the deploy before PM2 reloads any process — as
// opposed to relying only on the auto-migrate that cmd/http and
// cmd/billing also run on every boot.
package main

import (
	"context"
	"log"
	"time"

	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
)

func main() {
	cfg := config.Load()

	db, err := postgres.Open(cfg.Postgres)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.Migrate(ctx, "migrations"); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Print("migrations applied")
}
