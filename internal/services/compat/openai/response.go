package openai

import (
	"encoding/json"
	"time"

	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type chatCompletionResponse struct {
	ID                string           `json:"id"`
	Object            string           `json:"object"`
	Created           int64            `json:"created"`
	Model             string           `json:"model"`
	Choices           []responseChoice `json:"choices"`
	Usage             *responseUsage   `json:"usage,omitempty"`
	SystemFingerprint string           `json:"system_fingerprint,omitempty"`
}

type responseChoice struct {
	Index        int             `json:"index"`
	Message      responseMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type responseMessage struct {
	Role      string                 `json:"role"`
	Content   *string                `json:"content"`
	ToolCalls []responseToolCallItem `json:"tool_calls,omitempty"`
}

type responseToolCallItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type responseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// stopReasonToFinishReason maps Bedrock's Converse stopReason values onto
// OpenAI's finish_reason enum.
func stopReasonToFinishReason(stopReason string) string {
	switch stopReason {
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	case "content_filtered", "guardrail_intervened":
		return "content_filter"
	default:
		return "stop"
	}
}

// NewChatCompletionResponse formats a buffered Bedrock Converse result
// into an OpenAI chat.completion JSON body.
func NewChatCompletionResponse(model string, result *inferenceService.ConverseResponse) ([]byte, error) {
	var textParts []string
	var toolCalls []responseToolCallItem
	for _, block := range result.Content {
		if block.Text != "" {
			textParts = append(textParts, block.Text)
		}
		if block.ToolUse != nil {
			args, err := json.Marshal(block.ToolUse.Input)
			if err != nil {
				args = []byte("{}")
			}
			item := responseToolCallItem{ID: block.ToolUse.ToolUseID, Type: "function"}
			item.Function.Name = block.ToolUse.Name
			item.Function.Arguments = string(args)
			toolCalls = append(toolCalls, item)
		}
	}

	var content *string
	if len(textParts) > 0 || len(toolCalls) == 0 {
		joined := joinText(textParts)
		content = &joined
	}

	resp := chatCompletionResponse{
		ID:      "chatcmpl-" + billingService.NewIdempotencyKey(),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []responseChoice{{
			Index: 0,
			Message: responseMessage{
				Role:      "assistant",
				Content:   content,
				ToolCalls: toolCalls,
			},
			FinishReason: stopReasonToFinishReason(result.StopReason),
		}},
		Usage: &responseUsage{
			PromptTokens:     result.InputTokens,
			CompletionTokens: result.OutputTokens,
			TotalTokens:      result.InputTokens + result.OutputTokens,
		},
	}
	return json.Marshal(resp)
}

func joinText(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "\n"
		}
		out += p
	}
	return out
}
