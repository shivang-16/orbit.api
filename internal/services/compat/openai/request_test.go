package openai

import (
	"encoding/json"
	"testing"
)

func TestChatCompletionRequest_IsValid(t *testing.T) {
	cases := []struct {
		name string
		req  ChatCompletionRequest
		want bool
	}{
		{"valid", ChatCompletionRequest{Model: "gpt-5.6-sol", Messages: []requestMessage{{Role: "user"}}}, true},
		{"missing model", ChatCompletionRequest{Messages: []requestMessage{{Role: "user"}}}, false},
		{"missing messages", ChatCompletionRequest{Model: "gpt-5.6-sol"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.IsValid(); got != tc.want {
				t.Fatalf("IsValid() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestContentText(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain string", `"hello there"`, "hello there"},
		{"null", `null`, ""},
		{"array of text parts", `[{"type":"text","text":"a"},{"type":"text","text":"b"}]`, "a\nb"},
		{"array skips non-text parts", `[{"type":"image_url","image_url":{"url":"x"}},{"type":"text","text":"only this"}]`, "only this"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := contentText(json.RawMessage(tc.raw)); got != tc.want {
				t.Fatalf("contentText(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestStopSequences(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", ``, nil},
		{"single string", `"STOP"`, []string{"STOP"}},
		{"empty string", `""`, nil},
		{"array", `["a","b"]`, []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stopSequences(json.RawMessage(tc.raw))
			if len(got) != len(tc.want) {
				t.Fatalf("stopSequences(%s) = %v, want %v", tc.raw, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("stopSequences(%s) = %v, want %v", tc.raw, got, tc.want)
				}
			}
		})
	}
}

func TestToolChoice(t *testing.T) {
	choice, drop := toolChoice(json.RawMessage(`"none"`))
	if !drop || choice != nil {
		t.Fatalf(`tool_choice "none": choice=%v drop=%t, want nil/true`, choice, drop)
	}

	choice, drop = toolChoice(json.RawMessage(`"required"`))
	if drop || choice == nil || choice.Mode != "any" {
		t.Fatalf(`tool_choice "required": choice=%v drop=%t, want Mode=any/false`, choice, drop)
	}

	choice, drop = toolChoice(json.RawMessage(`"auto"`))
	if drop || choice == nil || choice.Mode != "auto" {
		t.Fatalf(`tool_choice "auto": choice=%v drop=%t, want Mode=auto/false`, choice, drop)
	}

	choice, drop = toolChoice(json.RawMessage(`{"type":"function","function":{"name":"get_weather"}}`))
	if drop || choice == nil || choice.Mode != "tool" || choice.ToolName != "get_weather" {
		t.Fatalf("tool_choice named function: choice=%+v drop=%t", choice, drop)
	}

	choice, drop = toolChoice(nil)
	if drop || choice != nil {
		t.Fatalf("tool_choice absent: choice=%v drop=%t, want nil/false", choice, drop)
	}
}

func TestToConverse_SystemAndUserMessage(t *testing.T) {
	req := ChatCompletionRequest{
		Model: "claude-opus-5",
		Messages: []requestMessage{
			{Role: "system", Content: json.RawMessage(`"You are a helpful assistant."`)},
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}
	out := req.ToConverse()
	if out.System != "You are a helpful assistant." {
		t.Fatalf("System = %q", out.System)
	}
	if len(out.Messages) != 1 || out.Messages[0].Role != "user" {
		t.Fatalf("Messages = %+v", out.Messages)
	}
	if len(out.Messages[0].Content) != 1 || out.Messages[0].Content[0].Text != "Hello" {
		t.Fatalf("Messages[0].Content = %+v", out.Messages[0].Content)
	}
}

func TestToConverse_AssistantToolCallsAndToolResults(t *testing.T) {
	req := ChatCompletionRequest{
		Model: "gpt-5.6-sol",
		Messages: []requestMessage{
			{Role: "user", Content: json.RawMessage(`"What's the weather in two cities?"`)},
			{
				Role:    "assistant",
				Content: json.RawMessage(`null`),
				ToolCalls: []requestToolCall{
					{ID: "call_1", Type: "function", Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: "get_weather", Arguments: `{"city":"Delhi"}`}},
					{ID: "call_2", Type: "function", Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: "get_weather", Arguments: `{"city":"Mumbai"}`}},
				},
			},
			{Role: "tool", ToolCallID: "call_1", Content: json.RawMessage(`"30C"`)},
			{Role: "tool", ToolCallID: "call_2", Content: json.RawMessage(`"32C"`)},
		},
	}
	out := req.ToConverse()

	if len(out.Messages) != 3 {
		t.Fatalf("expected 3 converse messages (user, assistant, coalesced tool-results user), got %d: %+v", len(out.Messages), out.Messages)
	}

	assistantMsg := out.Messages[1]
	if assistantMsg.Role != "assistant" || len(assistantMsg.Content) != 2 {
		t.Fatalf("assistant message = %+v", assistantMsg)
	}
	if assistantMsg.Content[0].ToolUse == nil || assistantMsg.Content[0].ToolUse.Name != "get_weather" {
		t.Fatalf("assistant tool use[0] = %+v", assistantMsg.Content[0])
	}
	if assistantMsg.Content[0].ToolUse.Input["city"] != "Delhi" {
		t.Fatalf("assistant tool use[0] input = %+v", assistantMsg.Content[0].ToolUse.Input)
	}

	toolResultsMsg := out.Messages[2]
	if toolResultsMsg.Role != "user" || len(toolResultsMsg.Content) != 2 {
		t.Fatalf("expected both tool results coalesced into one user message, got %+v", toolResultsMsg)
	}
	if toolResultsMsg.Content[0].ToolResult == nil || toolResultsMsg.Content[0].ToolResult.ToolUseID != "call_1" {
		t.Fatalf("tool result[0] = %+v", toolResultsMsg.Content[0])
	}
	if toolResultsMsg.Content[1].ToolResult == nil || toolResultsMsg.Content[1].ToolResult.ToolUseID != "call_2" {
		t.Fatalf("tool result[1] = %+v", toolResultsMsg.Content[1])
	}
}

func TestToConverse_ToolChoiceNoneDropsTools(t *testing.T) {
	req := ChatCompletionRequest{
		Model:    "gpt-5.6-sol",
		Messages: []requestMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools: []requestTool{{Type: "function", Function: struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			Parameters  map[string]any `json:"parameters"`
		}{Name: "get_weather"}}},
		ToolChoice: json.RawMessage(`"none"`),
	}
	out := req.ToConverse()
	if len(out.Tools) != 0 || out.ToolChoice != nil {
		t.Fatalf("expected tools dropped, got Tools=%+v ToolChoice=%+v", out.Tools, out.ToolChoice)
	}
}

func TestToConverse_MaxTokensPrefersCompletionTokens(t *testing.T) {
	legacy := 10
	completion := 20
	req := ChatCompletionRequest{
		Model:               "gpt-5.6-sol",
		Messages:            []requestMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		MaxTokens:           &legacy,
		MaxCompletionTokens: &completion,
	}
	out := req.ToConverse()
	if out.MaxTokens == nil || *out.MaxTokens != completion {
		t.Fatalf("MaxTokens = %v, want %d", out.MaxTokens, completion)
	}
}
