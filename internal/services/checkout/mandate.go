package checkout

import "math"

// usdToInrRate is a conservative INR-per-USD used only to size the e-mandate.
// Cents × rate = paise (the /100 and ×100 cancel). A high rate keeps FX
// swings from pushing a GST-inclusive renewal above the bank authorization.
const usdToInrRate = 90

// mandateBuffer is 70% headroom on the converted plan price — GST (18%) plus
// extra room for rounding and FX. Same formula as Choppr.
const mandateBuffer = 1.7

// mandateMinAmountInrPaise converts a USD plan price in micros into Dodo's
// mandate_min_amount_inr_paise override. Returns 0 when the plan has no
// price so the field is omitted and Dodo's system default applies.
func mandateMinAmountInrPaise(priceMicros int64) int {
	if priceMicros <= 0 {
		return 0
	}
	usdCents := priceMicros / 10_000
	if usdCents <= 0 {
		return 0
	}
	inrPaise := usdCents * usdToInrRate
	return int(math.Ceil(float64(inrPaise) * mandateBuffer))
}
