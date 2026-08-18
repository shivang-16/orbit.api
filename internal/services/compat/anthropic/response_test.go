package anthropic

import (
	"encoding/json"
	"testing"

	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

func TestNewMessageResponse_TextAndToolUse(t *testing.T) {
	result := &inferenceService.ConverseResponse{
		Content: []inferenceService.ContentBlock{
			{Text: "Let me check that for you."},
			{ToolUse: &inferenceService.ToolUseBlock{ToolUseID: "toolu_1", Name: "get_weather", Input: map[string]any{"city": "Delhi"}}},
		},
		StopReason:   "tool_use",
		InputTokens:  8,
		OutputTokens: 12,
	}
	body, err := NewMessageResponse("claude-opus-5", result)
	if err != nil {
		t.Fatalf("NewMessageResponse: %v", err)
	}

	var resp messageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Type != "message" || resp.Role != "assistant" {
		t.Fatalf("Type/Role = %q/%q", resp.Type, resp.Role)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("Content = %+v", resp.Content)
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "Let me check that for you." {
		t.Fatalf("Content[0] = %+v", resp.Content[0])
	}
	if resp.Content[1].Type != "tool_use" || resp.Content[1].ID != "toolu_1" || resp.Content[1].Name != "get_weather" {
		t.Fatalf("Content[1] = %+v", resp.Content[1])
	}
	if resp.Content[1].Input["city"] != "Delhi" {
		t.Fatalf("Content[1].Input = %+v", resp.Content[1].Input)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("StopReason = %q", resp.StopReason)
	}
	if resp.Usage.InputTokens != 8 || resp.Usage.OutputTokens != 12 {
		t.Fatalf("Usage = %+v", resp.Usage)
	}
}

func TestStopReasonToAnthropic(t *testing.T) {
	cases := map[string]string{
		"end_turn":             "end_turn",
		"tool_use":             "tool_use",
		"max_tokens":           "max_tokens",
		"stop_sequence":        "stop_sequence",
		"content_filtered":     "refusal",
		"guardrail_intervened": "refusal",
		"":                     "end_turn",
	}
	for in, want := range cases {
		if got := stopReasonToAnthropic(in); got != want {
			t.Errorf("stopReasonToAnthropic(%q) = %q, want %q", in, got, want)
		}
	}
}
