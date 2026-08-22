package main

import (
	"context"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
)

// catalogueModel is the curated, hand-researched half of a catalogue row.
// The remaining tags (tier, long-context, vision, audio, cost-efficient)
// are derived at insert time — see buildTags — from data we already store,
// so they can't drift out of sync with reality the way a fully static tag
// list can.
type catalogueModel struct {
	Name              string
	Vendor            string
	Provider          string
	ModelID           string
	InputContextLimit int
	SortOrder         int

	// Tier is exactly one of "flagship" | "balanced" | "lightweight",
	// reflecting where the vendor itself positions this model in its
	// current lineup (e.g. Anthropic's own Opus/Sonnet/Haiku tiering,
	// Mistral's Large/Devstral/Ministral branding), cross-checked against
	// relative Bedrock pricing within the same vendor so a cheaper sibling
	// never outranks a pricier one.
	Tier string

	// ExtraTags are curated capability/economics tags that aren't
	// mechanically derivable from stored columns: "reasoning" (documented
	// extended-thinking/CoT mode), "coding", "agentic" (tool-use/multi-step
	// agent workflows), "open-source" (genuinely open-weight, not just
	// "available on Bedrock"), "safety" (guardrail/moderation models), and
	// "fast" (vendor's latency-optimized variant). Tier, long-context,
	// vision, audio and cost-efficient are computed — do not repeat them
	// here.
	ExtraTags []string

	Modalities []string
	IsActive   bool

	// ReleasedDate is the model's real public release date (vendor
	// announcement, model card, or changelog), YYYY-MM-DD. Where only the
	// release month is confirmable, this uses the 1st of that month.
	ReleasedDate string
}

// Bedrock on-demand list prices in USD micros per 1 million tokens
// (1_000_000 = $1.00). Region: us-east-1. Credits are charged at these rates.
type modelPrice struct {
	VendorInputPerMillionMicros  int64
	VendorOutputPerMillionMicros int64
}

const longContextTokens = 200_000

// blendedMicros approximates real-world spend per model by weighting
// output tokens 3x input tokens (typical chat/agent traffic is
// output-heavy). Used only to rank a vendor's own models against each
// other for the "cost-efficient" tag — never compared across vendors.
func blendedMicros(p modelPrice) int64 {
	return (p.VendorInputPerMillionMicros + 3*p.VendorOutputPerMillionMicros) / 4
}

// costEfficientSet returns the set of model names that rank in the
// cheapest 40% (rounded up, minimum one) of their own vendor's lineup by
// blended price. "Cost-efficient" is relative to siblings from the same
// lab, not the whole catalogue — a cheap-for-Anthropic model can cost more
// than an expensive-for-Mistral one.
func costEfficientSet(models []catalogueModel, prices map[string]modelPrice) map[string]bool {
	byVendor := map[string][]catalogueModel{}
	for _, m := range models {
		byVendor[m.Vendor] = append(byVendor[m.Vendor], m)
	}

	result := map[string]bool{}
	for _, group := range byVendor {
		sort.SliceStable(group, func(i, j int) bool {
			return blendedMicros(prices[group[i].Name]) < blendedMicros(prices[group[j].Name])
		})
		cutoff := int(math.Ceil(float64(len(group)) * 0.4))
		if cutoff < 1 {
			cutoff = 1
		}
		for i := 0; i < cutoff && i < len(group); i++ {
			result[group[i].Name] = true
		}
	}
	return result
}

// buildTags assembles a model's final tag list: the curated Tier, tags
// mechanically derived from stored columns (long-context, vision, audio),
// the computed cost-efficient flag, then the curated ExtraTags — deduped,
// tier first.
func buildTags(m catalogueModel, costEfficient bool) []string {
	seen := map[string]bool{}
	var tags []string
	add := func(tag string) {
		if tag == "" || seen[tag] {
			return
		}
		seen[tag] = true
		tags = append(tags, tag)
	}

	add(m.Tier)
	if m.InputContextLimit >= longContextTokens {
		add("long-context")
	}
	for _, modality := range m.Modalities {
		switch modality {
		case "image":
			add("vision")
		case "audio":
			add("audio")
		}
	}
	if costEfficient {
		add("cost-efficient")
	}
	for _, tag := range m.ExtraTags {
		add(tag)
	}
	return tags
}

// Add models here (latest → oldest within each vendor), then run:
//
//	go run ./cmd/seed
//
// Seed deletes that vendor's existing rows, then inserts this list in
// order. InputContextLimit is the Bedrock model-card context window in
// tokens (AWS docs, Aug 2026). ReleasedDate and Tier/ExtraTags are
// researched against each vendor's own announcement/model-card/changelog
// (see PR history for sources) — see buildTags for how the full tag list
// is assembled.
var models = []catalogueModel{
	{
		Name:              "Claude Opus 5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-5",
		InputContextLimit: 1_000_000,
		SortOrder:         1,
		Tier:              "flagship",
		ExtraTags:         []string{"reasoning", "coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-07-24",
	},
	{
		Name:              "Claude Sonnet 5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-5",
		InputContextLimit: 1_000_000,
		SortOrder:         2,
		Tier:              "balanced",
		ExtraTags:         []string{"coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-06-30",
	},
	{
		Name:              "Claude Fable 5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-fable-5",
		InputContextLimit: 1_000_000,
		SortOrder:         3,
		Tier:              "flagship",
		ExtraTags:         []string{"reasoning", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-06-09",
	},
	{
		Name:              "Claude Opus 4.8",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-8",
		InputContextLimit: 1_000_000,
		SortOrder:         4,
		Tier:              "flagship",
		ExtraTags:         []string{"reasoning", "coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-05-28",
	},
	{
		Name:              "Claude Opus 4.7",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-7",
		InputContextLimit: 1_000_000,
		SortOrder:         5,
		Tier:              "flagship",
		ExtraTags:         []string{"reasoning", "coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-04-16",
	},
	{
		Name:              "Claude Sonnet 4.6",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-6",
		InputContextLimit: 1_000_000,
		SortOrder:         6,
		Tier:              "balanced",
		ExtraTags:         []string{"coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-02-17",
	},
	{
		Name:              "Claude Opus 4.6",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-6-v1",
		InputContextLimit: 1_000_000,
		SortOrder:         7,
		Tier:              "flagship",
		ExtraTags:         []string{"reasoning", "coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-02-05",
	},
	{
		Name:              "Claude Opus 4.5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-opus-4-5-20251101-v1:0",
		InputContextLimit: 200_000,
		SortOrder:         8,
		Tier:              "flagship",
		ExtraTags:         []string{"reasoning", "coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-11-24",
	},
	{
		Name:              "Claude Haiku 4.5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-haiku-4-5-20251001-v1:0",
		InputContextLimit: 200_000,
		SortOrder:         9,
		Tier:              "lightweight",
		ExtraTags:         []string{"fast"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-10-15",
	},
	{
		Name:              "Claude Sonnet 4.5",
		Vendor:            "anthropic",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-4-5-20250929-v1:0",
		InputContextLimit: 200_000,
		SortOrder:         10,
		Tier:              "balanced",
		ExtraTags:         []string{"coding", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-09-29",
	},
	{
		Name:              "GPT 5.6 Luna",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-luna",
		InputContextLimit: 1_000_000,
		SortOrder:         1,
		Tier:              "lightweight",
		ExtraTags:         []string{"fast"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-07-09",
	},
	{
		Name:              "GPT 5.6 Sol",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-sol",
		InputContextLimit: 1_000_000,
		SortOrder:         2,
		Tier:              "flagship",
		ExtraTags:         []string{"reasoning", "agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-07-09",
	},
	{
		Name:              "GPT 5.6 Terra",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-terra",
		InputContextLimit: 1_000_000,
		SortOrder:         3,
		Tier:              "balanced",
		ExtraTags:         []string{"agentic"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2026-07-09",
	},
	{
		Name:              "GPT OSS Safeguard 120B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-safeguard-120b",
		InputContextLimit: 128_000,
		SortOrder:         4,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "safety", "reasoning"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-10-29",
	},
	{
		Name:              "GPT OSS Safeguard 20B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-safeguard-20b",
		InputContextLimit: 128_000,
		SortOrder:         5,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "safety", "reasoning"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-10-29",
	},
	{
		Name:              "GPT OSS 120B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-120b-1:0",
		InputContextLimit: 128_000,
		SortOrder:         6,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "reasoning", "agentic"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-08-05",
	},
	{
		Name:              "GPT OSS 20B",
		Vendor:            "openai",
		Provider:          "bedrock",
		ModelID:           "openai.gpt-oss-20b-1:0",
		InputContextLimit: 128_000,
		SortOrder:         7,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "reasoning", "fast"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-08-05",
	},
	{
		Name:              "Kimi K2.5",
		Vendor:            "moonshot",
		Provider:          "bedrock",
		ModelID:           "moonshotai.kimi-k2.5",
		InputContextLimit: 256_000,
		SortOrder:         1,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source", "agentic"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2026-01-27",
	},
	{
		Name:              "Kimi K2 Thinking",
		Vendor:            "moonshot",
		Provider:          "bedrock",
		ModelID:           "moonshot.kimi-k2-thinking",
		InputContextLimit: 256_000,
		SortOrder:         2,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "reasoning", "agentic"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-11-06",
	},
	{
		Name:              "DeepSeek V3.2",
		Vendor:            "deepseek",
		Provider:          "bedrock",
		ModelID:           "deepseek.v3.2",
		InputContextLimit: 164_000,
		SortOrder:         1,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "reasoning", "coding", "agentic"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-12-01",
	},
	{
		Name:              "DeepSeek V3.1",
		Vendor:            "deepseek",
		Provider:          "bedrock",
		ModelID:           "deepseek.v3-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         2,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-08-21",
	},
	{
		Name:              "DeepSeek R1",
		Vendor:            "deepseek",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.deepseek.r1-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source", "reasoning", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-01-20",
	},
	{
		Name:              "Mistral Large 3",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-large-3-675b-instruct",
		InputContextLimit: 256_000,
		SortOrder:         1,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source", "coding"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-12-02",
	},
	{
		Name:              "Devstral 2 123B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.devstral-2-123b",
		InputContextLimit: 256_000,
		SortOrder:         2,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "coding", "agentic"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-12-09",
	},
	{
		Name:              "Magistral Small 2509",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.magistral-small-2509",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "reasoning"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-09-01",
	},
	{
		Name:              "Ministral 3 14B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.ministral-3-14b-instruct",
		InputContextLimit: 128_000,
		SortOrder:         4,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-12-02",
	},
	{
		Name:              "Ministral 3 8B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.ministral-3-8b-instruct",
		InputContextLimit: 128_000,
		SortOrder:         5,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-12-02",
	},
	{
		Name:              "Ministral 3 3B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.ministral-3-3b-instruct",
		InputContextLimit: 128_000,
		SortOrder:         6,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "fast"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-12-02",
	},
	{
		Name:              "Voxtral Small 24B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.voxtral-small-24b-2507",
		InputContextLimit: 32_000,
		SortOrder:         7,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "audio"},
		IsActive:          true,
		ReleasedDate:      "2025-07-15",
	},
	{
		Name:              "Voxtral Mini 3B",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.voxtral-mini-3b-2507",
		InputContextLimit: 32_000,
		SortOrder:         8,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "fast"},
		Modalities:        []string{"text", "audio"},
		IsActive:          true,
		ReleasedDate:      "2025-07-15",
	},
	{
		Name:              "Pixtral Large",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.mistral.pixtral-large-2502-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         9,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2024-11-18",
	},
	{
		Name:              "Mistral Large 2402",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-large-2402-v1:0",
		InputContextLimit: 32_000,
		SortOrder:         10,
		Tier:              "flagship",
		ExtraTags:         nil,
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2024-02-26",
	},
	{
		Name:              "Mistral Small 2402",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-small-2402-v1:0",
		InputContextLimit: 32_000,
		SortOrder:         11,
		Tier:              "balanced",
		ExtraTags:         []string{"fast"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2024-02-26",
	},
	{
		Name:              "Mixtral 8x7B Instruct",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mixtral-8x7b-instruct-v0:1",
		InputContextLimit: 32_000,
		SortOrder:         12,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2023-12-11",
	},
	{
		Name:              "Mistral 7B Instruct",
		Vendor:            "mistral",
		Provider:          "bedrock",
		ModelID:           "mistral.mistral-7b-instruct-v0:2",
		InputContextLimit: 32_000,
		SortOrder:         13,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "fast"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2023-09-27",
	},
	{
		Name:              "Gemma 3 27B",
		Vendor:            "google",
		Provider:          "bedrock",
		ModelID:           "google.gemma-3-27b-it",
		InputContextLimit: 128_000,
		SortOrder:         1,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-03-12",
	},
	{
		Name:              "Gemma 3 12B",
		Vendor:            "google",
		Provider:          "bedrock",
		ModelID:           "google.gemma-3-12b-it",
		InputContextLimit: 128_000,
		SortOrder:         2,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-03-12",
	},
	{
		Name:              "Gemma 3 4B",
		Vendor:            "google",
		Provider:          "bedrock",
		ModelID:           "google.gemma-3-4b-it",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "fast"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-03-12",
	},
	{
		Name:              "Llama 4 Maverick 17B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-maverick-17b-instruct-v1:0",
		InputContextLimit: 1_000_000,
		SortOrder:         1,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-04-05",
	},
	{
		Name:              "Llama 4 Scout 17B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama4-scout-17b-instruct-v1:0",
		InputContextLimit: 3_500_000,
		SortOrder:         2,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-04-05",
	},
	{
		Name:              "Llama 3.3 70B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-3-70b-instruct-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         3,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2024-12-06",
	},
	{
		Name:              "Llama 3.1 70B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-70b-instruct-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         4,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2024-07-23",
	},
	{
		Name:              "Llama 3.1 8B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.meta.llama3-1-8b-instruct-v1:0",
		InputContextLimit: 128_000,
		SortOrder:         5,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "fast"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2024-07-23",
	},
	{
		Name:              "Llama 3 70B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "meta.llama3-70b-instruct-v1:0",
		InputContextLimit: 8_000,
		SortOrder:         6,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2024-04-18",
	},
	{
		Name:              "Llama 3 8B",
		Vendor:            "meta",
		Provider:          "bedrock",
		ModelID:           "meta.llama3-8b-instruct-v1:0",
		InputContextLimit: 8_000,
		SortOrder:         7,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "fast"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2024-04-18",
	},
	{
		Name:              "MiniMax M2.5",
		Vendor:            "minimax",
		Provider:          "bedrock",
		ModelID:           "minimax.minimax-m2.5",
		InputContextLimit: 196_000,
		SortOrder:         1,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source", "agentic", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2026-02-12",
	},
	{
		Name:              "MiniMax M2.1",
		Vendor:            "minimax",
		Provider:          "bedrock",
		ModelID:           "minimax.minimax-m2.1",
		InputContextLimit: 196_000,
		SortOrder:         2,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "agentic", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-12-22",
	},
	{
		Name:              "MiniMax M2",
		Vendor:            "minimax",
		Provider:          "bedrock",
		ModelID:           "minimax.minimax-m2",
		InputContextLimit: 1_000_000,
		SortOrder:         3,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "agentic", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-10-27",
	},
	{
		Name:              "Qwen3 Coder Next",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-coder-next",
		InputContextLimit: 256_000,
		SortOrder:         1,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source", "coding", "agentic"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2026-02-03",
	},
	{
		Name:              "Qwen3 VL 235B A22B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-vl-235b-a22b",
		InputContextLimit: 256_000,
		SortOrder:         2,
		Tier:              "flagship",
		ExtraTags:         []string{"open-source"},
		Modalities:        []string{"text", "image"},
		IsActive:          true,
		ReleasedDate:      "2025-09-23",
	},
	{
		Name:              "Qwen3 Next 80B A3B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-next-80b-a3b",
		InputContextLimit: 256_000,
		SortOrder:         3,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "reasoning"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-09-11",
	},
	{
		Name:              "Qwen3 Coder 30B A3B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-coder-30b-a3b-v1:0",
		InputContextLimit: 256_000,
		SortOrder:         4,
		Tier:              "balanced",
		ExtraTags:         []string{"open-source", "coding"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-07-22",
	},
	{
		Name:              "Qwen3 32B",
		Vendor:            "qwen",
		Provider:          "bedrock",
		ModelID:           "qwen.qwen3-32b-v1:0",
		InputContextLimit: 32_000,
		SortOrder:         5,
		Tier:              "lightweight",
		ExtraTags:         []string{"open-source", "reasoning"},
		Modalities:        []string{"text"},
		IsActive:          true,
		ReleasedDate:      "2025-04-28",
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

	costEfficient := costEfficientSet(models, prices)

	const sql = `
		INSERT INTO model_catalogue (name, slug, vendor, provider, model_id, input_context_limit, sort_order, tags, modalities, is_active, model_released_date)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, name, vendor, sort_order
	`

	for _, model := range models {
		var releasedDate *time.Time
		if model.ReleasedDate != "" {
			parsed, err := time.Parse("2006-01-02", model.ReleasedDate)
			if err != nil {
				log.Fatalf("bad released date for %s: %v", model.Name, err)
			}
			releasedDate = &parsed
		}

		tags := buildTags(model, costEfficient[model.Name])

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
			pq.Array(tags),
			pq.Array(model.Modalities),
			model.IsActive,
			releasedDate,
		).Scan(&id, &name, &vendor, &sortOrder)
		if err != nil {
			log.Fatalf("insert %s: %v", model.Name, err)
		}
		log.Printf("inserted %s [%s #%d] tags=%v → %s", name, vendor, sortOrder, tags, id)

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
