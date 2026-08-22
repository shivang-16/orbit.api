package main

import (
	"context"
	"flag"

	"github.com/shivang-16/orbit.api/cmd/billing"
	httpserver "github.com/shivang-16/orbit.api/cmd/http"
	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/logger"
)

func main() {
	mode := flag.String("mode", "http", "http or billing")
	flag.Parse()

	cfg := config.Load()
	logger.Init(cfg)
	ctx := context.Background()

	switch *mode {
	case "http":
		httpserver.Start(ctx, cfg)
	case "billing":
		billing.Start(ctx, cfg)
	default:
		logger.Fatal(ctx, "unknown mode", "mode", *mode)
	}
}
