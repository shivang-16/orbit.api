package main

import (
	"context"
	"flag"
	"log"

	"github.com/shivang-16/orbit.api/cmd/billing"
	httpserver "github.com/shivang-16/orbit.api/cmd/http"
	"github.com/shivang-16/orbit.api/internal/config"
)

func main() {
	mode := flag.String("mode", "http", "http or billing")
	flag.Parse()

	cfg := config.Load()
	ctx := context.Background()

	switch *mode {
	case "http":
		httpserver.Start(ctx, cfg)
	case "billing":
		billing.Start(ctx, cfg)
	default:
		log.Fatalf("unknown mode %q (use --mode=http or --mode=billing)", *mode)
	}
}
