package webhook

import "testing"

func TestNormalizeRefundStatus(t *testing.T) {
	cases := []struct {
		value     string
		isPartial bool
		want      string
	}{
		{"full", false, "full"},
		{"FULL", false, "full"},
		{"refunded", false, "full"},
		{"partial", false, "partial"},
		{"partially_refunded", false, "partial"},
		{"", true, "partial"},
		{"", false, "full"},
		{"unknown", true, "partial"},
		{"unknown", false, "full"},
	}
	for _, tc := range cases {
		if got := normalizeRefundStatus(tc.value, tc.isPartial); got != tc.want {
			t.Fatalf("normalizeRefundStatus(%q, %v) = %q, want %q", tc.value, tc.isPartial, got, tc.want)
		}
	}
}

func TestMergeRefundStatus(t *testing.T) {
	cases := []struct {
		webhook, payment, want string
	}{
		{"full", "partial", "full"},
		{"full", "", "full"},
		{"partial", "full", "full"},
		{"partial", "refunded", "full"},
		{"partial", "partial", "partial"},
		{"partial", "", "partial"},
		{"partial", "stale", "partial"},
		{"", "partial", "partial"},
		{"", "", "full"},
	}
	for _, tc := range cases {
		if got := mergeRefundStatus(tc.webhook, tc.payment); got != tc.want {
			t.Fatalf("mergeRefundStatus(%q, %q) = %q, want %q", tc.webhook, tc.payment, got, tc.want)
		}
	}
}
