package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shivang-16/orbit.api/internal/config"
)

func TestChatCompletionsPreflightIsNot405(t *testing.T) {
	r := chi.NewRouter()
	r.Use(corsByPath(config.Config{CORSOrigins: []string{"https://tryorbit.cloud"}}))
	r.Post("/api/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/chat/completions", nil)
	req.Header.Set("Origin", "https://docs.tryorbit.cloud")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("preflight status = %d, want 200 (got Chi 405 before root CORS)", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

func TestDashboardPreflightStaysAllowlisted(t *testing.T) {
	r := chi.NewRouter()
	r.Use(corsByPath(config.Config{CORSOrigins: []string{"https://tryorbit.cloud"}}))
	r.Get("/api/v1/billing/credits", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/billing/credits", nil)
	req.Header.Set("Origin", "https://tryorbit.cloud")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard preflight status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://tryorbit.cloud" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want dashboard origin", got)
	}
}

func TestIsAPIKeyRoute(t *testing.T) {
	cases := map[string]bool{
		"/api/v1/chat/completions":            true,
		"/api/v1/messages":                    true,
		"/api/v1/models":                      true,
		"/api/v1/models/claude-sonnet-5/chat": true,
		"/api/v1/playground/models/x/chat":    false,
		"/api/v1/billing/credits":             false,
		"/api/v1/catalogue":                   false,
	}
	for path, want := range cases {
		if got := isAPIKeyRoute(path); got != want {
			t.Fatalf("isAPIKeyRoute(%q) = %v, want %v", path, got, want)
		}
	}
}
