// These are integration tests against real Bedrock, mirroring
// internal/controller/inference/sonnet_test.go: they require
// AWS_BEDROCK_API_KEY and a reachable Postgres (see .env), so they're run
// manually rather than in CI.
package anthropic_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"github.com/shivang-16/orbit.api/internal/config"
	anthropicController "github.com/shivang-16/orbit.api/internal/controller/compat/anthropic"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	apikeyRepository "github.com/shivang-16/orbit.api/internal/repositories/apikey"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
	apikeyService "github.com/shivang-16/orbit.api/internal/services/apikey"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type testHarness struct {
	db     *postgres.Client
	router *chi.Mux
	secret string
	keyID  string
	slug   string
}

func newTestHarness(t *testing.T) *testHarness {
	t.Helper()
	_ = godotenv.Load("../../../../.env")
	cfg := config.Load()
	if cfg.AWSBedrockAPIKey == "" {
		t.Fatal("AWS_BEDROCK_API_KEY is required")
	}

	db, err := postgres.Open(cfg.Postgres)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var modelID, slug string
	if err := db.DB().QueryRowContext(
		ctx,
		`SELECT id, slug FROM model_catalogue WHERE name = $1 AND is_active = true LIMIT 1`,
		"Claude Sonnet 4.5",
	).Scan(&modelID, &slug); err != nil {
		t.Fatalf("lookup Claude Sonnet 4.5: %v", err)
	}

	secret, keyID := insertTestKey(t, ctx, db)
	t.Cleanup(func() {
		_, _ = db.DB().ExecContext(context.Background(), `DELETE FROM api_keys WHERE id = $1`, keyID)
	})

	catalogueRepo := catalogueRepository.NewRepository(db.DB())
	apiKeyRepo := apikeyRepository.NewRepository(db.DB())
	billingRepo := billingRepository.NewRepository(db.DB())
	pricingRepo := pricingRepository.NewRepository(db.DB())
	billingWorker := billingService.NewWorker(billingRepo, pricingRepo)
	reserver := billingService.NewReserver(billingRepo, pricingRepo, cfg)
	svc := inferenceService.NewService(catalogueRepo, reserver, cfg)
	ctrl := anthropicController.NewController(svc, billingWorker)

	r := chi.NewRouter()
	r.Use(apikeyMiddleware.New(apiKeyRepo, nil).Authenticate)
	r.Post("/api/v1/messages", ctrl.Messages)

	return &testHarness{db: db, router: r, secret: secret, keyID: keyID, slug: slug}
}

// TestMessages_Buffered exercises the Anthropic-shaped buffered response,
// authenticating with X-Api-Key (the Anthropic SDK's default header)
// instead of Authorization: Bearer, to also cover that middleware path.
func TestMessages_Buffered(t *testing.T) {
	h := newTestHarness(t)

	body, _ := json.Marshal(map[string]any{
		"model":      h.slug,
		"max_tokens": 100,
		"system":     "Answer in exactly one short sentence.",
		"messages": []map[string]string{
			{"role": "user", "content": "Why is the sky blue?"},
		},
		"stream": false,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewReader(body))
	req.Header.Set("X-Api-Key", h.secret)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	if resp.Type != "message" || resp.Role != "assistant" {
		t.Fatalf("type/role = %q/%q", resp.Type, resp.Role)
	}
	if len(resp.Content) != 1 || resp.Content[0].Type != "text" || resp.Content[0].Text == "" {
		t.Fatalf("expected one non-empty text block, got %+v", resp.Content)
	}
	if resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens == 0 {
		t.Fatalf("expected non-zero usage, got %+v", resp.Usage)
	}
	fmt.Printf("buffered content: %s\nstop_reason: %s\nusage: %+v\n", resp.Content[0].Text, resp.StopReason, resp.Usage)
}

// TestMessages_StreamedToolUse exercises the streaming path with a tool
// definition, verifying the accumulated input_json_delta fragments join
// into valid JSON, matching the real Anthropic SDK's accumulation
// contract.
func TestMessages_StreamedToolUse(t *testing.T) {
	h := newTestHarness(t)

	body, _ := json.Marshal(map[string]any{
		"model":      h.slug,
		"max_tokens": 200,
		"messages": []map[string]string{
			{"role": "user", "content": "What is the weather like in Paris right now? Use the tool."},
		},
		"tools": []map[string]any{{
			"name":        "get_weather",
			"description": "Get the current weather for a city",
			"input_schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
				"required":   []string{"city"},
			},
		}},
		"tool_choice": map[string]any{"type": "any"},
		"stream":      true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewReader(body))
	req.Header.Set("X-Api-Key", h.secret)
	req.Header.Set("Content-Type", "application/json")

	rec := &streamRecorder{header: make(http.Header)}
	h.router.ServeHTTP(rec, req)

	if rec.status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.status, rec.body.String())
	}
	if got := rec.header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q", got)
	}

	raw := rec.body.String()
	if !strings.Contains(raw, "event: message_stop") {
		t.Fatalf("expected stream to end with message_stop:\n%s", raw)
	}

	var toolName, argsJoined string
	for _, block := range strings.Split(raw, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.SplitN(block, "\n", 2)
		if len(lines) != 2 {
			continue
		}
		payload := strings.TrimPrefix(lines[1], "data: ")

		switch strings.TrimPrefix(lines[0], "event: ") {
		case "content_block_start":
			var start struct {
				ContentBlock struct {
					Type string `json:"type"`
					Name string `json:"name"`
				} `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(payload), &start); err == nil && start.ContentBlock.Type == "tool_use" {
				toolName = start.ContentBlock.Name
			}
		case "content_block_delta":
			var delta struct {
				Delta struct {
					Type        string `json:"type"`
					PartialJSON string `json:"partial_json"`
				} `json:"delta"`
			}
			if err := json.Unmarshal([]byte(payload), &delta); err == nil && delta.Delta.Type == "input_json_delta" {
				argsJoined += delta.Delta.PartialJSON
			}
		}
	}

	if toolName != "get_weather" {
		t.Fatalf("expected tool call to get_weather, got %q (full stream:\n%s)", toolName, raw)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJoined), &args); err != nil {
		t.Fatalf("accumulated tool input is not valid JSON: %v (%q)", err, argsJoined)
	}
	if _, ok := args["city"]; !ok {
		t.Fatalf("expected a \"city\" argument, got %v", args)
	}
	fmt.Printf("tool call: %s(%s)\n", toolName, argsJoined)
}

type streamRecorder struct {
	header  http.Header
	status  int
	body    bytes.Buffer
	flushes int
}

func (s *streamRecorder) Header() http.Header { return s.header }
func (s *streamRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
}
func (s *streamRecorder) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.WriteHeader(http.StatusOK)
	}
	return s.body.Write(p)
}
func (s *streamRecorder) Flush() { s.flushes++ }

func insertTestKey(t *testing.T, ctx context.Context, db *postgres.Client) (secret, keyID string) {
	t.Helper()

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	secret = "sk-orbit-" + hex.EncodeToString(raw)

	var orgID, userID string
	if err := db.DB().QueryRowContext(ctx, `SELECT id FROM organizations LIMIT 1`).Scan(&orgID); err != nil {
		t.Fatalf("lookup organization: %v", err)
	}
	if err := db.DB().QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("lookup user: %v", err)
	}

	err := db.DB().QueryRowContext(
		ctx,
		`INSERT INTO api_keys (organization_id, created_by, name, key_hash, key_preview)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		orgID,
		userID,
		"anthropic-compat-test",
		apikeyService.HashSecret(secret),
		"sk-orbit- ... est",
	).Scan(&keyID)
	if err != nil {
		t.Fatalf("insert test key: %v", err)
	}
	return secret, keyID
}
