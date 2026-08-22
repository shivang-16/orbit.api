package invoices

import "testing"

func TestStatusLabel(t *testing.T) {
	cases := []struct {
		status, refund, want string
	}{
		{"succeeded", "", "Paid"},
		{"succeeded", "full", "Refunded"},
		{"succeeded", "partial", "Partially refunded"},
		{"failed", "", "Failed"},
		{"processing", "", "Processing"},
		{"requires_payment_method", "", "Processing"},
		{"cancelled", "", "Cancelled"},
		{"partially_captured", "", "Partially paid"},
		{"", "", "Unknown"},
	}
	for _, tc := range cases {
		if got := statusLabel(tc.status, tc.refund); got != tc.want {
			t.Fatalf("statusLabel(%q, %q) = %q, want %q", tc.status, tc.refund, got, tc.want)
		}
	}
}

func TestPlanDisplayName(t *testing.T) {
	if got := planDisplayName("starter"); got != "Starter" {
		t.Fatalf("got %q", got)
	}
	if got := planDisplayName(""); got != "Plan purchase" {
		t.Fatalf("empty = %q", got)
	}
}
