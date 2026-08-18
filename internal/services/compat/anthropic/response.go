package anthropic

import (
	"encoding/json"

	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

type messageResponse struct {
	ID           string                 `json:"id"`
	Type         string                 `json:"type"`
	Role         string                 `json:"role"`
	Model        string                 `json:"model"`
	Content      []responseContentBlock `json:"content"`
	StopReason   string                 `json:"stop_reason"`
	StopSequence *string                `json:"stop_sequence"`
	Usage        responseUsage          `json:"usage"`
}

type responseContentBlock struct {
	Type  string         `json:"type"`
	Text  string         `json:"text,omitempty"`
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`
}

type responseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// stopReasonToAnthropic maps Bedrock's Converse stopReason values onto
// Anthropic's stop_reason enum.
func stopReasonToAnthropic(stopReason string) string {
	switch stopReason {
	case "tool_use":
		return "tool_use"
	case "max_tokens":
		return "max_tokens"
	case "stop_sequence":
		return "stop_sequence"
	case "content_filtered", "guardrail_intervened":
		return "refusal"
	default:
		return "end_turn"
	}
}

// NewMessageResponse formats a buffered Bedrock Converse result into an
// Anthropic message JSON body.
func NewMessageResponse(model string, result *inferenceService.ConverseResponse) ([]byte, error) {
	blocks := make([]responseContentBlock, 0, len(result.Content))
	for _, b := range result.Content {
		if b.Text != "" {
			blocks = append(blocks, responseContentBlock{Type: "text", Text: b.Text})
		}
		if b.ToolUse != nil {
			input := b.ToolUse.Input
			if input == nil {
				input = map[string]any{}
			}
			blocks = append(blocks, responseContentBlock{
				Type:  "tool_use",
				ID:    b.ToolUse.ToolUseID,
				Name:  b.ToolUse.Name,
				Input: input,
			})
		}
	}

	resp := messageResponse{
		ID:         "msg_" + billingService.NewIdempotencyKey(),
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    blocks,
		StopReason: stopReasonToAnthropic(result.StopReason),
		Usage:      responseUsage{InputTokens: result.InputTokens, OutputTokens: result.OutputTokens},
	}
	return json.Marshal(resp)
}
