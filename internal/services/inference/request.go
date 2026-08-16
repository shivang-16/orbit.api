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
