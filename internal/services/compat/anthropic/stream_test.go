package anthropic

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type sseEvent struct {
	eventType string
	data      map[string]any
}

func parseSSEEvents(t *testing.T, raw string) []sseEvent {
	t.Helper()
	var out []sseEvent
	for _, block := range strings.Split(raw, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") || !strings.HasPrefix(lines[1], "data: ") {
			t.Fatalf("expected Anthropic-style \"event:\"/\"data:\" pair, got: %q", block)
		}
		eventType := strings.TrimPrefix(lines[0], "event: ")
		var data map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &data); err != nil {
			t.Fatalf("event %s: invalid JSON data: %v", eventType, err)
		}
		if data["type"] != eventType {
			t.Fatalf("event: %s but data.type = %v", eventType, data["type"])
		}
		out = append(out, sseEvent{eventType: eventType, data: data})
	}
	return out
}

func eventTypes(events []sseEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.eventType
	}
	return out
}

func TestAnthropicStreamSink_TextOnly(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "claude-opus-5")

	frames := []struct {
		eventType string
		payload   string
	}{
		{"messageStart", `{"role":"assistant"}`},
		{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"text":"Hi"}}`},
		{"contentBlockDelta", `{"contentBlockIndex":0,"delta":{"text":" there"}}`},
		{"contentBlockStop", `{"contentBlockIndex":0}`},
		{"messageStop", `{"stopReason":"end_turn"}`},
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

	events := parseSSEEvents(t, buf.String())
	want := []string{
		"message_start",
		"content_block_start", // lazily synthesized before the first text delta
		"content_block_delta",
		"content_block_delta",
		"content_block_stop",
		"message_delta",
		"message_stop",
	}
	got := eventTypes(events)
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}

	// content_block_start for a text block should declare an empty
	// "text" block, per Anthropic's own protocol.
	startBlock := events[1].data["content_block"].(map[string]any)
	if startBlock["type"] != "text" {
		t.Fatalf("content_block_start.content_block.type = %v, want text", startBlock["type"])
	}

	joined := ""
	for _, e := range events {
		if e.eventType == "content_block_delta" {
			delta := e.data["delta"].(map[string]any)
			joined += delta["text"].(string)
		}
	}
	if joined != "Hi there" {
		t.Fatalf("joined text = %q, want %q", joined, "Hi there")
	}

	finalDelta := events[len(events)-2].data["delta"].(map[string]any)
	if finalDelta["stop_reason"] != "end_turn" {
		t.Fatalf("message_delta.delta.stop_reason = %v", finalDelta["stop_reason"])
	}
}

func TestAnthropicStreamSink_ToolUse(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "claude-opus-5")

	frames := []struct {
		eventType string
		payload   string
	}{
		{"messageStart", `{"role":"assistant"}`},
		{"contentBlockStart", `{"contentBlockIndex":0,"start":{"toolUse":{"toolUseId":"toolu_1","name":"get_weather"}}}`},
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

	events := parseSSEEvents(t, buf.String())

	start := events[1]
	if start.eventType != "content_block_start" {
		t.Fatalf("events[1] = %s, want content_block_start", start.eventType)
	}
	block := start.data["content_block"].(map[string]any)
	if block["type"] != "tool_use" || block["id"] != "toolu_1" || block["name"] != "get_weather" {
		t.Fatalf("content_block_start.content_block = %+v", block)
	}

	partial := ""
	for _, e := range events {
		if e.eventType == "content_block_delta" {
			delta := e.data["delta"].(map[string]any)
			if delta["type"] != "input_json_delta" {
				t.Fatalf("delta.type = %v, want input_json_delta", delta["type"])
			}
			partial += delta["partial_json"].(string)
		}
	}
	if partial != `{"city":"Delhi"}` {
		t.Fatalf("joined partial_json = %q", partial)
	}

	last := events[len(events)-1]
	if last.eventType != "message_stop" {
		t.Fatalf("last event = %s, want message_stop", last.eventType)
	}
}

func TestAnthropicStreamSink_MidStreamError(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "claude-opus-5")

	_ = sink.HandleFrame("messageStart", []byte(`{"role":"assistant"}`))
	if err := sink.HandleFrame("error", []byte(`{"message":"overloaded"}`)); err != nil {
		t.Fatalf("HandleFrame(error): %v", err)
	}
	if err := sink.Close(true); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := parseSSEEvents(t, buf.String())
	last := events[len(events)-1]
	if last.eventType != "error" {
		t.Fatalf("last event = %s, want error", last.eventType)
	}
	errDetail := last.data["error"].(map[string]any)
	if errDetail["message"] != "overloaded" {
		t.Fatalf("error.message = %v", errDetail["message"])
	}
}

func TestAnthropicStreamSink_CloseWithoutMetadataStillTerminates(t *testing.T) {
	var buf bytes.Buffer
	sink := NewStreamSink(&buf, nil, "claude-opus-5")

	_ = sink.HandleFrame("messageStart", []byte(`{"role":"assistant"}`))
	_ = sink.HandleFrame("messageStop", []byte(`{"stopReason":"end_turn"}`))
	// No "metadata" frame arrives (e.g. connection dropped); Close must
	// still emit message_delta/message_stop so the client doesn't hang.
	if err := sink.Close(true); err != nil {
		t.Fatalf("Close: %v", err)
	}

	events := parseSSEEvents(t, buf.String())
	got := eventTypes(events)
	if len(got) != 3 || got[1] != "message_delta" || got[2] != "message_stop" {
		t.Fatalf("events = %v, want [message_start message_delta message_stop]", got)
	}
}
