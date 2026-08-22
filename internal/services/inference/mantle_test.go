package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestUsesMantleResponses(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-sol", true},
		{"arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-terra", true},
		{"arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-luna", true},
		{"us.openai.gpt-5.5", true},
		{"openai.gpt-5.4", true},
		{"openai.gpt-oss-120b-1:0", false},
		{"openai.gpt-oss-safeguard-20b", false},
		{"arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.anthropic.claude-sonnet-5", false},
		{"moonshotai.kimi-k2.5", false},
	}
	for _, tc := range cases {
		if got := usesMantleResponses(tc.id); got != tc.want {
			t.Errorf("usesMantleResponses(%q) = %t, want %t", tc.id, got, tc.want)
		}
	}
}

func TestMantleModelID(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"arn:aws:bedrock:us-east-1:471112741644:inference-profile/us.openai.gpt-5.6-sol", "openai.gpt-5.6-sol"},
		{"us.openai.gpt-5.6-terra", "openai.gpt-5.6-terra"},
		{"global.openai.gpt-5.6-luna", "openai.gpt-5.6-luna"},
		{"openai.gpt-5.4", "openai.gpt-5.4"},
	}
	for _, tc := range cases {
		if got := mantleModelID(tc.in); got != tc.want {
			t.Errorf("mantleModelID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResponsesBody_TextSystemAndLimits(t *testing.T) {
	maxTokens := 128
	temp := 0.2
	body, err := responsesBody("us.openai.gpt-5.6-sol", ConverseRequest{
		Messages:    []ConverseMessage{{Role: "user", Content: []ContentBlock{{Text: "Hello!"}}}},
		System:      "be brief",
		MaxTokens:   &maxTokens,
		Temperature: &temp,
		Stream:      true,
	})
	if err != nil {
		t.Fatalf("responsesBody: %v", err)
	}
	var parsed responsesRequest
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Model != "openai.gpt-5.6-sol" {
		t.Fatalf("model = %q", parsed.Model)
	}
	if parsed.Instructions != "be brief" || !parsed.Stream || parsed.Store {
		t.Fatalf("instructions/stream/store = %+v", parsed)
	}
	if parsed.MaxOutputTokens == nil || *parsed.MaxOutputTokens != 128 {
		t.Fatalf("max_output_tokens = %v", parsed.MaxOutputTokens)
	}
	if len(parsed.Input) != 1 || parsed.Input[0].Role != "user" || parsed.Input[0].Content != "Hello!" {
		t.Fatalf("input = %+v", parsed.Input)
	}
}

func TestResponsesBody_ToolsAndHistory(t *testing.T) {
	body, err := responsesBody("openai.gpt-5.6-sol", ConverseRequest{
		Messages: []ConverseMessage{
			{Role: "user", Content: []ContentBlock{{Text: "weather?"}}},
			{Role: "assistant", Content: []ContentBlock{{ToolUse: &ToolUseBlock{
				ToolUseID: "call_1",
				Name:      "get_weather",
				Input:     map[string]any{"city": "Delhi"},
			}}}},
			{Role: "user", Content: []ContentBlock{{ToolResult: &ToolResultBlock{
				ToolUseID: "call_1",
				Content:   []ContentBlock{{Text: "30C"}},
			}}}},
		},
		Tools:      []ToolSpec{{Name: "get_weather", Description: "weather", InputSchema: map[string]any{"type": "object"}}},
		ToolChoice: &ToolChoice{Mode: "any"},
	})
	if err != nil {
		t.Fatalf("responsesBody: %v", err)
	}
	raw := string(body)
	if !strings.Contains(raw, `"type":"function_call"`) || !strings.Contains(raw, `"call_id":"call_1"`) {
		t.Fatalf("missing function_call: %s", raw)
	}
	if !strings.Contains(raw, `"type":"function_call_output"`) || !strings.Contains(raw, `"output":"30C"`) {
		t.Fatalf("missing function_call_output: %s", raw)
	}
	if !strings.Contains(raw, `"tool_choice":"required"`) {
		t.Fatalf("missing tool_choice required: %s", raw)
	}
}

func TestResponsesToConverseJSON(t *testing.T) {
	raw := []byte(`{
		"status": "completed",
		"output": [
			{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "hello"}]},
			{"type": "function_call", "call_id": "call_1", "name": "get_weather", "arguments": "{\"city\":\"Delhi\"}"}
		],
		"usage": {"input_tokens": 11, "output_tokens": 7}
	}`)
	body, in, out, err := responsesToConverseJSON(raw, 42)
	if err != nil {
		t.Fatalf("responsesToConverseJSON: %v", err)
	}
	if in != 11 || out != 7 {
		t.Fatalf("tokens = %d/%d", in, out)
	}
	parsed, err := ParseConverseResponse(body)
	if err != nil {
		t.Fatalf("ParseConverseResponse: %v", err)
	}
	if parsed.StopReason != "tool_use" || parsed.Content[0].Text != "hello" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if parsed.Content[1].ToolUse == nil || parsed.Content[1].ToolUse.Name != "get_weather" {
		t.Fatalf("tool = %+v", parsed.Content[1])
	}
	if parsed.LatencyMS != 42 {
		t.Fatalf("latency = %d", parsed.LatencyMS)
	}
}

type recordingSink struct {
	frames []string
	closed bool
	err    bool
}

func (s *recordingSink) HandleFrame(eventType string, payload []byte) error {
	s.frames = append(s.frames, eventType+" "+string(payload))
	return nil
}

func (s *recordingSink) Close(streamErr bool) error {
	s.closed = true
	s.err = streamErr
	return nil
}

func TestRelayResponsesStream_TextAndUsage(t *testing.T) {
	sse := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"Hel"}`,
		``,
		`data: {"type":"response.output_text.delta","delta":"lo"}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":3,"output_tokens":2}}}`,
		``,
	}, "\n")
	sink := &recordingSink{}
	in, out, streamErr := relayResponsesStream(context.Background(), bytes.NewBufferString(sse), sink)
	if streamErr {
		t.Fatalf("streamErr = true")
	}
	if in != 3 || out != 2 {
		t.Fatalf("tokens = %d/%d", in, out)
	}
	if !sink.closed {
		t.Fatalf("sink not closed")
	}
	joined := strings.Join(sink.frames, "\n")
	if !strings.Contains(joined, "messageStart") {
		t.Fatalf("missing messageStart: %s", joined)
	}
	if !strings.Contains(joined, `"text":"Hel"`) || !strings.Contains(joined, `"text":"lo"`) {
		t.Fatalf("missing text deltas: %s", joined)
	}
	if !strings.Contains(joined, "messageStop") || !strings.Contains(joined, `"stopReason":"end_turn"`) {
		t.Fatalf("missing messageStop: %s", joined)
	}
	if !strings.Contains(joined, "contentBlockStop") {
		t.Fatalf("missing contentBlockStop: %s", joined)
	}
	if !strings.Contains(joined, "metadata") {
		t.Fatalf("missing metadata: %s", joined)
	}
}

func TestRelayResponsesStream_ToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"response.output_item.added","item":{"id":"fc_1","type":"function_call","call_id":"call_1","name":"get_weather"}}`,
		``,
		`data: {"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"city\""}`,
		``,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":1,"output_tokens":4}}}`,
		``,
	}, "\n")
	sink := &recordingSink{}
	_, _, streamErr := relayResponsesStream(context.Background(), bytes.NewBufferString(sse), sink)
	if streamErr {
		t.Fatalf("streamErr = true")
	}
	joined := strings.Join(sink.frames, "\n")
	if !strings.Contains(joined, "contentBlockStart") || !strings.Contains(joined, `"name":"get_weather"`) {
		t.Fatalf("missing tool start: %s", joined)
	}
	if !strings.Contains(joined, `"contentBlockIndex":1`) || !strings.Contains(joined, `contentBlockDelta`) || !strings.Contains(joined, `city`) {
		t.Fatalf("argument delta not mapped via item_id: %s", joined)
	}
	if !strings.Contains(joined, "contentBlockStop") {
		t.Fatalf("missing contentBlockStop: %s", joined)
	}
	if !strings.Contains(joined, `"stopReason":"tool_use"`) {
		t.Fatalf("expected tool_use stop: %s", joined)
	}
}

func TestResponsesToConverseJSON_FailedStatus(t *testing.T) {
	raw := []byte(`{
		"status": "failed",
		"error": {"message": "content filter"},
		"output": [],
		"usage": {"input_tokens": 8, "output_tokens": 0}
	}`)
	_, in, out, err := responsesToConverseJSON(raw, 12)
	if err == nil {
		t.Fatal("expected error for failed status")
	}
	var failed *responsesStatusError
	if !errors.As(err, &failed) {
		t.Fatalf("err type = %T (%v)", err, err)
	}
	if failed.Message != "content filter" {
		t.Fatalf("message = %q", failed.Message)
	}
	if in != 8 || out != 0 {
		t.Fatalf("tokens = %d/%d", in, out)
	}
	if msg := BedrockErrorMessage(failedProviderBody(err)); msg != "content filter" {
		t.Fatalf("provider body = %q", msg)
	}
}

func TestChatRequestToConverse(t *testing.T) {
	stream := true
	got := chatRequestToConverse(ChatRequest{
		Messages:    []ChatMessage{{Role: "user", Content: "hi"}},
		MaxTokens:   64,
		Temperature: 0.1,
		Stream:      &stream,
	})
	if !got.Stream || len(got.Messages) != 1 || got.Messages[0].Content[0].Text != "hi" {
		t.Fatalf("got %+v", got)
	}
	if got.MaxTokens == nil || *got.MaxTokens != 64 {
		t.Fatalf("max tokens = %v", got.MaxTokens)
	}
}
