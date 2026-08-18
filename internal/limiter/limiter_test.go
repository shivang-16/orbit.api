package limiter

import (
	"errors"
	"testing"
	"time"

	"github.com/shivang-16/orbit.api/internal/config"
)

func testLimits(rpm, burst, concurrent, playgroundRPM, playgroundBurst int) config.RateLimits {
	return config.RateLimits{
		Organization: config.RateLimitWindow{
			RequestsPerMinute: rpm,
			Burst:             burst,
			Concurrent:        concurrent,
		},
		Playground: config.RateLimitWindow{
			RequestsPerMinute: playgroundRPM,
			Burst:             playgroundBurst,
		},
	}
}

func TestMemoryOrgRPM(t *testing.T) {
	m := NewMemory(testLimits(60, 1, 5, 20, 5))

	res, err := m.Allow("org-1", "", false)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	res.Release()

	_, err = m.Allow("org-1", "", false)
	if err == nil {
		t.Fatal("expected org rpm to reject the second request")
	}
	var rl *Error
	if !errors.As(err, &rl) || rl.Scope != "organization" {
		t.Fatalf("got %#v", err)
	}
	if rl.RetryAfter < time.Second {
		t.Fatalf("retry-after = %s", rl.RetryAfter)
	}

	_, err = m.Allow("org-2", "", false)
	if err != nil {
		t.Fatalf("other org should be independent: %v", err)
	}
}

func TestMemoryConcurrency(t *testing.T) {
	m := NewMemory(testLimits(60, 10, 1, 20, 5))

	first, err := m.Allow("org-1", "", false)
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	_, err = m.Allow("org-1", "", false)
	if err == nil {
		t.Fatal("expected concurrency rejection")
	}
	var rl *Error
	if !errors.As(err, &rl) || rl.Scope != "concurrency" {
		t.Fatalf("got %#v", err)
	}

	first.Release()
	if _, err := m.Allow("org-1", "", false); err != nil {
		t.Fatalf("after release: %v", err)
	}
}

func TestMemoryPlaygroundRPM(t *testing.T) {
	m := NewMemory(testLimits(60, 10, 5, 20, 1))

	res, err := m.Allow("org-1", "user-a", true)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	res.Release()

	_, err = m.Allow("org-1", "user-a", true)
	if err == nil {
		t.Fatal("expected playground rpm rejection")
	}
	var rl *Error
	if !errors.As(err, &rl) || rl.Scope != "playground" {
		t.Fatalf("got %#v", err)
	}

	if _, err := m.Allow("org-1", "user-b", true); err != nil {
		t.Fatalf("other playground user: %v", err)
	}
	if _, err := m.Allow("org-1", "", false); err != nil {
		t.Fatalf("api key path still allowed: %v", err)
	}
}

func TestMemoryReleaseOnce(t *testing.T) {
	m := NewMemory(testLimits(60, 10, 1, 20, 5))
	res, err := m.Allow("org-1", "", false)
	if err != nil {
		t.Fatal(err)
	}
	res.Release()
	res.Release()
	if _, err := m.Allow("org-1", "", false); err != nil {
		t.Fatalf("double release should not leak slots: %v", err)
	}
}

func TestMemoryEmptyOrgSkipped(t *testing.T) {
	m := NewMemory(testLimits(60, 1, 1, 20, 1))
	if _, err := m.Allow("", "user", true); err != nil {
		t.Fatalf("empty org: %v", err)
	}
}
