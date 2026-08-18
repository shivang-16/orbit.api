package inference_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"

	"github.com/shivang-16/orbit.api/internal/config"
	inferenceController "github.com/shivang-16/orbit.api/internal/controller/inference"
	"github.com/shivang-16/orbit.api/internal/infra/postgres"
	apikeyMiddleware "github.com/shivang-16/orbit.api/internal/middleware/apikey"
	apikeyRepository "github.com/shivang-16/orbit.api/internal/repositories/apikey"
	billingRepository "github.com/shivang-16/orbit.api/internal/repositories/billing"
	catalogueRepository "github.com/shivang-16/orbit.api/internal/repositories/catalogue"
	organizationRepository "github.com/shivang-16/orbit.api/internal/repositories/organization"
	pricingRepository "github.com/shivang-16/orbit.api/internal/repositories/pricing"
	apikeyService "github.com/shivang-16/orbit.api/internal/services/apikey"
	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

func TestChatSonnet45(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	cfg := config.Load()
	if cfg.AWSBedrockAPIKey == "" {
		t.Fatal("AWS_BEDROCK_API_KEY is required")
	}

	db, err := postgres.Open(cfg.Postgres)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var modelID, modelName string
	err = db.DB().QueryRowContext(
		ctx,
		`SELECT id, name FROM model_catalogue
		 WHERE name = $1 AND is_active = true
		 LIMIT 1`,
		"Claude Sonnet 4.5",
	).Scan(&modelID, &modelName)
	if err != nil {
		t.Fatalf("lookup Claude Sonnet 4.5: %v", err)
	}

	secret, keyID := insertTestKey(t, ctx, db)
	t.Cleanup(func() {
		_, _ = db.DB().ExecContext(context.Background(), `DELETE FROM api_keys WHERE id = $1`, keyID)
	})

	catalogueRepo := catalogueRepository.NewRepository(db.DB())
	apiKeyRepo := apikeyRepository.NewRepository(db.DB())
	orgRepo := organizationRepository.NewRepository(db.DB())
	billingWorker := billingService.NewWorker(
		billingRepository.NewRepository(db.DB()),
		pricingRepository.NewRepository(db.DB()),
	)
	svc := inferenceService.NewService(catalogueRepo, orgRepo, cfg.AWSBedrockAPIKey, cfg.AWSBedrockRegion)
	ctrl := inferenceController.NewController(svc, billingWorker, orgRepo)

	r := chi.NewRouter()
	r.Use(apikeyMiddleware.New(apiKeyRepo).Authenticate)
	r.Post("/api/v1/models/{id}/chat", ctrl.Chat)

	body, err := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "In 3-4 sentences, explain how a satellite stays in orbit around Earth."},
		},
		"max_tokens": 250,
		"stream":     false, // this test asserts on a single buffered JSON body
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/models/"+modelID+"/chat", bytes.NewReader(body))
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	raw, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	pretty := prettyJSON(raw)
	fmt.Printf("\n=== %s (%s) ===\nstatus: %d\n%s\n", modelName, modelID, rec.Code, pretty)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, pretty)
	}

	deadline := time.Now().Add(5 * time.Second)
	var logged, charged int
	for time.Now().Before(deadline) {
		_ = db.DB().QueryRowContext(
			ctx,
			`SELECT
				(SELECT COUNT(*) FROM inference_requests WHERE model_catalogue_id = $1),
				(SELECT COUNT(*) FROM credit_ledger WHERE organization_id = (
					SELECT organization_id FROM api_keys WHERE id = $2
				) AND entry_type = 'usage')`,
			modelID,
			keyID,
		).Scan(&logged, &charged)
		if logged > 0 && charged > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if logged == 0 {
		t.Fatal("expected an inference_requests row")
	}
	fmt.Printf("logged requests: %d  ledger usage rows: %d\n", logged, charged)
}

func TestChatSonnet45Stream(t *testing.T) {
	_ = godotenv.Load("../../../.env")
	cfg := config.Load()
	if cfg.AWSBedrockAPIKey == "" {
		t.Fatal("AWS_BEDROCK_API_KEY is required")
	}

	db, err := postgres.Open(cfg.Postgres)
	if err != nil {
		t.Fatalf("postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var modelID, modelName string
	err = db.DB().QueryRowContext(
		ctx,
		`SELECT id, name FROM model_catalogue
		 WHERE name = $1 AND is_active = true
		 LIMIT 1`,
		"Claude Sonnet 4.5",
	).Scan(&modelID, &modelName)
	if err != nil {
		t.Fatalf("lookup Claude Sonnet 4.5: %v", err)
	}

	secret, keyID := insertTestKey(t, ctx, db)
	t.Cleanup(func() {
		_, _ = db.DB().ExecContext(context.Background(), `DELETE FROM api_keys WHERE id = $1`, keyID)
	})

	catalogueRepo := catalogueRepository.NewRepository(db.DB())
	apiKeyRepo := apikeyRepository.NewRepository(db.DB())
	orgRepo := organizationRepository.NewRepository(db.DB())
	billingWorker := billingService.NewWorker(
		billingRepository.NewRepository(db.DB()),
		pricingRepository.NewRepository(db.DB()),
	)
	svc := inferenceService.NewService(catalogueRepo, orgRepo, cfg.AWSBedrockAPIKey, cfg.AWSBedrockRegion)
	ctrl := inferenceController.NewController(svc, billingWorker, orgRepo)

	r := chi.NewRouter()
	r.Use(apikeyMiddleware.New(apiKeyRepo).Authenticate)
	r.Post("/api/v1/models/{id}/chat", ctrl.Chat)

	body, err := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "user", "content": "Count from 1 to 8, one number per sentence."},
		},
		"max_tokens": 80,
		"stream":     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/models/"+modelID+"/chat", bytes.NewReader(body))
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+secret)
	req.Header.Set("Content-Type", "application/json")

	testStart := time.Now()
	rec := &streamRecorder{header: make(http.Header), started: time.Now()}
	fmt.Printf("\n=== STREAM %s (%s) ===\n", modelName, modelID)
	r.ServeHTTP(rec, req)

	fmt.Printf("status: %d  content-type: %s  chunks: %d  flushes: %d  elapsed: %dms\n",
		rec.status, rec.header.Get("Content-Type"), rec.chunks, rec.flushes, time.Since(rec.started).Milliseconds())

	if rec.status != http.StatusOK {
		t.Fatalf("expected 200, got %d:\n%s", rec.status, rec.body.String())
	}
	if got := rec.header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q", got)
	}
	if rec.chunks < 3 {
		t.Fatalf("expected several SSE chunks, got %d:\n%s", rec.chunks, rec.body.String())
	}
	if rec.flushes < 2 {
		t.Fatalf("expected multiple flushes (true streaming), got %d", rec.flushes)
	}
	raw := rec.body.String()

	// Dump the exact bytes we relayed to the client so we can inspect the
	// wire format directly (plain SSE text vs AWS's binary event-stream
	// framing) instead of guessing from how a terminal/GUI client renders it.
	dumpPath := "/tmp/orbit_stream_dump.bin"
	if err := os.WriteFile(dumpPath, rec.body.Bytes(), 0o644); err != nil {
		t.Logf("could not write dump file: %v", err)
	} else {
		t.Logf("wrote %d raw relayed bytes to %s", rec.body.Len(), dumpPath)
	}
	t.Logf("valid UTF-8: %t", utf8.Valid(rec.body.Bytes()))
	preview := rec.body.Bytes()
	if len(preview) > 300 {
		preview = preview[:300]
	}
	t.Logf("hex preview (first %d bytes):\n%s", len(preview), hex.Dump(preview))

	if !bytes.Contains(rec.body.Bytes(), []byte("event: contentBlockDelta")) {
		t.Fatalf("response is not decoded plain-text SSE:\n%s", raw)
	}
	if !bytes.Contains(rec.body.Bytes(), []byte("event: metadata")) {
		t.Fatalf("response is missing the final metadata/usage SSE event:\n%s", raw)
	}

	// The billing worker records usage asynchronously (see
	// billingService.Worker.Enqueue): wait for the new ledger row instead
	// of racing t.Cleanup's db.Close() against that background write.
	deadline := time.Now().Add(5 * time.Second)
	var logged, charged int
	for time.Now().Before(deadline) {
		_ = db.DB().QueryRowContext(
			ctx,
			`SELECT
				(SELECT COUNT(*) FROM inference_requests WHERE model_catalogue_id = $1 AND created_at >= $2),
				(SELECT COUNT(*) FROM credit_ledger WHERE organization_id = (
					SELECT organization_id FROM api_keys WHERE id = $3
				) AND entry_type = 'usage' AND created_at >= $2)`,
			modelID,
			testStart,
			keyID,
		).Scan(&logged, &charged)
		if logged > 0 && charged > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if logged == 0 {
		t.Fatal("expected a new inference_requests row for this stream")
	}
	if charged == 0 {
		t.Fatal("expected a new credit_ledger usage row for this stream — billing not recorded")
	}
	fmt.Printf("logged requests: %d  ledger usage rows: %d\n", logged, charged)
}

// streamRecorder is an http.ResponseWriter that also implements
// http.Flusher, so the streaming path actually flushes tokens as they
// arrive instead of buffering them like httptest.ResponseRecorder.
type streamRecorder struct {
	header  http.Header
	status  int
	body    bytes.Buffer
	chunks  int
	flushes int
	started time.Time
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
	s.chunks++
	fmt.Printf("[%4dms] %s", time.Since(s.started).Milliseconds(), p)
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
		"sonnet-test",
		apikeyService.HashSecret(secret),
		"sk-orbit- ... est",
	).Scan(&keyID)
	if err != nil {
		t.Fatalf("insert test key: %v", err)
	}
	return secret, keyID
}

func prettyJSON(raw []byte) string {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}
