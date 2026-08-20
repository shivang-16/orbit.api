package billing

import (
	"errors"
	"fmt"
)

var ErrInsufficientCredits = errors.New("low on credits")

type HoldPlan struct {
	AmountMicros int64
	MaxTokens    int
}

// ComputeHold sizes a freeze: ceil(input estimate) + ceil(output cap).
// The output cap is the client's max_tokens (or defaultMax), clamped so
// the freeze fits remaining and never exceeds MaxOutputTokens. After the
// call, remaining and credits_used move by the ledger's actual (floor),
// not by the freeze: unused freeze is returned; extra input is deducted.
func ComputeHold(
	inputTokens, requestedMax, defaultMax int,
	inputPerMillion, outputPerMillion, available, threshold int64,
) (HoldPlan, error) {
	if available < threshold {
		return HoldPlan{}, fmt.Errorf("%w", ErrInsufficientCredits)
	}

	if inputTokens < 0 {
		inputTokens = 0
	}
	if defaultMax < 1 {
		defaultMax = 4096
	}
	if requestedMax <= 0 {
		requestedMax = defaultMax
	}
	if requestedMax > MaxOutputTokens {
		requestedMax = MaxOutputTokens
	}

	inputHold := chargeMicrosCeil(inputTokens, inputPerMillion)
	if inputHold >= available {
		return HoldPlan{}, fmt.Errorf("%w", ErrInsufficientCredits)
	}

	outBudget := available - inputHold
	affordableOut := maxTokensForBudget(outBudget, outputPerMillion)
	if outputPerMillion <= 0 {
		affordableOut = requestedMax
	}
	if affordableOut < 1 {
		return HoldPlan{}, fmt.Errorf("%w", ErrInsufficientCredits)
	}
	if requestedMax > affordableOut {
		requestedMax = affordableOut
	}

	outputHold := chargeMicrosCeil(requestedMax, outputPerMillion)
	hold := inputHold + outputHold
	if hold < 1 {
		hold = 1
	}
	if hold > available {
		return HoldPlan{}, fmt.Errorf("%w", ErrInsufficientCredits)
	}

	return HoldPlan{AmountMicros: hold, MaxTokens: requestedMax}, nil
}
