package openai

import (
	"encoding/json"
	"testing"

	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

func TestNewChatCompletionResponse_TextOnly(t *testing.T) {
	result := &inferenceService.ConverseResponse{
		Content:      []inferenceService.ContentBlock{{Text: "hello world"}},
		StopReason:   "end_turn",
		InputTokens:  10,
		OutputTokens: 5,
	}
	body, err := NewChatCompletionResponse("claude-opus-5", result)
	if err != nil {
		t.Fatalf("NewChatCompletionResponse: %v", err)
	}

	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Object != "chat.completion" {
		t.Fatalf("Object = %q", resp.Object)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("Choices = %+v", resp.Choices)
	}
	choice := resp.Choices[0]
	if choice.Message.Content == nil || *choice.Message.Content != "hello world" {
		t.Fatalf("Content = %v", choice.Message.Content)
	}
	if len(choice.Message.ToolCalls) != 0 {
		t.Fatalf("expected no tool calls, got %+v", choice.Message.ToolCalls)
	}
	if choice.FinishReason != "stop" {
		t.Fatalf("FinishReason = %q, want stop", choice.FinishReason)
	}
	if resp.Usage.PromptTokens != 10 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 15 {
		t.Fatalf("Usage = %+v", resp.Usage)
	}
}

func TestNewChatCompletionResponse_ToolUse(t *testing.T) {
	result := &inferenceService.ConverseResponse{
		Content: []inferenceService.ContentBlock{
			{ToolUse: &inferenceService.ToolUseBlock{
				ToolUseID: "tooluse_1",
				Name:      "get_weather",
				Input:     map[string]any{"city": "Delhi"},
			}},
		},
		StopReason: "tool_use",
	}
	body, err := NewChatCompletionResponse("gpt-5.6-sol", result)
	if err != nil {
		t.Fatalf("NewChatCompletionResponse: %v", err)
	}

	var resp chatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	choice := resp.Choices[0]
	if choice.Message.Content != nil {
		t.Fatalf("expected nil content for a tool-only response, got %v", *choice.Message.Content)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v", choice.Message.ToolCalls)
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "tooluse_1" || tc.Type != "function" || tc.Function.Name != "get_weather" {
		t.Fatalf("tool call = %+v", tc)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("arguments not valid JSON: %v (%q)", err, tc.Function.Arguments)
	}
	if args["city"] != "Delhi" {
		t.Fatalf("arguments = %v", args)
	}
	if choice.FinishReason != "tool_calls" {
		t.Fatalf("FinishReason = %q, want tool_calls", choice.FinishReason)
	}
}

func TestStopReasonToFinishReason(t *testing.T) {
	cases := map[string]string{
		"end_turn":             "stop",
		"tool_use":             "tool_calls",
		"max_tokens":           "length",
		"stop_sequence":        "stop",
		"content_filtered":     "content_filter",
		"guardrail_intervened": "content_filter",
		"":                     "stop",
	}
	for in, want := range cases {
		if got := stopReasonToFinishReason(in); got != want {
			t.Errorf("stopReasonToFinishReason(%q) = %q, want %q", in, got, want)
		}
	}
}
