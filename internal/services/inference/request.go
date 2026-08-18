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
	// Stream defaults to false (one buffered JSON response). Pass
	// "stream": true to receive tokens as server-sent events instead.
	Stream *bool `json:"stream,omitempty"`
}

// WantsStream reports whether the caller asked for a streamed (SSE)
// response. Omitted or false both mean buffered JSON.
func (r ChatRequest) WantsStream() bool {
	return r.Stream != nil && *r.Stream
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
