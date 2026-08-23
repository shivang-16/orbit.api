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

func TestMergePriorSubscriptionIDs(t *testing.T) {
	cases := []struct {
		name       string
		stored     string
		incoming   string
		invoiceIDs []string
		want       []string
	}{
		{
			name:     "stored only, no invoices",
			stored:   "sub_trial",
			incoming: "sub_starter",
			want:     []string{"sub_trial"},
		},
		{
			name:       "stored set still includes invoice orphans",
			stored:     "sub_starter",
			incoming:   "sub_builder",
			invoiceIDs: []string{"sub_trial", "sub_starter"},
			want:       []string{"sub_starter", "sub_trial"},
		},
		{
			name:       "empty stored uses invoice history",
			stored:     "",
			incoming:   "sub_starter",
			invoiceIDs: []string{"sub_trial"},
			want:       []string{"sub_trial"},
		},
		{
			name:       "never cancel incoming even if it is on invoices",
			stored:     "sub_trial",
			incoming:   "sub_starter",
			invoiceIDs: []string{"sub_starter", "sub_trial", "sub_orphan"},
			want:       []string{"sub_trial", "sub_orphan"},
		},
		{
			name:       "dedupe blanks and repeats",
			stored:     "  sub_a  ",
			incoming:   "sub_new",
			invoiceIDs: []string{"", "sub_a", "  sub_b  ", "sub_b", "sub_new"},
			want:       []string{"sub_a", "sub_b"},
		},
		{
			name:       "keep path: stored equals incoming, still cancel invoice orphans",
			stored:     "sub_starter",
			incoming:   "sub_starter",
			invoiceIDs: []string{"sub_trial", "sub_starter"},
			want:       []string{"sub_trial"},
		},
		{
			name:     "nothing to cancel",
			stored:   "",
			incoming: "sub_trial",
			want:     []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mergePriorSubscriptionIDs(tc.stored, tc.incoming, tc.invoiceIDs)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestPickHigherPlan(t *testing.T) {
	if got := pickHigherPlan("starter", "trial", true, 2, true, 1); got != "starter" {
		t.Fatalf("upgrade in same request: %q", got)
	}
	if got := pickHigherPlan("trial", "starter", true, 1, true, 2); got != "starter" {
		t.Fatalf("keep org plan when incoming is lower: %q", got)
	}
	if got := pickHigherPlan("", "starter", false, 0, true, 2); got != "starter" {
		t.Fatalf("unknown incoming keeps org plan: %q", got)
	}
	if got := pickHigherPlan("starter", "", true, 2, false, 0); got != "starter" {
		t.Fatalf("known incoming when org plan missing: %q", got)
	}
}

func TestStaleInvoiceIDs(t *testing.T) {
	cases := []struct {
		name         string
		incoming     string
		currentOrder int
		currentKnown bool
		refs         []invoiceSubRef
		want         []string
	}{
		{
			name:         "cancel lower plan leftover",
			incoming:     "sub_starter",
			currentOrder: 2,
			currentKnown: true,
			refs: []invoiceSubRef{
				{ID: "sub_starter", Order: 2, Known: true},
				{ID: "sub_trial", Order: 1, Known: true},
			},
			want: []string{"sub_trial"},
		},
		{
			name:         "do not cancel deferred upgrade invoice",
			incoming:     "sub_trial",
			currentOrder: 1,
			currentKnown: true,
			refs: []invoiceSubRef{
				{ID: "sub_trial", Order: 1, Known: true},
				{ID: "sub_starter", Order: 2, Known: true},
			},
			want: []string{},
		},
		{
			name:         "do not cancel unknown-plan invoice",
			incoming:     "sub_starter",
			currentOrder: 2,
			currentKnown: true,
			refs: []invoiceSubRef{
				{ID: "sub_mystery", Order: 0, Known: false},
			},
			want: []string{},
		},
		{
			name:         "do not cancel when org plan unknown",
			incoming:     "sub_trial",
			currentKnown: false,
			refs: []invoiceSubRef{
				{ID: "sub_other", Order: 1, Known: true},
			},
			want: []string{},
		},
		{
			name:         "same-tier extra is not cancelled on keep",
			incoming:     "sub_a",
			currentOrder: 1,
			currentKnown: true,
			refs: []invoiceSubRef{
				{ID: "sub_b", Order: 1, Known: true},
			},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := staleInvoiceIDs(tc.incoming, tc.currentOrder, tc.currentKnown, tc.refs)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestSubscriptionPersistAction(t *testing.T) {
	cases := []struct {
		name                     string
		stored, incoming         string
		incomingKnown            bool
		incomingOrder, prevOrder int
		storedKnown              bool
		storedOrder              int
		want                     string
	}{
		{name: "same id", stored: "sub_new", incoming: "sub_new", incomingKnown: true, incomingOrder: 2, prevOrder: 2, want: persistKeep},
		{name: "same id leftover lower than org plan", stored: "sub_trial", incoming: "sub_trial", incomingKnown: true, incomingOrder: 1, prevOrder: 2, want: persistIgnoreStale},
		{name: "first purchase", stored: "", incoming: "sub_trial", incomingKnown: true, incomingOrder: 1, prevOrder: 0, want: persistAdopt},
		{name: "upgrade replaces stored", stored: "sub_trial", incoming: "sub_starter", incomingKnown: true, incomingOrder: 2, prevOrder: 1, storedKnown: true, storedOrder: 1, want: persistAdopt},
		{name: "retry after attachPlan already raised org", stored: "sub_trial", incoming: "sub_starter", incomingKnown: true, incomingOrder: 2, prevOrder: 2, storedKnown: true, storedOrder: 1, want: persistAdopt},
		{name: "upgrade without stored id", stored: "", incoming: "sub_starter", incomingKnown: true, incomingOrder: 2, prevOrder: 1, want: persistAdopt},
		{name: "stale old sub after upgrade", stored: "sub_starter", incoming: "sub_trial", incomingKnown: true, incomingOrder: 1, prevOrder: 2, storedKnown: true, storedOrder: 2, want: persistIgnoreStale},
		{name: "stale old sub before id stored", stored: "", incoming: "sub_trial", incomingKnown: true, incomingOrder: 1, prevOrder: 2, want: persistIgnoreStale},
		{name: "same-tier extra sub", stored: "sub_a", incoming: "sub_b", incomingKnown: true, incomingOrder: 1, prevOrder: 1, storedKnown: true, storedOrder: 1, want: persistIgnoreStale},
		{name: "unknown plan does not cancel paid upgrade", stored: "sub_trial", incoming: "sub_starter", incomingKnown: false, incomingOrder: 0, prevOrder: 1, want: persistDefer},
		{name: "unknown plan does not overwrite newer stored id", stored: "sub_starter", incoming: "sub_trial", incomingKnown: false, incomingOrder: 0, prevOrder: 2, want: persistDefer},
		{name: "unknown plan first purchase still adopts", stored: "", incoming: "sub_trial", incomingKnown: false, incomingOrder: 0, prevOrder: 0, want: persistAdopt},
		{name: "unknown plan with org plan and no stored id defers", stored: "", incoming: "sub_starter", incomingKnown: false, incomingOrder: 0, prevOrder: 1, want: persistDefer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := subscriptionPersistAction(tc.stored, tc.incoming, tc.incomingKnown, tc.incomingOrder, tc.prevOrder, tc.storedKnown, tc.storedOrder); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDodoObjectPaymentID(t *testing.T) {
	if got := (dodoObject{PaymentID: "pay_1", ID: "sub_1"}).paymentID(); got != "pay_1" {
		t.Fatalf("explicit payment id: %q", got)
	}
	if got := (dodoObject{PayloadType: "Payment", ID: "pay_2"}).paymentID(); got != "pay_2" {
		t.Fatalf("payment payload id: %q", got)
	}
	if got := (dodoObject{PayloadType: "Subscription", ID: "sub_1"}).paymentID(); got != "" {
		t.Fatalf("subscription id must not be used as payment id: %q", got)
	}
}

func TestDodoObjectAmount(t *testing.T) {
	if got := (dodoObject{TotalAmount: 500, Amount: 1}).amount(); got != 500 {
		t.Fatalf("prefer total_amount: %d", got)
	}
	if got := (dodoObject{Amount: 500}).amount(); got != 500 {
		t.Fatalf("fallback amount: %d", got)
	}
}

func TestSubscriptionGrantKey(t *testing.T) {
	cases := []struct {
		name           string
		subscriptionID string
		incomingPlan   string
		currentPlan    string
		legacyGranted  bool
		want           string
	}{
		{
			name:         "no subscription id",
			incomingPlan: "trial",
			want:         "dodo_event:subscription.active:2026-08:org-1",
		},
		{
			name:           "legacy renewal same month",
			subscriptionID: "sub_1",
			incomingPlan:   "trial",
			currentPlan:    "trial",
			legacyGranted:  true,
			want:           "dodo_period:sub_1:2026-08",
		},
		{
			name:           "upgrade to new plan",
			subscriptionID: "sub_1",
			incomingPlan:   "starter",
			currentPlan:    "trial",
			legacyGranted:  true,
			want:           "dodo_period:sub_1:starter:2026-08",
		},
		{
			name:           "new month renewal",
			subscriptionID: "sub_1",
			incomingPlan:   "trial",
			currentPlan:    "trial",
			legacyGranted:  false,
			want:           "dodo_period:sub_1:trial:2026-08",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := subscriptionGrantKey(tc.subscriptionID, tc.incomingPlan, tc.currentPlan, "subscription.active", "2026-08", "org-1", tc.legacyGranted)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
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
