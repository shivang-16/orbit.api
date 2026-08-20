// Package shared holds small helpers reused across the native
// /models/{id}/chat controller and the OpenAI/Anthropic compat
// controllers, so all three inference entry points behave identically
// for concerns that have nothing to do with wire-format translation.
package shared

import (
	"context"
	"log"

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
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		status = "error"
		errText = truncate(errBody, 500)
		log.Printf("%s provider error org=%s model=%s status=%d streamed=%t: %s", logTag, orgID, result.ModelCatalogueID, result.StatusCode, result.Streamed, errText)
	}

	if billing == nil {
		log.Printf("%s: billing enqueuer is nil — usage not recorded org=%s model=%s", logTag, orgID, result.ModelCatalogueID)
		return
	}

	log.Printf(
		"%s: enqueue billing org=%s key=%s model=%s in=%d out=%d latency_ms=%d status=%s hold=%s",
		logTag, orgID, apiKeyID, result.ModelCatalogueID, result.InputTokens, result.OutputTokens, result.LatencyMS, status, result.HoldID,
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
