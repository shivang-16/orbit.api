// These are integration tests against real Bedrock, mirroring
// internal/controller/inference/sonnet_test.go: they require
// AWS_BEDROCK_API_KEY and a reachable Postgres (see .env), so they're run
// manually rather than in CI.
package openai_test

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
	openaiController "github.com/shivang-16/orbit.api/internal/controller/compat/openai"
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
	db      *postgres.Client
	router  *chi.Mux
	secret  string
	keyID   string
	slug    string
	modelID string
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
	ctrl := openaiController.NewController(svc, catalogueRepo, billingWorker)

	r := chi.NewRouter()
	r.Use(apikeyMiddleware.New(apiKeyRepo, nil).Authenticate)
	r.Post("/api/v1/chat/completions", ctrl.ChatCompletions)
	r.Get("/api/v1/models", ctrl.ListModels)

	return &testHarness{db: db, router: r, secret: secret, keyID: keyID, slug: slug, modelID: modelID}
}

// TestChatCompletions_Buffered exercises the OpenAI-shaped buffered
// response, resolving the model by its public slug (not the catalogue
// UUID) to also cover GetByIdentifier.
func TestChatCompletions_Buffered(t *testing.T) {
	h := newTestHarness(t)

	body, _ := json.Marshal(map[string]any{
		"model": h.slug,
		"messages": []map[string]string{
			{"role": "system", "content": "Answer in exactly one short sentence."},
			{"role": "user", "content": "Why is the sky blue?"},
		},
		"max_tokens": 100,
		"stream":     false,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+h.secret)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
	if resp.Object != "chat.completion" {
		t.Fatalf("object = %q", resp.Object)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content == "" {
		t.Fatalf("expected non-empty assistant content: %+v", resp.Choices)
	}
	if resp.Usage.PromptTokens == 0 || resp.Usage.CompletionTokens == 0 {
		t.Fatalf("expected non-zero usage, got %+v", resp.Usage)
	}
	fmt.Printf("buffered content: %s\nfinish_reason: %s\nusage: %+v\n", resp.Choices[0].Message.Content, resp.Choices[0].FinishReason, resp.Usage)
}

// TestChatCompletions_StreamedToolCall exercises the streaming path with
// a tool definition, verifying the accumulated tool_calls[].function.
// arguments fragments join into valid JSON, matching the real OpenAI SDK's
// accumulation contract.
func TestChatCompletions_StreamedToolCall(t *testing.T) {
	h := newTestHarness(t)

	body, _ := json.Marshal(map[string]any{
		"model": h.slug,
		"messages": []map[string]string{
			{"role": "user", "content": "What is the weather like in Paris right now? Use the tool."},
		},
		"tools": []map[string]any{{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get the current weather for a city",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string"}},
					"required":   []string{"city"},
				},
			},
		}},
		"tool_choice": "required",
		"max_tokens":  200,
		"stream":      true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+h.secret)
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
	if !strings.Contains(raw, "data: [DONE]") {
		t.Fatalf("expected stream to end with [DONE]:\n%s", raw)
	}

	var toolName, argsJoined string
	for _, block := range strings.Split(raw, "\n\n") {
		block = strings.TrimSpace(strings.TrimPrefix(block, "data: "))
		if block == "" || block == "[DONE]" {
			continue
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					ToolCalls []struct {
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(block), &chunk); err != nil {
			t.Fatalf("chunk not valid JSON: %v (%s)", err, block)
		}
		for _, c := range chunk.Choices {
			for _, tc := range c.Delta.ToolCalls {
				if tc.Function.Name != "" {
					toolName = tc.Function.Name
				}
				argsJoined += tc.Function.Arguments
			}
		}
	}

	if toolName != "get_weather" {
		t.Fatalf("expected tool call to get_weather, got %q (full stream:\n%s)", toolName, raw)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJoined), &args); err != nil {
		t.Fatalf("accumulated tool arguments are not valid JSON: %v (%q)", err, argsJoined)
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
		"openai-compat-test",
		apikeyService.HashSecret(secret),
		"sk-orbit- ... est",
	).Scan(&keyID)
	if err != nil {
		t.Fatalf("insert test key: %v", err)
	}
	return secret, keyID
}
