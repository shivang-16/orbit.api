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
	"log"
	"net/http"
	"net/url"
	"time"

	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
)

var (
	ErrInvalid             = errors.New("invalid request")
	ErrModelNotFound       = errors.New("model not found")
	ErrUnsupportedProvider = errors.New("model provider not supported yet")
	// ErrLowCredits is returned when an organization's remaining balance is
	// below lowBalanceThresholdMicros. Checked synchronously before calling
	// the model provider so a request never gets a paid response while out
	// of credit. See internal/repositories/billing.Repository.Record for the
	// matching post-request deduction, which is allowed to take the balance
	// negative for the single request that crosses the threshold.
	ErrLowCredits = errors.New("low on credits")
)

// lowBalanceThresholdMicros is $1.00 (1_000_000 micros = $1, matching the
// credits_micros/price_micros convention used across billing). Organizations
// at or above this remaining balance may proceed, even if the request that
// follows costs more than what's left — that one request is allowed to take
// the balance negative, to be settled by the organization's next credit
// grant/payment.
const lowBalanceThresholdMicros int64 = 1_000_000

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
	orgs          *organizationRepository.Repository
	bedrockAPIKey string
	bedrockRegion string
}

func NewService(
	catalogue *catalogueRepository.Repository,
	orgs *organizationRepository.Repository,
	bedrockAPIKey, bedrockRegion string,
) *Service {
	return &Service{catalogue: catalogue, orgs: orgs, bedrockAPIKey: bedrockAPIKey, bedrockRegion: bedrockRegion}
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

	if err := s.requireCredits(ctx); err != nil {
		return nil, err
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

// requireCredits is the hard pre-flight balance check: any organization
// below lowBalanceThresholdMicros is rejected before we ever call the model
// provider, so a $0-balance key can't keep getting billed inference. If the
// org has at least the threshold, the request is allowed through even
// though its actual cost may exceed what's left — that overage is expected
// to be settled by the next credit grant (see ErrLowCredits godoc).
func (s *Service) requireCredits(ctx context.Context) error {
	if s.orgs == nil {
		return nil
	}
	orgID, ok := apikeyMiddleware.OrganizationID(ctx)
	if !ok || orgID == "" {
		return nil
	}

	remaining, found, err := s.orgs.GetCreditsRemaining(ctx, orgID)
	if err != nil {
		return fmt.Errorf("check credits: %w", err)
	}
	if !found {
		log.Printf("inference: org=%s not found on credit check — blocking", orgID)
		return ErrLowCredits
	}
	if remaining < lowBalanceThresholdMicros {
		log.Printf("inference: org=%s blocked, remaining_micros=%d below threshold=%d", orgID, remaining, lowBalanceThresholdMicros)
		return ErrLowCredits
	}
	return nil
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
