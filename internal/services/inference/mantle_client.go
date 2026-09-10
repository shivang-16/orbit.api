package inference

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shivang-16/orbit.api/internal/logger"
	"github.com/shivang-16/orbit.api/internal/model"
)

func (s *Service) callMantle(ctx context.Context, entry *model.ModelCatalogue, req ConverseRequest, sink StreamSink, w http.ResponseWriter) (*ChatResult, error) {
	payload, err := responsesBody(entry.ModelID, req)
	if err != nil {
		return nil, fmt.Errorf("encode mantle request: %w", err)
	}
	logger.Info(ctx, "inference: mantle responses",
		"model", responsesModelID(entry.ModelID),
		"slug", entry.Slug,
		"stream", req.Stream,
	)
	if req.Stream {
		if sink == nil {
			flusher, _ := w.(http.Flusher)
			sink = &passthroughSink{w: w, flusher: flusher}
		}
		return s.mantleStream(ctx, entry, payload, sink, w)
	}
	return s.mantleOnce(ctx, entry, payload)
}

func (s *Service) mantleEndpoint(modelID string) string {
	// GPT-5.x is served on bedrock-mantle. GPT-6 Astra's Global/Geo CRIS
	// profiles are on bedrock-runtime's OpenAI Responses API; Mantle only
	// has the in-region foundation id in us-west-2.
	if isGPT6(modelID) {
		return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/openai/v1/responses", s.bedrockRegion)
	}
	return fmt.Sprintf("https://bedrock-mantle.%s.api.aws/openai/v1/responses", s.bedrockRegion)
}

func (s *Service) mantleOnce(ctx context.Context, entry *model.ModelCatalogue, payload []byte) (*ChatResult, error) {
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, s.mantleEndpoint(entry.ModelID), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build mantle request: %w", err)
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Authorization", "Bearer "+s.bedrockAPIKey)

	started := time.Now()
	resp, err := s.httpClient.Do(upstream)
	if err != nil {
		return nil, fmt.Errorf("call mantle: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read mantle response: %w", err)
	}
	latencyMS := int(time.Since(started).Milliseconds())

	if resp.StatusCode != http.StatusOK {
		return &ChatResult{
			StatusCode:       resp.StatusCode,
			Body:             normalizeProviderError(body),
			ModelCatalogueID: entry.ID,
			ModelSlug:        entry.Slug,
			LatencyMS:        latencyMS,
		}, nil
	}

	converseBody, inputTokens, outputTokens, err := responsesToConverseJSON(body, latencyMS)
	if err != nil {
		logger.Error(ctx, "inference: parse mantle response", "slug", entry.Slug, "error", err)
		var failed *responsesStatusError
		if errors.As(err, &failed) {
			inputTokens = failed.InputTokens
			outputTokens = failed.OutputTokens
		}
		return &ChatResult{
			StatusCode:       http.StatusBadGateway,
			Body:             failedProviderBody(err),
			ModelCatalogueID: entry.ID,
			ModelSlug:        entry.Slug,
			InputTokens:      inputTokens,
			OutputTokens:     outputTokens,
			LatencyMS:        latencyMS,
		}, nil
	}

	return &ChatResult{
		StatusCode:       http.StatusOK,
		Body:             converseBody,
		ModelCatalogueID: entry.ID,
		ModelSlug:        entry.Slug,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		LatencyMS:        latencyMS,
	}, nil
}

func (s *Service) mantleStream(ctx context.Context, entry *model.ModelCatalogue, payload []byte, sink StreamSink, w http.ResponseWriter) (*ChatResult, error) {
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, s.mantleEndpoint(entry.ModelID), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build mantle request: %w", err)
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Accept", "text/event-stream")
	upstream.Header.Set("Authorization", "Bearer "+s.bedrockAPIKey)

	started := time.Now()
	resp, err := s.httpClient.Do(upstream)
	if err != nil {
		return nil, fmt.Errorf("call mantle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read mantle error response: %w", err)
		}
		return &ChatResult{
			StatusCode:       resp.StatusCode,
			Body:             normalizeProviderError(body),
			ModelCatalogueID: entry.ID,
			ModelSlug:        entry.Slug,
			LatencyMS:        int(time.Since(started).Milliseconds()),
		}, nil
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// See chatStream: keeps an nginx/CDN hop from re-buffering our flushes.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	inputTokens, outputTokens, streamErr, cancelled := relayResponsesStream(ctx, resp.Body, sink)
	latencyMS := int(time.Since(started).Milliseconds())

	status := http.StatusOK
	if streamErr {
		status = http.StatusBadGateway
		cancelled = false
	}

	return &ChatResult{
		StatusCode:       status,
		Streamed:         true,
		ModelCatalogueID: entry.ID,
		ModelSlug:        entry.Slug,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		LatencyMS:        latencyMS,
		Cancelled:        cancelled,
	}, nil
}
