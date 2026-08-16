// Package inference validates and forwards chat requests made against an
// Orbit API key to the underlying model provider. For now the only
// supported provider is Bedrock, called directly over HTTPS with a Bedrock
// API key (bearer token) rather than SigV4, so no AWS SDK is required.
package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
)

var (
	ErrInvalid             = errors.New("invalid request")
	ErrModelNotFound       = errors.New("model not found")
	ErrUnsupportedProvider = errors.New("model provider not supported yet")
)

// httpClient is shared across every request so TCP/TLS connections to the
// provider are pooled and reused instead of re-established per call, which
// otherwise dominates latency on a hot inference path.
var httpClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	},
}

type Service struct {
	catalogue     *catalogueRepository.Repository
	bedrockAPIKey string
	bedrockRegion string
}

func NewService(catalogue *catalogueRepository.Repository, bedrockAPIKey, bedrockRegion string) *Service {
	return &Service{catalogue: catalogue, bedrockAPIKey: bedrockAPIKey, bedrockRegion: bedrockRegion}
}

type ChatResult struct {
	StatusCode       int
	Body             []byte
	ModelCatalogueID string
	InputTokens      int
	OutputTokens     int
	LatencyMS        int
}

func (s *Service) Chat(ctx context.Context, modelID string, req ChatRequest) (*ChatResult, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}

	entry, err := s.catalogue.GetByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("lookup model: %w", err)
	}
	if entry == nil {
		return nil, ErrModelNotFound
	}
	if entry.Provider != "bedrock" {
		return nil, ErrUnsupportedProvider
	}

	payload, err := bedrockConverseBody(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://bedrock-runtime.%s.amazonaws.com/model/%s/converse",
		s.bedrockRegion,
		url.PathEscape(entry.ModelID),
	)
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Authorization", "Bearer "+s.bedrockAPIKey)

	started := time.Now()
	resp, err := httpClient.Do(upstream)
	if err != nil {
		return nil, fmt.Errorf("call bedrock: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read bedrock response: %w", err)
	}

	inputTokens, outputTokens, latencyMS := parseBedrockUsage(body)
	if latencyMS == 0 {
		latencyMS = int(time.Since(started).Milliseconds())
	}

	return &ChatResult{
		StatusCode:       resp.StatusCode,
		Body:             body,
		ModelCatalogueID: entry.ID,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		LatencyMS:        latencyMS,
	}, nil
}

func parseBedrockUsage(body []byte) (inputTokens, outputTokens, latencyMS int) {
	var parsed struct {
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		} `json:"usage"`
		Metrics struct {
			LatencyMS int `json:"latencyMs"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, 0, 0
	}
	return parsed.Usage.InputTokens, parsed.Usage.OutputTokens, parsed.Metrics.LatencyMS
}

// bedrockConverseBody maps our simple {role, content} messages onto
// Bedrock's Converse API request shape, which is provider-agnostic across
// every model family hosted on Bedrock (Anthropic, OpenAI OSS, Kimi, ...).
func bedrockConverseBody(req ChatRequest) ([]byte, error) {
	type content struct {
		Text string `json:"text"`
	}
	type message struct {
		Role    string    `json:"role"`
		Content []content `json:"content"`
	}
	type inferenceConfig struct {
		MaxTokens   int     `json:"maxTokens,omitempty"`
		Temperature float64 `json:"temperature,omitempty"`
	}

	messages := make([]message, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = message{Role: m.Role, Content: []content{{Text: m.Content}}}
	}

	payload := struct {
		Messages        []message        `json:"messages"`
		InferenceConfig *inferenceConfig `json:"inferenceConfig,omitempty"`
	}{Messages: messages}

	if req.MaxTokens > 0 || req.Temperature > 0 {
		payload.InferenceConfig = &inferenceConfig{MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	}

	return json.Marshal(payload)
}
