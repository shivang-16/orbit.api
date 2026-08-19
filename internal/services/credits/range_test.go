package credits

import (
	"testing"
	"time"
)

func TestParseUsageRange(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 30, 0, 0, time.UTC)

	tests := []struct {
		preset string
		from   string
		to     string
	}{
		{"", "2026-08-12", "2026-08-19"},
		{"7d", "2026-08-12", "2026-08-19"},
		{"1d", "2026-08-18", "2026-08-19"},
		{"30d", "2026-07-20", "2026-08-19"},
		{"mtd", "2026-08-01", "2026-08-19"},
		{"last_month", "2026-07-01", "2026-08-01"},
	}

	for _, test := range tests {
		got, err := parseUsageRange(test.preset, now)
		if err != nil {
			t.Fatalf("preset %q: %v", test.preset, err)
		}
		if got.From.Format("2006-01-02") != test.from {
			t.Errorf("preset %q from = %s, want %s", test.preset, got.From.Format("2006-01-02"), test.from)
		}
		if got.To.Format("2006-01-02") != test.to {
			t.Errorf("preset %q to = %s, want %s", test.preset, got.To.Format("2006-01-02"), test.to)
		}
	}

	if _, err := parseUsageRange("week", now); err != ErrInvalidRange {
		t.Fatalf("expected ErrInvalidRange, got %v", err)
	}
}

func TestParseUsagePage(t *testing.T) {
	page, limit, err := parseUsagePage("", "")
	if err != nil || page != 1 || limit != 25 {
		t.Fatalf("defaults: page=%d limit=%d err=%v", page, limit, err)
	}
	page, limit, err = parseUsagePage("3", "50")
	if err != nil || page != 3 || limit != 50 {
		t.Fatalf("explicit: page=%d limit=%d err=%v", page, limit, err)
	}
	if _, _, err := parseUsagePage("0", "25"); err != ErrInvalidPage {
		t.Fatalf("page 0: %v", err)
	}
	if _, _, err := parseUsagePage("1", "100"); err != ErrInvalidPage {
		t.Fatalf("limit 100: %v", err)
	}
	if _, _, err := parseUsagePage("x", "25"); err != ErrInvalidPage {
		t.Fatalf("bad page: %v", err)
	}
}

func TestEachUTCDay(t *testing.T) {
	from := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	days := eachUTCDay(from, to)
	if len(days) != 3 {
		t.Fatalf("got %d days, want 3", len(days))
	}
	if days[0].Format("2006-01-02") != "2026-08-16" || days[2].Format("2006-01-02") != "2026-08-18" {
		t.Fatalf("unexpected days: %v %v", days[0], days[2])
	}
}
