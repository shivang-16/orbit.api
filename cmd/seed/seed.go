package main

import (
	"context"
	"log"
	"time"

	"github.com/lib/pq"

	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
)

type catalogueModel struct {
	Name              string
	Vendor            string
	Provider          string
	ModelID           string
	InputContextLimit int
	SortOrder         int
	Tags              []string
	Modalities        []string
	IsActive          bool
}

// Add models here (latest → oldest within each vendor), then run:
//
//	go run ./cmd/seed
//
// Seed deletes that vendor's existing rows, then inserts this list in order.
var models = []catalogueModel{
	{
		Name:              "Claude Opus 5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-5",
		InputContextLimit: 200000,
		SortOrder:         1,
		Tags:              []string{"flagship", "reasoning", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Sonnet 5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-5",
		InputContextLimit: 200000,
		SortOrder:         2,
		Tags:              []string{"balanced", "agentic", "coding"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Fable 5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-fable-5",
		InputContextLimit: 200000,
		SortOrder:         3,
		Tags:              []string{"creative-writing", "balanced"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Opus 4.8",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-8",
		InputContextLimit: 200000,
		SortOrder:         4,
		Tags:              []string{"flagship", "reasoning"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Opus 4.7",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-7",
		InputContextLimit: 200000,
		SortOrder:         5,
		Tags:              []string{"flagship", "reasoning"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Sonnet 4.6",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-6",
		InputContextLimit: 200000,
		SortOrder:         6,
		Tags:              []string{"balanced", "coding"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Opus 4.6",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-6-v1",
		InputContextLimit: 200000,
		SortOrder:         7,
		Tags:              []string{"flagship", "reasoning"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Opus 4.5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-5-20251101-v1:0",
		InputContextLimit: 200000,
		SortOrder:         8,
		Tags:              []string{"flagship", "reasoning"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Haiku 4.5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-haiku-4-5-20251001-v1:0",
		InputContextLimit: 200000,
		SortOrder:         9,
		Tags:              []string{"fast", "lightweight", "cost-efficient"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Claude Sonnet 4.5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		InputContextLimit: 200000,
		SortOrder:         10,
		Tags:              []string{"balanced", "coding"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "GPT 5.6 Luna",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-luna",
		InputContextLimit: 200000,
		SortOrder:         1,
		Tags:              []string{"fast", "lightweight"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "GPT 5.6 Sol",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-sol",
		InputContextLimit: 200000,
		SortOrder:         2,
		Tags:              []string{"balanced", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "GPT 5.6 Terra",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-terra",
		InputContextLimit: 200000,
		SortOrder:         3,
		Tags:              []string{"flagship", "reasoning"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "GPT OSS Safeguard 120B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-safeguard-120b",
		InputContextLimit: 200000,
		SortOrder:         4,
		Tags:              []string{"open-source", "safety"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "GPT OSS Safeguard 20B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-safeguard-20b",
		InputContextLimit: 200000,
		SortOrder:         5,
		Tags:              []string{"open-source", "safety", "lightweight"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "GPT OSS 120B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-120b-1:0",
		InputContextLimit: 200000,
		SortOrder:         6,
		Tags:              []string{"open-source", "balanced"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "GPT OSS 20B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-20b-1:0",
		InputContextLimit: 200000,
		SortOrder:         7,
		Tags:              []string{"open-source", "lightweight", "cost-efficient"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Kimi K2.5",
		Vendor:            "moonshot",
		Provider:          "bedrock",
		ModelID:           "moonshotai.kimi-k2.5",
		InputContextLimit: 200000,
		SortOrder:         1,
		Tags:              []string{"flagship", "balanced", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Kimi K2 Thinking",
		Vendor:            "moonshot",
		Provider:          "bedrock",
		ModelID:           "moonshot.kimi-k2-thinking",
		InputContextLimit: 200000,
		SortOrder:         2,
		Tags:              []string{"reasoning", "thinking"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
}

func main() {
	if len(models) == 0 {
		log.Print("no models in seed.go — nothing to insert")
		return
	}

	cfg := config.Load()
	db, err := postgres.Open(cfg.Postgres)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vendors := map[string]struct{}{}
	for _, model := range models {
		vendors[model.Vendor] = struct{}{}
	}
	for vendor := range vendors {
		res, err := db.DB().ExecContext(ctx, `DELETE FROM model_catalogue WHERE vendor = $1`, vendor)
		if err != nil {
			log.Fatalf("delete %s: %v", vendor, err)
		}
		n, _ := res.RowsAffected()
		log.Printf("deleted %d existing %s row(s)", n, vendor)
	}

	const sql = `
		INSERT INTO model_catalogue (name, vendor, provider, model_id, input_context_limit, sort_order, tags, modalities, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, name, vendor, sort_order
	`

	for _, model := range models {
		var id, name, vendor string
		var sortOrder int
		err := db.DB().QueryRowContext(
			ctx,
			sql,
			model.Name,
			model.Vendor,
			model.Provider,
			model.ModelID,
			model.InputContextLimit,
			model.SortOrder,
			pq.Array(model.Tags),
			pq.Array(model.Modalities),
			model.IsActive,
		).Scan(&id, &name, &vendor, &sortOrder)
		if err != nil {
			log.Fatalf("insert %s: %v", model.Name, err)
		}
		log.Printf("inserted %s [%s #%d] → %s", name, vendor, sortOrder, id)
	}
}
