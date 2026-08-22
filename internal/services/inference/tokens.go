package inference

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// EstimateTokens is a coarse rune/4 count used when Bedrock never sent
// a usage frame (the client stopped the stream before metadata). It is
// not padded — billing should not invent extra tokens.
func EstimateTokens(text string) int {
	n := utf8.RuneCountInString(text)
	if n <= 0 {
		return 0
	}
	return (n + 3) / 4
}

func EstimateInputTokens(text string) int {
	est := EstimateTokens(text)
	if est <= 0 {
		return 0
	}
	buffered := est + est/5
	if buffered < 1 {
		return 1
	}
	return buffered
}

func chatInputText(req ChatRequest) string {
	var b strings.Builder
	for _, message := range req.Messages {
		b.WriteString(message.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

func converseInputText(req ConverseRequest) string {
	var b strings.Builder
	b.WriteString(req.System)
	b.WriteByte('\n')
	for _, message := range req.Messages {
		for _, block := range message.Content {
			b.WriteString(block.Text)
			if block.ToolUse != nil {
				b.WriteString(block.ToolUse.Name)
				if raw, err := json.Marshal(block.ToolUse.Input); err == nil {
					b.Write(raw)
				}
			}
			if block.ToolResult != nil {
				for _, inner := range block.ToolResult.Content {
					b.WriteString(inner.Text)
				}
			}
		}
	}
	if raw, err := json.Marshal(req.Tools); err == nil {
		b.Write(raw)
	}
	return b.String()
}

func fillCancelledInput(result *ChatResult, prompt string) {
	if result == nil || !result.Cancelled || result.InputTokens > 0 {
		return
	}
	result.InputTokens = EstimateTokens(prompt)
}

// isRequestCanceled is a caller hang-up (playground Stop, aborted
// fetch). DeadlineExceeded is our own inference timeout (chi
// middleware + http.Client), not a user stop — that must stay a
// stream error so the hold is refunded instead of billed.
func isRequestCanceled(err error) bool {
	return err != nil && errors.Is(err, context.Canceled)
}

// isClientWriteAbort is the client disappearing while we Flush a
// frame (broken pipe). "connection reset" is intentionally not
// matched here — on the Bedrock/Mantle *read* path that usually
// means the provider dropped the body.
func isClientWriteAbort(err error) bool {
	if err == nil {
		return false
	}
	if isRequestCanceled(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "broken pipe")
}
