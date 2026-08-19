package main

import (
	"context"
	"log"
	"regexp"
	"strings"
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

// Bedrock on-demand list prices in USD micros per 1 million tokens
// (1_000_000 = $1.00). Region: us-east-1. Credits are charged at these rates.
type modelPrice struct {
	VendorInputPerMillionMicros  int64
	VendorOutputPerMillionMicros int64
}

// Add models here (latest → oldest within each vendor), then run:
//
//	go run ./cmd/seed
//
// Seed deletes that vendor's existing rows, then inserts this list in order.
// InputContextLimit is the Bedrock model-card context window in tokens
// (AWS docs, Aug 2026).
var models = []catalogueModel{
	{
		Name:              "Claude Opus 5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-5",
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 200_000,
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
		InputContextLimit: 200_000,
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
		InputContextLimit: 200_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 1_000_000,
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
		InputContextLimit: 128_000,
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
		InputContextLimit: 128_000,
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
		InputContextLimit: 128_000,
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
		InputContextLimit: 128_000,
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
		InputContextLimit: 256_000,
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
		InputContextLimit: 256_000,
		SortOrder:         2,
		Tags:              []string{"reasoning", "thinking"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "DeepSeek V3.2",
		Vendor:            "deepseek",
		Provider:          "bedrock",
		ModelID:           "deepseek.v3.2",
		InputContextLimit: 164_000,
		SortOrder:         1,
		Tags:              []string{"flagship", "reasoning", "coding", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "DeepSeek V3.1",
		Vendor:            "deepseek",
		Provider:          "bedrock",
		ModelID:           "deepseek.v3-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         2,
		Tags:              []string{"flagship", "reasoning", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "DeepSeek R1",
		Vendor:            "deepseek",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.deepseek.r1-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tags:              []string{"flagship", "reasoning", "thinking", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Mistral Large 3",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-large-3-675b-instruct",
		InputContextLimit: 256_000,
		SortOrder:         1,
		Tags:              []string{"flagship", "reasoning", "coding", "long-context"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Devstral 2 123B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.devstral-2-123b",
		InputContextLimit: 256_000,
		SortOrder:         2,
		Tags:              []string{"coding", "agentic", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Magistral Small 2509",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.magistral-small-2509",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tags:              []string{"reasoning", "thinking"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Ministral 3 14B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.ministral-3-14b-instruct",
		InputContextLimit: 128_000,
		SortOrder:         4,
		Tags:              []string{"balanced", "lightweight"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Ministral 3 8B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.ministral-3-8b-instruct",
		InputContextLimit: 128_000,
		SortOrder:         5,
		Tags:              []string{"lightweight", "cost-efficient"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Ministral 3 3B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.ministral-3-3b-instruct",
		InputContextLimit: 128_000,
		SortOrder:         6,
		Tags:              []string{"lightweight", "fast", "cost-efficient"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Voxtral Small 24B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.voxtral-small-24b-2507",
		InputContextLimit: 32_000,
		SortOrder:         7,
		Tags:              []string{"balanced", "audio"},
		Modalities:        []string{"text", "audio"},
		IsActive:          true,
	},
	{
		Name:              "Voxtral Mini 3B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.voxtral-mini-3b-2507",
		InputContextLimit: 32_000,
		SortOrder:         8,
		Tags:              []string{"lightweight", "fast", "cost-efficient", "audio"},
		Modalities:        []string{"text", "audio"},
		IsActive:          true,
	},
	{
		Name:              "Pixtral Large",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.mistral.pixtral-large-2502-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         9,
		Tags:              []string{"flagship", "vision"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Mistral Large 2402",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-large-2402-v1:0",
		InputContextLimit: 32_000,
		SortOrder:         10,
		Tags:              []string{"flagship", "reasoning"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Mistral Small 2402",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-small-2402-v1:0",
		InputContextLimit: 32_000,
		SortOrder:         11,
		Tags:              []string{"fast", "cost-efficient"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Mixtral 8x7B Instruct",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mixtral-8x7b-instruct-v0:1",
		InputContextLimit: 32_000,
		SortOrder:         12,
		Tags:              []string{"balanced", "open-source"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Mistral 7B Instruct",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-7b-instruct-v0:2",
		InputContextLimit: 32_000,
		SortOrder:         13,
		Tags:              []string{"lightweight", "open-source", "cost-efficient"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Gemma 3 27B",
		Vendor:            "google",
		Provider:          "bedrock",
		ModelID:           "google.gemma-3-27b-it",
		InputContextLimit: 128_000,
		SortOrder:         1,
		Tags:              []string{"flagship", "open-source", "balanced"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Gemma 3 12B",
		Vendor:            "google",
		Provider:          "bedrock",
		ModelID:           "google.gemma-3-12b-it",
		InputContextLimit: 128_000,
		SortOrder:         2,
		Tags:              []string{"balanced", "open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Gemma 3 4B",
		Vendor:            "google",
		Provider:          "bedrock",
		ModelID:           "google.gemma-3-4b-it",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tags:              []string{"lightweight", "open-source", "fast", "cost-efficient"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Llama 4 Maverick 17B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-maverick-17b-instruct-v1:0",
		InputContextLimit: 1_000_000,
		SortOrder:         1,
		Tags:              []string{"flagship", "balanced", "long-context"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Llama 4 Scout 17B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-scout-17b-instruct-v1:0",
		InputContextLimit: 10_000_000,
		SortOrder:         2,
		Tags:              []string{"long-context", "cost-efficient"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Llama 3.3 70B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-3-70b-instruct-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tags:              []string{"balanced", "reasoning"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Llama 3.1 70B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-70b-instruct-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         4,
		Tags:              []string{"balanced"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Llama 3.1 8B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-8b-instruct-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         5,
		Tags:              []string{"lightweight", "fast", "cost-efficient"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Llama 3 70B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "meta.llama3-70b-instruct-v1:0",
		InputContextLimit: 8_000,
		SortOrder:         6,
		Tags:              []string{"balanced"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Llama 3 8B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "meta.llama3-8b-instruct-v1:0",
		InputContextLimit: 8_000,
		SortOrder:         7,
		Tags:              []string{"lightweight", "cost-efficient"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "MiniMax M2.5",
		Vendor:            "minimax",
		Provider:          "bedrock",
		ModelID:           "minimax.minimax-m2.5",
		InputContextLimit: 196_000,
		SortOrder:         1,
		Tags:              []string{"flagship", "agentic", "coding", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "MiniMax M2.1",
		Vendor:            "minimax",
		Provider:          "bedrock",
		ModelID:           "minimax.minimax-m2.1",
		InputContextLimit: 196_000,
		SortOrder:         2,
		Tags:              []string{"agentic", "coding", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "MiniMax M2",
		Vendor:            "minimax",
		Provider:          "bedrock",
		ModelID:           "minimax.minimax-m2",
		InputContextLimit: 1_000_000,
		SortOrder:         3,
		Tags:              []string{"agentic", "coding", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Qwen3 Coder Next",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-coder-next",
		InputContextLimit: 256_000,
		SortOrder:         1,
		Tags:              []string{"flagship", "coding", "agentic", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Qwen3 VL 235B A22B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-vl-235b-a22b",
		InputContextLimit: 256_000,
		SortOrder:         2,
		Tags:              []string{"flagship", "vision", "long-context"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
	},
	{
		Name:              "Qwen3 Next 80B A3B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-next-80b-a3b",
		InputContextLimit: 256_000,
		SortOrder:         3,
		Tags:              []string{"balanced", "reasoning", "long-context"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Qwen3 Coder 30B A3B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-coder-30b-a3b-v1:0",
		InputContextLimit: 256_000,
		SortOrder:         4,
		Tags:              []string{"coding", "long-context", "cost-efficient"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
	{
		Name:              "Qwen3 32B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-32b-v1:0",
		InputContextLimit: 32_000,
		SortOrder:         5,
		Tags:              []string{"reasoning", "thinking", "balanced"},
		Modalities:        []string{"text"},
		IsActive:          true,
	},
}

// Source: AWS Bedrock on-demand pricing (us-east-1), Aug 2026.
// https://aws.amazon.com/bedrock/pricing/
var prices = map[string]modelPrice{
	"Claude Opus 5":          {5_000_000, 25_000_000},
	"Claude Sonnet 5":        {2_000_000, 10_000_000},
	"Claude Fable 5":         {10_000_000, 50_000_000},
	"Claude Opus 4.8":        {5_000_000, 25_000_000},
	"Claude Opus 4.7":        {5_000_000, 25_000_000},
	"Claude Sonnet 4.6":      {3_000_000, 15_000_000},
	"Claude Opus 4.6":        {5_000_000, 25_000_000},
	"Claude Opus 4.5":        {5_000_000, 25_000_000},
	"Claude Haiku 4.5":       {1_000_000, 5_000_000},
	"Claude Sonnet 4.5":      {3_000_000, 15_000_000},
	"GPT 5.6 Luna":           {220_000, 1_320_000},
	"GPT 5.6 Sol":            {5_500_000, 33_000_000},
	"GPT 5.6 Terra":          {2_200_000, 13_200_000},
	"GPT OSS Safeguard 120B": {150_000, 600_000},
	"GPT OSS Safeguard 20B":  {70_000, 200_000},
	"GPT OSS 120B":           {150_000, 600_000},
	"GPT OSS 20B":            {70_000, 300_000},
	"Kimi K2.5":              {600_000, 3_000_000},
	"Kimi K2 Thinking":       {600_000, 2_500_000},
	"DeepSeek V3.2":          {620_000, 1_850_000},
	"DeepSeek V3.1":          {620_000, 1_850_000},
	"DeepSeek R1":            {1_350_000, 5_400_000},
	"Mistral Large 3":        {500_000, 1_500_000},
	"Devstral 2 123B":        {400_000, 2_000_000},
	"Magistral Small 2509":   {500_000, 1_500_000},
	"Ministral 3 14B":        {200_000, 200_000},
	"Ministral 3 8B":         {150_000, 150_000},
	"Ministral 3 3B":         {100_000, 100_000},
	"Voxtral Small 24B":      {100_000, 300_000},
	"Voxtral Mini 3B":        {40_000, 40_000},
	"Pixtral Large":          {2_000_000, 6_000_000},
	"Mistral Large 2402":     {4_000_000, 12_000_000},
	"Mistral Small 2402":     {1_000_000, 3_000_000},
	"Mixtral 8x7B Instruct":  {450_000, 700_000},
	"Mistral 7B Instruct":    {150_000, 200_000},
	"Gemma 3 27B":            {230_000, 380_000},
	"Gemma 3 12B":            {90_000, 290_000},
	"Gemma 3 4B":             {40_000, 80_000},
	"Llama 4 Maverick 17B":   {240_000, 970_000},
	"Llama 4 Scout 17B":      {170_000, 660_000},
	"Llama 3.3 70B":          {720_000, 720_000},
	"Llama 3.1 70B":          {720_000, 720_000},
	"Llama 3.1 8B":           {220_000, 220_000},
	"Llama 3 70B":            {2_650_000, 3_500_000},
	"Llama 3 8B":             {300_000, 600_000},
	"MiniMax M2.5":           {300_000, 1_200_000},
	"MiniMax M2.1":           {300_000, 1_200_000},
	"MiniMax M2":             {300_000, 1_200_000},
	"Qwen3 Coder Next":       {500_000, 1_200_000},
	"Qwen3 VL 235B A22B":     {530_000, 2_660_000},
	"Qwen3 Next 80B A3B":     {150_000, 1_200_000},
	"Qwen3 Coder 30B A3B":    {150_000, 600_000},
	"Qwen3 32B":              {150_000, 600_000},
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

	if err := db.Migrate(ctx, "migrations"); err != nil {
		log.Fatalf("migrate: %v", err)
	}

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
		INSERT INTO model_catalogue (name, slug, vendor, provider, model_id, input_context_limit, sort_order, tags, modalities, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, name, vendor, sort_order
	`

	for _, model := range models {
		var id, name, vendor string
		var sortOrder int
		err := db.DB().QueryRowContext(
			ctx,
			sql,
			model.Name,
			slugify(model.Name),
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

		price, ok := prices[model.Name]
		if !ok {
			log.Fatalf("missing Bedrock price for %s", model.Name)
		}
		_, err = db.DB().ExecContext(
			ctx,
			`INSERT INTO model_pricing (
				model_catalogue_id,
				vendor_input_per_million_micros,
				vendor_output_per_million_micros
			) VALUES ($1, $2, $3)`,
			id,
			price.VendorInputPerMillionMicros,
			price.VendorOutputPerMillionMicros,
		)
		if err != nil {
			log.Fatalf("price %s: %v", model.Name, err)
		}
		log.Printf(
			"  vendor $%.4f / $%.4f per 1M tokens",
			float64(price.VendorInputPerMillionMicros)/1_000_000,
			float64(price.VendorOutputPerMillionMicros)/1_000_000,
		)
	}
}

var (
	slugNonAlnumRun = regexp.MustCompile(`[^a-z0-9]+`)
	slugTrimDashes  = regexp.MustCompile(`(^-+)|(-+$)`)
)

// slugify mirrors orbit.web's lib/slug.ts and the SQL backfill in
// migrations/0018_model_catalogue_slug.up.sql, so every insertion path
// produces the same slug for a given name.
func slugify(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	dashed := slugNonAlnumRun.ReplaceAllString(lower, "-")
	return slugTrimDashes.ReplaceAllString(dashed, "")
}
