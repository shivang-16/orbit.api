package config

import "testing"

func TestLoadFileConfig(t *testing.T) {
	cfg := Load()

	if cfg.Credits.SignupMicros != 2_000_000 {
		t.Fatalf("signup_micros = %d", cfg.Credits.SignupMicros)
	}
	if cfg.Credits.LowBalanceThresholdMicros != 1_000_000 {
		t.Fatalf("low_balance_threshold_micros = %d", cfg.Credits.LowBalanceThresholdMicros)
	}
	if cfg.RateLimits.Organization.RequestsPerMinute != 60 || cfg.RateLimits.Organization.Concurrent != 5 {
		t.Fatalf("organization limits = %+v", cfg.RateLimits.Organization)
	}
	if cfg.RateLimits.Playground.RequestsPerMinute != 20 {
		t.Fatalf("playground limits = %+v", cfg.RateLimits.Playground)
	}
	if cfg.Server.DashboardTimeoutSeconds != 30 || cfg.Server.InferenceTimeoutSeconds != 300 {
		t.Fatalf("server timeouts = %+v", cfg.Server)
	}
	if cfg.Postgres.MaxOpenConns != 10 {
		t.Fatalf("postgres max_open_conns = %d", cfg.Postgres.MaxOpenConns)
	}
}

func TestRateLimitsValidate(t *testing.T) {
	limits := RateLimits{
		Organization: RateLimitWindow{RequestsPerMinute: 60, Burst: 10, Concurrent: 5},
		Playground:   RateLimitWindow{RequestsPerMinute: 20, Burst: 5},
	}
	if err := limits.validate(); err != nil {
		t.Fatalf("valid limits: %v", err)
	}
	limits.Organization.Concurrent = 0
	if err := limits.validate(); err == nil {
		t.Fatal("expected concurrent=0 to fail")
	}
}

func TestResolveCORSFallback(t *testing.T) {
	t.Setenv("CORS_ORIGINS", "")
	got := resolveCORS(nil)
	if len(got) != 1 || got[0] != "http://localhost:3000" {
		t.Fatalf("empty yaml cors = %v", got)
	}

	got = resolveCORS([]string{"https://app.orbit.example"})
	if len(got) != 1 || got[0] != "https://app.orbit.example" {
		t.Fatalf("yaml cors = %v", got)
	}

	t.Setenv("CORS_ORIGINS", "https://a.example, https://b.example")
	got = resolveCORS([]string{"https://ignored.example"})
	if len(got) != 2 || got[0] != "https://a.example" || got[1] != "https://b.example" {
		t.Fatalf("env cors = %v", got)
	}
}
