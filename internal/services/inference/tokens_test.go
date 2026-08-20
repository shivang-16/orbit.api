package inference

import "testing"

func TestEstimateInputTokensBuffer(t *testing.T) {
	t.Parallel()

	if got := EstimateInputTokens(""); got != 0 {
		t.Fatalf("empty = %d", got)
	}
	// 16 runes → 4 tokens raw, +20% = 4.
	if got := EstimateInputTokens("abcdefghijklmnop"); got != 4 {
		t.Fatalf("16 chars = %d", got)
	}
	if got := EstimateInputTokens("abcd"); got < 1 {
		t.Fatalf("tiny prompt = %d", got)
	}
}
