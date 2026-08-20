package billing

import (
	"errors"
	"testing"
)

func TestChargeMicrosCeil(t *testing.T) {
	t.Parallel()

	if got := chargeMicrosCeil(4096, 10_000_000); got != 40_960 {
		t.Fatalf("sonnet 5 4096 out = %d", got)
	}
	if got := chargeMicrosCeil(4096, 25_000_000); got != 102_400 {
		t.Fatalf("opus 5 4096 out = %d", got)
	}
	if got := chargeMicrosCeil(1, 70_000); got != 1 {
		t.Fatalf("cheap model 1 token should ceil to 1 micro, got %d", got)
	}
	if got := chargeMicrosCeil(0, 10_000_000); got != 0 {
		t.Fatalf("zero tokens = %d", got)
	}
}

func TestComputeHoldDefaultAndClamp(t *testing.T) {
	t.Parallel()

	// Sonnet 5: $2 in / $10 out. $1 available, 200 input tokens, no max_tokens.
	plan, err := ComputeHold(200, 0, 4096, 2_000_000, 10_000_000, 1_000_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaxTokens != 4096 {
		t.Fatalf("max tokens = %d", plan.MaxTokens)
	}
	if plan.AmountMicros != chargeMicrosCeil(200, 2_000_000)+chargeMicrosCeil(4096, 10_000_000) {
		t.Fatalf("hold = %d", plan.AmountMicros)
	}

	_, err = ComputeHold(200, 0, 4096, 2_000_000, 10_000_000, 5_000, 10_000)
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("below $0.01 floor: %v", err)
	}

	// Only $0.05 left — clamp output so the hold fits.
	plan, err = ComputeHold(200, 8192, 4096, 2_000_000, 10_000_000, 50_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaxTokens >= 8192 {
		t.Fatalf("expected clamped max tokens, got %d", plan.MaxTokens)
	}
	if plan.AmountMicros > 50_000 {
		t.Fatalf("hold %d exceeds available", plan.AmountMicros)
	}
}

func TestComputeHoldCannotAffordInput(t *testing.T) {
	t.Parallel()

	_, err := ComputeHold(200_000, 4096, 4096, 5_000_000, 25_000_000, 50_000, 10_000)
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("huge prompt should 402, got %v", err)
	}
}

func TestComputeHoldCapsAtMaxOutputTokens(t *testing.T) {
	t.Parallel()

	plan, err := ComputeHold(10, 1_000_000, 4096, 1_000_000, 1_000_000, 1_000_000_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaxTokens != MaxOutputTokens {
		t.Fatalf("max tokens = %d, want %d", plan.MaxTokens, MaxOutputTokens)
	}
}

func TestComputeHoldExactFloor(t *testing.T) {
	t.Parallel()

	plan, err := ComputeHold(0, 1, 4096, 1_000_000, 10_000, 10_000, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.MaxTokens != 1 {
		t.Fatalf("max tokens = %d", plan.MaxTokens)
	}
	if plan.AmountMicros > 10_000 {
		t.Fatalf("hold %d exceeds the $0.01 floor wallet", plan.AmountMicros)
	}
}
