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
		"model", mantleModelID(entry.ModelID),
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

func (s *Service) mantleEndpoint() string {
	// GPT-5.x on mantle is served at /openai/v1/responses, not the
	// default /v1/responses used by gpt-oss. See the Sol/Terra/Luna model cards.
	return fmt.Sprintf("https://bedrock-mantle.%s.api.aws/openai/v1/responses", s.bedrockRegion)
}

func (s *Service) mantleOnce(ctx context.Context, entry *model.ModelCatalogue, payload []byte) (*ChatResult, error) {
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, s.mantleEndpoint(), bytes.NewReader(payload))
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
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, s.mantleEndpoint(), bytes.NewReader(payload))
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
	w.WriteHeader(http.StatusOK)

	inputTokens, outputTokens, streamErr := relayResponsesStream(ctx, resp.Body, sink)
	latencyMS := int(time.Since(started).Milliseconds())

	status := http.StatusOK
	if streamErr {
		status = http.StatusBadGateway
	}

	return &ChatResult{
		StatusCode:       status,
		Streamed:         true,
		ModelCatalogueID: entry.ID,
		ModelSlug:        entry.Slug,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		LatencyMS:        latencyMS,
	}, nil
}
