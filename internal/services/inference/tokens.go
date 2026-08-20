package inference

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

func EstimateInputTokens(text string) int {
	n := utf8.RuneCountInString(text)
	if n <= 0 {
		return 0
	}
	est := (n + 3) / 4
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
