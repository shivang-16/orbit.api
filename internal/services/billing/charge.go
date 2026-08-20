package billing

const microsPerMillion = 1_000_000

// MaxOutputTokens is the hard ceiling Orbit will send to Bedrock, even if
// the client asked for more. It bounds a hold so one request cannot freeze
// an entire wallet.
const MaxOutputTokens = 128_000

func chargeMicros(inputTokens, outputTokens int, inputPerMillion, outputPerMillion int64) int64 {
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	return (int64(inputTokens)*inputPerMillion + int64(outputTokens)*outputPerMillion) / microsPerMillion
}

func chargeMicrosCeil(tokens int, perMillion int64) int64 {
	if tokens <= 0 || perMillion <= 0 {
		return 0
	}
	return (int64(tokens)*perMillion + microsPerMillion - 1) / microsPerMillion
}

func maxTokensForBudget(budgetMicros, perMillion int64) int {
	if budgetMicros <= 0 || perMillion <= 0 {
		return 0
	}
	n := budgetMicros * microsPerMillion / perMillion
	if n > int64(MaxOutputTokens) {
		return MaxOutputTokens
	}
	return int(n)
}
