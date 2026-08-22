package inference

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsRequestCanceled(t *testing.T) {
	t.Parallel()

	if isRequestCanceled(context.Canceled) != true {
		t.Fatal("Canceled should be a client stop")
	}
	if isRequestCanceled(context.DeadlineExceeded) {
		t.Fatal("DeadlineExceeded is our timeout, not a client stop")
	}
	if isRequestCanceled(fmt.Errorf("read: connection reset by peer")) {
		t.Fatal("connection reset on the read path is a provider drop")
	}
	if isRequestCanceled(nil) {
		t.Fatal("nil")
	}
}

func TestIsClientWriteAbort(t *testing.T) {
	t.Parallel()

	if !isClientWriteAbort(errors.New("write tcp: broken pipe")) {
		t.Fatal("broken pipe while flushing is a client hang-up")
	}
	if isClientWriteAbort(errors.New("read tcp: connection reset by peer")) {
		t.Fatal("connection reset must not be treated as a write abort")
	}
	if !isClientWriteAbort(fmt.Errorf("flush: %w", context.Canceled)) {
		t.Fatal("canceled write is a client hang-up")
	}
}

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
