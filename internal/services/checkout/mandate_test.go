package checkout

import "testing"

func TestMandateMinAmountInrPaise(t *testing.T) {
	cases := []struct {
		name        string
		priceMicros int64
		want        int
	}{
		{name: "zero price omits override", priceMicros: 0, want: 0},
		{name: "negative price omits override", priceMicros: -1, want: 0},
		{name: "sub-cent price omits override", priceMicros: 9_999, want: 0},
		// $5 → 500¢ × 90 = 45,000 paise × 1.7 = ₹765 (~$8.50)
		{name: "trial $5", priceMicros: 5_000_000, want: 76_500},
		// $20 → 2,000¢ × 90 = 180,000 paise × 1.7 = ₹3,060
		{name: "starter $20", priceMicros: 20_000_000, want: 306_000},
		// $60 → 6,000¢ × 90 = 540,000 paise × 1.7 = ₹9,180
		{name: "builder $60", priceMicros: 60_000_000, want: 918_000},
		// $200 → 20,000¢ × 90 = 1,800,000 paise × 1.7 = ₹30,600
		{name: "pro $200", priceMicros: 200_000_000, want: 3_060_000},
		// $500 → 50,000¢ × 90 = 4,500,000 paise × 1.7 = ₹76,500
		{name: "business $500", priceMicros: 500_000_000, want: 7_650_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mandateMinAmountInrPaise(tc.priceMicros); got != tc.want {
				t.Fatalf("mandateMinAmountInrPaise(%d) = %d, want %d", tc.priceMicros, got, tc.want)
			}
		})
	}
}
