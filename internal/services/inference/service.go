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

// Chat resolves the catalogue model and forwards the conversation upstream.
// It returns the raw upstream *http.Response so the controller can stream
// the body straight through to the caller without an extra decode/encode
// round trip; callers must close the response body.
func (s *Service) Chat(ctx context.Context, modelID string, req ChatRequest) (*http.Response, error) {
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

	body, err := bedrockConverseBody(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://bedrock-runtime.%s.amazonaws.com/model/%s/converse",
		s.bedrockRegion,
		url.PathEscape(entry.ModelID),
	)
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Authorization", "Bearer "+s.bedrockAPIKey)

	resp, err := httpClient.Do(upstream)
	if err != nil {
		return nil, fmt.Errorf("call bedrock: %w", err)
	}
	return resp, nil
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
