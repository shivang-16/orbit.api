package anthropic

import (
	"encoding/json"
	"testing"
)

func TestMessagesRequest_IsValid(t *testing.T) {
	cases := []struct {
		name string
		req  MessagesRequest
		want bool
	}{
		{"valid", MessagesRequest{Model: "claude-opus-5", MaxTokens: 100, Messages: []requestMessage{{Role: "user"}}}, true},
		{"missing model", MessagesRequest{MaxTokens: 100, Messages: []requestMessage{{Role: "user"}}}, false},
		{"missing max_tokens", MessagesRequest{Model: "claude-opus-5", Messages: []requestMessage{{Role: "user"}}}, false},
		{"missing messages", MessagesRequest{Model: "claude-opus-5", MaxTokens: 100}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.IsValid(); got != tc.want {
				t.Fatalf("IsValid() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestBlocksText(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain string", `"hi there"`, "hi there"},
		{"empty", ``, ""},
		{"array of text blocks", `[{"type":"text","text":"a"},{"type":"text","text":"b"}]`, "a\nb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := blocksText(json.RawMessage(tc.raw)); got != tc.want {
				t.Fatalf("blocksText(%s) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestToConverse_SystemAndTextMessage(t *testing.T) {
	req := MessagesRequest{
		Model:     "claude-opus-5",
		MaxTokens: 512,
		System:    json.RawMessage(`"You are terse."`),
		Messages: []requestMessage{
			{Role: "user", Content: json.RawMessage(`"Hello"`)},
		},
	}
	out := req.ToConverse()
	if out.System != "You are terse." {
		t.Fatalf("System = %q", out.System)
	}
	if len(out.Messages) != 1 || out.Messages[0].Role != "user" {
		t.Fatalf("Messages = %+v", out.Messages)
	}
	if out.MaxTokens == nil || *out.MaxTokens != 512 {
		t.Fatalf("MaxTokens = %v", out.MaxTokens)
	}
}

func TestToConverse_ToolUseAndToolResult(t *testing.T) {
	req := MessagesRequest{
		Model:     "claude-opus-5",
		MaxTokens: 512,
		Messages: []requestMessage{
			{Role: "user", Content: json.RawMessage(`"What's the weather in Delhi?"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Delhi"}}]`)},
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"30C"}]`)},
		},
		Tools: []requestTool{{Name: "get_weather", Description: "Get current weather", InputSchema: map[string]any{"type": "object"}}},
	}
	out := req.ToConverse()

	if len(out.Messages) != 3 {
		t.Fatalf("Messages = %+v", out.Messages)
	}
	assistantMsg := out.Messages[1]
	if assistantMsg.Content[0].ToolUse == nil || assistantMsg.Content[0].ToolUse.ToolUseID != "toolu_1" {
		t.Fatalf("assistant tool_use = %+v", assistantMsg.Content[0])
	}
	if assistantMsg.Content[0].ToolUse.Input["city"] != "Delhi" {
		t.Fatalf("tool_use input = %+v", assistantMsg.Content[0].ToolUse.Input)
	}

	toolResultMsg := out.Messages[2]
	if toolResultMsg.Role != "user" {
		t.Fatalf("tool result message role = %q", toolResultMsg.Role)
	}
	tr := toolResultMsg.Content[0].ToolResult
	if tr == nil || tr.ToolUseID != "toolu_1" || tr.Status != "success" {
		t.Fatalf("tool result = %+v", tr)
	}
	if len(tr.Content) != 1 || tr.Content[0].Text != "30C" {
		t.Fatalf("tool result content = %+v", tr.Content)
	}

	if len(out.Tools) != 1 || out.Tools[0].Name != "get_weather" {
		t.Fatalf("Tools = %+v", out.Tools)
	}
}

func TestToConverse_ToolResultIsError(t *testing.T) {
	req := MessagesRequest{
		Model:     "claude-opus-5",
		MaxTokens: 512,
		Messages: []requestMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"boom","is_error":true}]`)},
		},
	}
	out := req.ToConverse()
	if len(out.Messages) != 1 {
		t.Fatalf("Messages = %+v", out.Messages)
	}
	tr := out.Messages[0].Content[0].ToolResult
	if tr == nil || tr.Status != "error" {
		t.Fatalf("expected error status tool result, got %+v", tr)
	}
}

func TestToConverse_ToolChoiceModes(t *testing.T) {
	base := MessagesRequest{
		Model:     "claude-opus-5",
		MaxTokens: 100,
		Messages:  []requestMessage{{Role: "user", Content: json.RawMessage(`"hi"`)}},
		Tools:     []requestTool{{Name: "get_weather"}},
	}

	anyChoice := base
	anyChoice.ToolChoice = &requestToolChoice{Type: "any"}
	if out := anyChoice.ToConverse(); out.ToolChoice == nil || out.ToolChoice.Mode != "any" {
		t.Fatalf("any: %+v", out.ToolChoice)
	}

	named := base
	named.ToolChoice = &requestToolChoice{Type: "tool", Name: "get_weather"}
	if out := named.ToConverse(); out.ToolChoice == nil || out.ToolChoice.Mode != "tool" || out.ToolChoice.ToolName != "get_weather" {
		t.Fatalf("tool: %+v", out.ToolChoice)
	}

	none := base
	none.ToolChoice = &requestToolChoice{Type: "none"}
	if out := none.ToConverse(); len(out.Tools) != 0 {
		t.Fatalf("none: expected tools dropped, got %+v", out.Tools)
	}
}
