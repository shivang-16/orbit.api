// Package inference validates and forwards chat requests made against an
// Orbit API key to the underlying model provider. For now the only
// supported provider is Bedrock, called directly over HTTPS with a Bedrock
// API key (bearer token) rather than SigV4, so no AWS SDK is required.
//
// Bedrock's Converse API has two operations behind the same request body:
// Converse (POST .../converse) returns one buffered JSON response,
// ConverseStream (POST .../converse-stream) returns the same content as an
// ordered Server-Sent-Events stream (messageStart/contentBlockDelta/...
// /metadata frames). We default every request to streaming and relay
// Bedrock's SSE frames to the caller unchanged; callers that want the old
// buffered behavior pass "stream": false.
package inference

import (
	"bufio"
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
	"github.com/shivang-16/orbit.api/internal/model"
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
//
// Timeout covers the whole request including reading the response body,
// so for a streamed call it's really "max total stream duration", not a
// connect timeout. It's set to match the router's per-route timeout for
// the chat endpoint (see routes/v1.go) so neither one is the surprise
// bottleneck for a long-running completion.
var httpClient = &http.Client{
	Timeout: 5 * time.Minute,
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
	StatusCode int
	Body       []byte
	// Streamed is true once the service has already written status/headers
	// and body chunks directly to the ResponseWriter passed to Chat. The
	// controller must not write Body/StatusCode again in that case.
	Streamed         bool
	ModelCatalogueID string
	InputTokens      int
	OutputTokens     int
	LatencyMS        int
}

// Chat validates the request, enforces the credit gate, resolves the
// model, and then dispatches to Bedrock either streamed (default) or
// buffered ("stream": false). w is only written to in the streaming case.
func (s *Service) Chat(ctx context.Context, modelID string, req ChatRequest, w http.ResponseWriter) (*ChatResult, error) {
	entry, err := s.prepare(ctx, modelID, req)
	if err != nil {
		return nil, err
	}

	if req.WantsStream() {
		return s.chatStream(ctx, entry, req, w)
	}
	return s.chatOnce(ctx, entry, req)
}

// prepare runs every check that must happen before we ever talk to
// Bedrock, shared by both the buffered and streamed paths: request shape,
// the hard credit gate, and model/provider resolution.
func (s *Service) prepare(ctx context.Context, modelID string, req ChatRequest) (*model.ModelCatalogue, error) {
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
	return entry, nil
}

// chatOnce is the original buffered Converse call: one request, one
// response body, used when the caller passes "stream": false.
func (s *Service) chatOnce(ctx context.Context, entry *model.ModelCatalogue, req ChatRequest) (*ChatResult, error) {
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

// chatStream calls Bedrock's ConverseStream operation. Bedrock only starts
// the SSE stream once it has accepted the request with a 200; validation
// errors (bad model, throttling, auth) come back as a normal buffered JSON
// body on a non-200 status, exactly like Converse, so that case is handled
// like chatOnce. Once streaming starts we relay every SSE line to w as it
// arrives and flush immediately, while also parsing the "metadata" frame
// for token usage (for billing) and watching for mid-stream exception
// frames (internalServerException, modelStreamErrorException, ...), which
// Bedrock can send after the 200 has already gone out.
func (s *Service) chatStream(ctx context.Context, entry *model.ModelCatalogue, req ChatRequest, w http.ResponseWriter) (*ChatResult, error) {
	payload, err := bedrockConverseBody(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://bedrock-runtime.%s.amazonaws.com/model/%s/converse-stream",
		s.bedrockRegion,
		url.PathEscape(entry.ModelID),
	)
	upstream, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Accept", "text/event-stream")
	upstream.Header.Set("Authorization", "Bearer "+s.bedrockAPIKey)

	started := time.Now()
	resp, err := httpClient.Do(upstream)
	if err != nil {
		return nil, fmt.Errorf("call bedrock: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read bedrock error response: %w", err)
		}
		return &ChatResult{
			StatusCode:       resp.StatusCode,
			Body:             body,
			ModelCatalogueID: entry.ID,
			LatencyMS:        int(time.Since(started).Milliseconds()),
		}, nil
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	inputTokens, outputTokens, latencyMS, streamErr := relayBedrockStream(resp.Body, w, flusher)
	if latencyMS == 0 {
		latencyMS = int(time.Since(started).Milliseconds())
	}

	status := http.StatusOK
	if streamErr {
		status = http.StatusBadGateway
	}

	return &ChatResult{
		StatusCode:       status,
		Streamed:         true,
		ModelCatalogueID: entry.ID,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		LatencyMS:        latencyMS,
	}, nil
}

// relayBedrockStream decodes Bedrock's binary AWS event-stream body one
// frame at a time and re-emits each as a plain-text SSE event to w
// (event: <bedrock event type>\ndata: <json payload>\n\n), flushing after
// every frame so the caller sees tokens as they're generated. Bedrock's
// own wire format is opaque binary framing (see eventstream.go); this is
// what turns it into something any standard SSE/EventSource client can
// read directly. The "metadata" event's payload carries final token usage
// (for billing), and a message-type of "exception" signals a stream
// failure that Bedrock can raise after the 200 has already gone out.
func relayBedrockStream(body io.Reader, w io.Writer, flusher http.Flusher) (inputTokens, outputTokens, latencyMS int, streamErr bool) {
	reader := bufio.NewReaderSize(body, 64*1024)

	for {
		frame, err := readAWSEventStreamFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return inputTokens, outputTokens, latencyMS, streamErr
			}
			log.Printf("inference: decode bedrock event-stream: %v", err)
			return inputTokens, outputTokens, latencyMS, true
		}

		eventType := frame.headers[":event-type"]
		if frame.headers[":message-type"] == "exception" {
			eventType = "error"
			log.Printf("inference: bedrock mid-stream exception type=%s payload=%s", frame.headers[":exception-type"], frame.payload)
			streamErr = true
		}
		if eventType == "" {
			eventType = "message"
		}

		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, frame.payload); err != nil {
			return inputTokens, outputTokens, latencyMS, true
		}
		if flusher != nil {
			flusher.Flush()
		}

		if eventType == "metadata" {
			var meta struct {
				Usage struct {
					InputTokens  int `json:"inputTokens"`
					OutputTokens int `json:"outputTokens"`
				} `json:"usage"`
				Metrics struct {
					LatencyMS int `json:"latencyMs"`
				} `json:"metrics"`
			}
			if err := json.Unmarshal(frame.payload, &meta); err == nil {
				inputTokens = meta.Usage.InputTokens
				outputTokens = meta.Usage.OutputTokens
				latencyMS = meta.Metrics.LatencyMS
			}
		}
	}
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
