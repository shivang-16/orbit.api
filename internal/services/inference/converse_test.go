package inference

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConverseRequest_IsValid(t *testing.T) {
	cases := []struct {
		name string
		req  ConverseRequest
		want bool
	}{
		{"valid", ConverseRequest{Messages: []ConverseMessage{{Role: "user", Content: []ContentBlock{{Text: "hi"}}}}}, true},
		{"no messages", ConverseRequest{}, false},
		{"bad role", ConverseRequest{Messages: []ConverseMessage{{Role: "system", Content: []ContentBlock{{Text: "hi"}}}}}, false},
		{"empty content", ConverseRequest{Messages: []ConverseMessage{{Role: "user"}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.req.isValid(); got != tc.want {
				t.Fatalf("isValid() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestConverseBody_TextMessageAndSystem(t *testing.T) {
	maxTokens := 100
	temp := 0.5
	body, err := converseBody(ConverseRequest{
		Messages:    []ConverseMessage{{Role: "user", Content: []ContentBlock{{Text: "hello"}}}},
		System:      "be terse",
		MaxTokens:   &maxTokens,
		Temperature: &temp,
	})
	if err != nil {
		t.Fatalf("converseBody: %v", err)
	}

	var parsed struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		InferenceConfig struct {
			MaxTokens   int     `json:"maxTokens"`
			Temperature float64 `json:"temperature"`
		} `json:"inferenceConfig"`
		ToolConfig json.RawMessage `json:"toolConfig"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal wire body: %v", err)
	}
	if len(parsed.Messages) != 1 || parsed.Messages[0].Role != "user" || parsed.Messages[0].Content[0].Text != "hello" {
		t.Fatalf("messages = %+v", parsed.Messages)
	}
	if len(parsed.System) != 1 || parsed.System[0].Text != "be terse" {
		t.Fatalf("system = %+v", parsed.System)
	}
	if parsed.InferenceConfig.MaxTokens != 100 || parsed.InferenceConfig.Temperature != 0.5 {
		t.Fatalf("inferenceConfig = %+v", parsed.InferenceConfig)
	}
	if parsed.ToolConfig != nil {
		t.Fatalf("expected no toolConfig when no tools given, got %s", parsed.ToolConfig)
	}
}

func TestConverseBody_ToolsAndToolChoice(t *testing.T) {
	body, err := converseBody(ConverseRequest{
		Messages: []ConverseMessage{{Role: "user", Content: []ContentBlock{{Text: "weather?"}}}},
		Tools: []ToolSpec{{
			Name:        "get_weather",
			Description: "Get current weather",
			InputSchema: map[string]any{"type": "object"},
		}},
		ToolChoice: &ToolChoice{Mode: "tool", ToolName: "get_weather"},
	})
	if err != nil {
		t.Fatalf("converseBody: %v", err)
	}

	var parsed struct {
		ToolConfig struct {
			Tools []struct {
				ToolSpec struct {
					Name        string `json:"name"`
					Description string `json:"description"`
					InputSchema struct {
						JSON map[string]any `json:"json"`
					} `json:"inputSchema"`
				} `json:"toolSpec"`
			} `json:"tools"`
			ToolChoice struct {
				Tool struct {
					Name string `json:"name"`
				} `json:"tool"`
			} `json:"toolChoice"`
		} `json:"toolConfig"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal wire body: %v", err)
	}
	if len(parsed.ToolConfig.Tools) != 1 || parsed.ToolConfig.Tools[0].ToolSpec.Name != "get_weather" {
		t.Fatalf("tools = %+v", parsed.ToolConfig.Tools)
	}
	if parsed.ToolConfig.Tools[0].ToolSpec.InputSchema.JSON["type"] != "object" {
		t.Fatalf("input schema = %+v", parsed.ToolConfig.Tools[0].ToolSpec.InputSchema.JSON)
	}
	if parsed.ToolConfig.ToolChoice.Tool.Name != "get_weather" {
		t.Fatalf("toolChoice.tool.name = %q", parsed.ToolConfig.ToolChoice.Tool.Name)
	}
}

func TestConverseBody_ToolResultBlock(t *testing.T) {
	body, err := converseBody(ConverseRequest{
		Messages: []ConverseMessage{{
			Role: "user",
			Content: []ContentBlock{{
				ToolResult: &ToolResultBlock{
					ToolUseID: "toolu_1",
					Content:   []ContentBlock{{Text: "30C"}},
					Status:    "success",
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("converseBody: %v", err)
	}
	if !containsAll(string(body), `"toolResult"`, `"toolUseId":"toolu_1"`, `"status":"success"`) {
		t.Fatalf("wire body missing expected toolResult shape: %s", body)
	}
}

func TestParseConverseResponse(t *testing.T) {
	raw := []byte(`{
		"output": {"message": {"role": "assistant", "content": [
			{"text": "hello"},
			{"toolUse": {"toolUseId": "toolu_1", "name": "get_weather", "input": {"city": "Delhi"}}}
		]}},
		"stopReason": "tool_use",
		"usage": {"inputTokens": 10, "outputTokens": 20},
		"metrics": {"latencyMs": 500}
	}`)
	resp, err := ParseConverseResponse(raw)
	if err != nil {
		t.Fatalf("ParseConverseResponse: %v", err)
	}
	if resp.Role != "assistant" || resp.StopReason != "tool_use" {
		t.Fatalf("Role/StopReason = %q/%q", resp.Role, resp.StopReason)
	}
	if len(resp.Content) != 2 || resp.Content[0].Text != "hello" {
		t.Fatalf("Content = %+v", resp.Content)
	}
	if resp.Content[1].ToolUse == nil || resp.Content[1].ToolUse.Name != "get_weather" {
		t.Fatalf("Content[1] = %+v", resp.Content[1])
	}
	if resp.Content[1].ToolUse.Input["city"] != "Delhi" {
		t.Fatalf("tool use input = %+v", resp.Content[1].ToolUse.Input)
	}
	if resp.InputTokens != 10 || resp.OutputTokens != 20 || resp.LatencyMS != 500 {
		t.Fatalf("usage/latency = %d/%d/%d", resp.InputTokens, resp.OutputTokens, resp.LatencyMS)
	}
}

func TestBedrockErrorMessage(t *testing.T) {
	if got := BedrockErrorMessage([]byte(`{"message":"bad request"}`)); got != "bad request" {
		t.Fatalf("got %q", got)
	}
	if got := BedrockErrorMessage([]byte(`{"error":{"message":"no such model"}}`)); got != "no such model" {
		t.Fatalf("openai envelope got %q", got)
	}
	if got := BedrockErrorMessage([]byte(`not json`)); got != "upstream model provider error" {
		t.Fatalf("fallback got %q", got)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
