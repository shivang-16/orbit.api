// Package inference validates and forwards chat requests made against an
// Orbit API key to the underlying model provider. For now the only
// supported provider is Bedrock, called directly over HTTPS with a Bedrock
// API key (bearer token) rather than SigV4, so no AWS SDK is required.
//
// Bedrock's Converse API has two operations behind the same request body:
// Converse (POST .../converse) returns one buffered JSON response,
// ConverseStream (POST .../converse-stream) returns the same content as an
// ordered Server-Sent-Events stream (messageStart/contentBlockDelta/...
// /metadata frames). OpenAI GPT-5.x frontier models are the exception:
// they are served only on Bedrock Mantle's Responses API
// (POST https://bedrock-mantle.{region}.api.aws/openai/v1/responses) and are
// translated back into the same Converse shapes so callers and billing
// do not change. Requests are buffered JSON unless the caller passes
// "stream": true, in which case we relay frames through StreamSink.
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

	"github.com/shivang-16/orbit.api/internal/config"
	"github.com/shivang-16/orbit.api/internal/limiter"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	authMiddleware "github.com/shivang-16/orbit.api/internal/middleware/auth"
	"github.com/shivang-16/orbit.api/internal/model"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
)

var (
	ErrInvalid             = errors.New("invalid request")
	ErrModelNotFound       = errors.New("model not found")
	ErrUnsupportedProvider = errors.New("model provider not supported yet")
	// ErrLowCredits is returned when an organization cannot cover this
	// request's cost ceiling (below the $0.01 floor, or the hold does not
	// fit remaining). Checked before calling the model provider.
	ErrLowCredits = errors.New("low on credits")
)

type Service struct {
	catalogue     *catalogueRepository.Repository
	reserver      Reserver
	bedrockAPIKey string
	bedrockRegion string
	httpClient    *http.Client
	limiter       limiter.Limiter
}

func NewService(
	catalogue *catalogueRepository.Repository,
	reserver Reserver,
	cfg config.Config,
) *Service {
	timeout := time.Duration(cfg.Server.InferenceTimeoutSeconds) * time.Second
	if timeout < time.Second {
		timeout = 5 * time.Minute
	}
	return &Service{
		catalogue:     catalogue,
		reserver:      reserver,
		bedrockAPIKey: cfg.AWSBedrockAPIKey,
		bedrockRegion: cfg.AWSBedrockRegion,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		limiter: limiter.NewMemory(cfg.RateLimits),
	}
}

type ChatResult struct {
	StatusCode int
	Body       []byte
	// Streamed is true once the service has already written status/headers
	// and body chunks directly to the ResponseWriter passed to Chat. The
	// controller must not write Body/StatusCode again in that case.
	Streamed         bool
	ModelCatalogueID string
	// ModelSlug is the resolved catalogue entry's public slug, used by the
	// OpenAI/Anthropic compat responses to echo back a "model" value the
	// caller recognizes (as opposed to the internal catalogue UUID).
	ModelSlug    string
	InputTokens  int
	OutputTokens int
	LatencyMS    int
	HoldID       string
}

// Chat validates the request, enforces the credit gate, resolves the
// model, and then dispatches to Bedrock either buffered (default) or
// streamed ("stream": true). w is only written to in the streaming case.
func (s *Service) Chat(ctx context.Context, modelID string, req ChatRequest, w http.ResponseWriter) (*ChatResult, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}
	release, err := s.acquireRateLimit(ctx, w)
	if err != nil {
		return nil, err
	}
	defer release()
	entry, err := s.resolveModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	hold, err := s.placeHold(ctx, entry.ID, EstimateInputTokens(chatInputText(req)), req.MaxTokens)
	if err != nil {
		return nil, err
	}
	req.MaxTokens = hold.MaxTokens

	result, err := s.dispatchChat(ctx, entry, req, w)
	if err != nil {
		s.releaseHold(ctx, hold.ID)
		return nil, err
	}
	result.HoldID = hold.ID
	return result, nil
}

func (s *Service) dispatchChat(ctx context.Context, entry *model.ModelCatalogue, req ChatRequest, w http.ResponseWriter) (*ChatResult, error) {

	if usesMantleResponses(entry.ModelID) {
		var sink StreamSink
		if req.WantsStream() {
			flusher, _ := w.(http.Flusher)
			sink = &passthroughSink{w: w, flusher: flusher}
		}
		return s.callMantle(ctx, entry, chatRequestToConverse(req), sink, w)
	}

	payload, err := bedrockConverseBody(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	if req.WantsStream() {
		flusher, _ := w.(http.Flusher)
		return s.chatStream(ctx, entry, payload, &passthroughSink{w: w, flusher: flusher}, w)
	}
	return s.chatOnce(ctx, entry, payload)
}

// resolveModel looks up a model by its public identifier (slug or
// catalogue UUID — see catalogueRepository.GetByIdentifier) and confirms
// it's servable, shared by the native Chat path and the OpenAI/Anthropic
// compat Converse path.
func (s *Service) resolveModel(ctx context.Context, modelIdentifier string) (*model.ModelCatalogue, error) {
	entry, err := s.catalogue.GetByIdentifier(ctx, modelIdentifier)
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
// response body, used when the caller passes "stream": false. payload is
// an already-encoded Bedrock Converse request body, shared with the
// OpenAI/Anthropic compat Converse path.
func (s *Service) chatOnce(ctx context.Context, entry *model.ModelCatalogue, payload []byte) (*ChatResult, error) {
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
	resp, err := s.httpClient.Do(upstream)
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
		ModelSlug:        entry.Slug,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		LatencyMS:        latencyMS,
	}, nil
}

// chatStream calls Bedrock's ConverseStream operation. Bedrock only starts
// the SSE stream once it has accepted the request with a 200; validation
// errors (bad model, throttling, auth) come back as a normal buffered JSON
// body on a non-200 status, exactly like Converse, so that case is handled
// like chatOnce. Once streaming starts, every decoded Bedrock frame is
// handed to sink (see StreamSink), which owns turning it into whatever
// wire dialect the caller wants (native passthrough, OpenAI, Anthropic),
// while this method centrally parses the "metadata" frame for token usage
// (for billing) and watches for mid-stream exception frames
// (internalServerException, modelStreamErrorException, ...), which Bedrock
// can send after the 200 has already gone out.
func (s *Service) chatStream(ctx context.Context, entry *model.ModelCatalogue, payload []byte, sink StreamSink, w http.ResponseWriter) (*ChatResult, error) {
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
	resp, err := s.httpClient.Do(upstream)
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
			ModelSlug:        entry.Slug,
			LatencyMS:        int(time.Since(started).Milliseconds()),
		}, nil
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	inputTokens, outputTokens, latencyMS, streamErr := relayBedrockStream(resp.Body, sink)
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
		ModelSlug:        entry.Slug,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		LatencyMS:        latencyMS,
	}, nil
}

// relayBedrockStream decodes Bedrock's binary AWS event-stream body one
// frame at a time and hands each decoded frame to sink, which owns
// encoding it into whatever wire dialect the caller wants (see
// StreamSink). Bedrock's own wire format is opaque binary framing (see
// eventstream.go); this is what turns it into named (eventType, payload)
// frames any sink can consume. The "metadata" event's payload carries
// final token usage (for billing), and a message-type of "exception"
// signals a stream failure that Bedrock can raise after the 200 has
// already gone out.
func relayBedrockStream(body io.Reader, sink StreamSink) (inputTokens, outputTokens, latencyMS int, streamErr bool) {
	reader := bufio.NewReaderSize(body, 64*1024)

	for {
		frame, err := readAWSEventStreamFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = sink.Close(streamErr)
				return inputTokens, outputTokens, latencyMS, streamErr
			}
			log.Printf("inference: decode bedrock event-stream: %v", err)
			_ = sink.Close(true)
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

		if err := sink.HandleFrame(eventType, frame.payload); err != nil {
			_ = sink.Close(true)
			return inputTokens, outputTokens, latencyMS, true
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

func (s *Service) acquireRateLimit(ctx context.Context, w http.ResponseWriter) (func(), error) {
	if s.limiter == nil {
		return func() {}, nil
	}
	orgID, _ := apikeyMiddleware.OrganizationID(ctx)
	userID, _ := authMiddleware.UserID(ctx)
	res, err := s.limiter.Allow(orgID, userID, limiter.IsPlayground(ctx))
	if err != nil {
		limiter.SetHeadersFromError(w, err)
		return nil, err
	}
	limiter.SetHeaders(w, res.Limit, res.Remaining, res.Reset, 0)
	return res.Release, nil
}

func (s *Service) placeHold(ctx context.Context, catalogueID string, inputTokens, requestedMaxTokens int) (*Hold, error) {
	if s.reserver == nil {
		return nil, ErrLowCredits
	}
	orgID, ok := apikeyMiddleware.OrganizationID(ctx)
	if !ok || orgID == "" {
		return nil, ErrLowCredits
	}
	hold, err := s.reserver.Reserve(ctx, ReserveRequest{
		OrganizationID:     orgID,
		CatalogueID:        catalogueID,
		InputTokens:        inputTokens,
		RequestedMaxTokens: requestedMaxTokens,
	})
	if err != nil {
		return nil, err
	}
	if hold == nil || hold.ID == "" || hold.MaxTokens < 1 {
		return nil, ErrLowCredits
	}
	log.Printf("inference: org=%s hold=%s amount_micros=%d max_tokens=%d", orgID, hold.ID, hold.AmountMicros, hold.MaxTokens)
	return hold, nil
}

func (s *Service) releaseHold(ctx context.Context, holdID string) {
	if s.reserver == nil || holdID == "" {
		return
	}
	releaseCtx := context.WithoutCancel(ctx)
	if err := s.reserver.Release(releaseCtx, holdID); err != nil {
		log.Printf("inference: release hold=%s: %v", holdID, err)
	}
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

	maxTokens := req.MaxTokens
	if maxTokens < 1 {
		maxTokens = 4096
	}
	payload.InferenceConfig = &inferenceConfig{MaxTokens: maxTokens, Temperature: req.Temperature}

	return json.Marshal(payload)
}
