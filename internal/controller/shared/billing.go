// Package shared holds small helpers reused across the native
// /models/{id}/chat controller and the OpenAI/Anthropic compat
// controllers, so all three inference entry points behave identically
// for concerns that have nothing to do with wire-format translation.
package shared

import (
	"context"
	"fmt"

	"github.com/shivang-16/orbit.api/internal/logger"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

// RecordUsage enqueues a billing job for one completed inference call and
// logs the outcome. errBody summarizes a provider-side failure for the
// billing job's Error field (ignored when result reflects success); pass
// "stream interrupted" when result.Streamed is true and failed, since
// there's no buffered body left to summarize at that point.
func RecordUsage(ctx context.Context, billing billingService.Enqueuer, logTag string, result *inferenceService.ChatResult, prompt string, errBody string) {
	orgID, _ := apikeyMiddleware.OrganizationID(ctx)
	apiKeyID, _ := apikeyMiddleware.APIKeyID(ctx)

	status := "success"
	errText := ""
	// Cancelled streams return 200 with tokens already received.
	// Provider failures return a non-2xx StatusCode (Cancelled is
	// cleared in that case), so this stays the single billing switch.
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		status = "error"
		errText = truncate(errBody, 500)
		logger.Error(ctx, "inference provider error",
			"source", logTag,
			"org_id", orgID,
			"model", result.ModelCatalogueID,
			"status", result.StatusCode,
			"streamed", result.Streamed,
			"error", errText,
		)
	}

	if billing == nil {
		logger.Error(ctx, "inference: billing enqueuer is nil — usage not recorded",
			"source", logTag,
			"org_id", orgID,
			"model", result.ModelCatalogueID,
		)
		return
	}

	logger.Info(ctx, fmt.Sprintf("inference: enqueue billing in=%d out=%d", result.InputTokens, result.OutputTokens),
		"source", logTag,
		"org_id", orgID,
		"api_key_id", apiKeyID,
		"model", result.ModelCatalogueID,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"latency_ms", result.LatencyMS,
		"status", status,
		"hold_id", result.HoldID,
	)

	billing.Enqueue(billingService.Job{
		IdempotencyKey:   billingService.NewIdempotencyKey(),
		OrganizationID:   orgID,
		APIKeyID:         apiKeyID,
		ModelCatalogueID: result.ModelCatalogueID,
		Prompt:           prompt,
		InputTokens:      result.InputTokens,
		OutputTokens:     result.OutputTokens,
		LatencyMS:        result.LatencyMS,
		Status:           status,
		Error:            errText,
		HoldID:           result.HoldID,
	})
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}
