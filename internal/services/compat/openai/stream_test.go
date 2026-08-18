package openai

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// parseSSEDataLines extracts the payload of every "data: ..." SSE line
// from raw output, in order, including the literal "[DONE]" terminator.
func parseSSEDataLines(t *testing.T, raw string) []string {
	t.Helper()
	var out []string
	for _, block := range strings.Split(raw, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		if !strings.HasPrefix(block, "data: ") {
			t.Fatalf("expected an OpenAI-style SSE line (no \"event:\" field), got: %q", block)
		}
		out = append(out, strings.TrimPrefix(block, "data: "))
	}
	return out
}

func TestStreamSink_TextOnly(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "claude-opus-5", false)

	frames := []struct {
		eventType string
		payload   string
	}{
		{"messageStart", `{"role":"assistant"}`},
		{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"text":"Hello"}}`},
		{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"text":" world"}}`},
		{"contentBlockStop", `{"contentBlockIndex":0}`},
		{"messageStop", `{"stopReason":"end_turn"}`},
		{"metadata", `{"usage":{"inputTokens":5,"outputTokens":2},"metrics":{"latencyMs":100}}`},
	}
	for _, f := range frames {
		if err := sink.HandleFrame(f.eventType, []byte(f.payload)); err != nil {
			t.Fatalf("HandleFrame(%s): %v", f.eventType, err)
		}
	}
	if err := sink.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := parseSSEDataLines(t, buf.String())
	if len(lines) == 0 || lines[len(lines)-1] != "[DONE]" {
		t.Fatalf("expected stream to end with [DONE], got %v", lines)
	}

	var sawRole, sawFinish bool
	var contentJoined string
	for _, line := range lines[:len(lines)-1] {
		var c chunk
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("chunk not valid JSON: %v (%s)", err, line)
		}
		if c.Object != "chat.completion.chunk" {
			t.Fatalf("Object = %q", c.Object)
		}
		delta := c.Choices[0].Delta
		if delta.Role == "assistant" {
			sawRole = true
		}
		if delta.Content != "" {
			contentJoined += delta.Content
		}
		if c.Choices[0].FinishReason != nil {
			sawFinish = true
			if *c.Choices[0].FinishReason != "stop" {
				t.Fatalf("finish_reason = %q, want stop", *c.Choices[0].FinishReason)
			}
		}
	}
	if !sawRole {
		t.Fatal("expected an assistant role chunk")
	}
	if !sawFinish {
		t.Fatal("expected a finish_reason chunk")
	}
	if contentJoined != "Hello world" {
		t.Fatalf("joined content = %q, want %q", contentJoined, "Hello world")
	}
}

func TestStreamSink_ToolCall(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "gpt-5.6-sol", false)

	frames := []struct {
		eventType string
		payload   string
	}{
		{"messageStart", `{"role":"assistant"}`},
		{"contentBlockStart", `{"contentBlockIndex":0,"start":{"toolUse":{"toolUseId":"tooluse_1","name":"get_weather"}}}`},
		// Bedrock can send an empty first input fragment; it must not
		// produce a frivolous "tool_calls":[{"function":{}}] chunk.
		{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"toolUse":{"input":""}}}`},
		{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"toolUse":{"input":"{\"city\":"}}}`},
		{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"toolUse":{"input":"\"Delhi\"}"}}}`},
		{"contentBlockStop", `{"contentBlockIndex":0}`},
		{"messageStop", `{"stopReason":"tool_use"}`},
		{"metadata", `{"usage":{"inputTokens":5,"outputTokens":2}}`},
	}
	for _, f := range frames {
		if err := sink.HandleFrame(f.eventType, []byte(f.payload)); err != nil {
			t.Fatalf("HandleFrame(%s): %v", f.eventType, err)
		}
	}
	if err := sink.Close(false); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := parseSSEDataLines(t, buf.String())
	var toolCallID, toolName, argsJoined, finishReason string
	for _, line := range lines {
		if line == "[DONE]" {
			continue
		}
		var c chunk
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("chunk not valid JSON: %v (%s)", err, line)
		}
		delta := c.Choices[0].Delta
		for _, tc := range delta.ToolCalls {
			if tc.ID != "" {
				toolCallID = tc.ID
			}
			if tc.Type != "" {
				if tc.Type != "function" {
					t.Fatalf("tool_call type = %q, want function", tc.Type)
				}
			}
			if tc.Function == nil {
				t.Fatalf("tool_calls delta with a nil function (frivolous empty chunk): %+v", tc)
			}
			if tc.Function.Name == "" && tc.Function.Arguments == "" {
				t.Fatalf("frivolous empty tool_calls delta emitted: %+v", tc)
			}
			if tc.Function.Name != "" {
				toolName = tc.Function.Name
			}
			argsJoined += tc.Function.Arguments
		}
		if c.Choices[0].FinishReason != nil {
			finishReason = *c.Choices[0].FinishReason
		}
	}

	if toolCallID != "tooluse_1" {
		t.Fatalf("tool call id = %q", toolCallID)
	}
	if toolName != "get_weather" {
		t.Fatalf("tool name = %q", toolName)
	}
	if argsJoined != `{"city":"Delhi"}` {
		t.Fatalf("joined arguments = %q", argsJoined)
	}
	if finishReason != "tool_calls" {
		t.Fatalf("finish_reason = %q, want tool_calls", finishReason)
	}
}

func TestStreamSink_CloseWithoutMessageStopStillEndsCleanly(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "claude-opus-5", false)

	if err := sink.HandleFrame("messageStart", []byte(`{"role":"assistant"}`)); err != nil {
		t.Fatalf("HandleFrame: %v", err)
	}
	if err := sink.Close(true); err != nil {
		t.Fatalf("Close: %v", err)
	}

	lines := parseSSEDataLines(t, buf.String())
	if lines[len(lines)-1] != "[DONE]" {
		t.Fatalf("expected [DONE] terminator even after a mid-stream error, got %v", lines)
	}
}

func TestStreamSink_UsageChunkOnlyWhenRequested(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "claude-opus-5", true)
	_ = sink.HandleFrame("messageStart", []byte(`{"role":"assistant"}`))
	_ = sink.HandleFrame("messageStop", []byte(`{"stopReason":"end_turn"}`))
	_ = sink.HandleFrame("metadata", []byte(`{"usage":{"inputTokens":3,"outputTokens":4}}`))
	_ = sink.Close(false)

	lines := parseSSEDataLines(t, buf.String())
	var sawUsage bool
	for _, line := range lines {
		if line == "[DONE]" {
			continue
		}
		var c chunk
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatalf("chunk not valid JSON: %v", err)
		}
		if c.Usage != nil {
			sawUsage = true
			if c.Usage.PromptTokens != 3 || c.Usage.CompletionTokens != 4 {
				t.Fatalf("usage = %+v", c.Usage)
			}
		}
	}
	if !sawUsage {
		t.Fatal("expected a usage chunk when stream_options.include_usage is set")
	}
}
