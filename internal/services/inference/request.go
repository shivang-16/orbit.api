package inference

import "strings"

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

func (r ChatRequest) Prompt() string {
	for i := len(r.Messages) - 1; i >= 0; i-- {
		if r.Messages[i].Role == "user" {
			return truncate(strings.TrimSpace(r.Messages[i].Content), 4000)
		}
	}
	return ""
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func (r ChatRequest) isValid() bool {
	if len(r.Messages) == 0 {
		return false
	}
	for _, message := range r.Messages {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" {
			return false
		}
		if strings.TrimSpace(message.Content) == "" {
			return false
		}
	}
	return true
}
