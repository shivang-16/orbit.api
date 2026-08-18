package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
)

// StreamSink implements inferenceService.StreamSink, translating Bedrock
// ConverseStream frames into Anthropic's Messages streaming SSE events
// (message_start / content_block_start / content_block_delta /
// content_block_stop / message_delta / message_stop). This is close to a
// direct relay since Bedrock modeled Converse's streaming shape on
// Anthropic's own. One instance is used per request; it is not safe for
// concurrent use.
type StreamSink struct {
	w       io.Writer
	flusher http.Flusher
	id      string
	model   string

	// started tracks which Bedrock contentBlockIndex values have already
	// had a content_block_start emitted. Bedrock only sends
	// contentBlockStart for tool_use blocks, but Anthropic's protocol
	// requires a content_block_start before any content_block_delta —
	// including text — so text blocks are lazily "started" on first delta.
	started map[int]bool

	stopReason     string
	haveStopReason bool
	inputTokens    int
	outputTokens   int
	closed         bool
	errored        bool
}

func NewStreamSink(w io.Writer, flusher http.Flusher, model string) *StreamSink {
	return &StreamSink{
		w:       w,
		flusher: flusher,
		id:      "msg_" + billingService.NewIdempotencyKey(),
		model:   model,
		started: make(map[int]bool),
	}
}

func (s *StreamSink) writeRaw(v map[string]any) error {
	eventType, _ := v["type"].(string)
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, body); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

func (s *StreamSink) HandleFrame(eventType string, payload []byte) error {
	switch eventType {
	case "messageStart":
		return s.writeRaw(map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            s.id,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         s.model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         map[string]any{"input_tokens": 0, "output_tokens": 0},
			},
		})

	case "contentBlockStart":
		var start struct {
			ContentBlockIndex int `json:"contentBlockIndex"`
			Start             struct {
				ToolUse *struct {
					ToolUseID string `json:"toolUseId"`
					Name      string `json:"name"`
				} `json:"toolUse"`
			} `json:"start"`
		}
		if err := json.Unmarshal(payload, &start); err != nil || start.Start.ToolUse == nil {
			return nil
		}
		s.started[start.ContentBlockIndex] = true
		return s.writeRaw(map[string]any{
			"type":  "content_block_start",
			"index": start.ContentBlockIndex,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    start.Start.ToolUse.ToolUseID,
				"name":  start.Start.ToolUse.Name,
				"input": map[string]any{},
			},
		})

	case "contentBlockDelta":
		var delta struct {
			ContentBlockIndex int `json:"contentBlockIndex"`
			Delta             struct {
				Text    string `json:"text"`
				ToolUse *struct {
					Input string `json:"input"`
				} `json:"toolUse"`
			} `json:"delta"`
		}
		if err := json.Unmarshal(payload, &delta); err != nil {
			return nil
		}
		if delta.Delta.Text != "" {
			if !s.started[delta.ContentBlockIndex] {
				s.started[delta.ContentBlockIndex] = true
				if err := s.writeRaw(map[string]any{
					"type":          "content_block_start",
					"index":         delta.ContentBlockIndex,
					"content_block": map[string]any{"type": "text", "text": ""},
				}); err != nil {
					return err
				}
			}
			return s.writeRaw(map[string]any{
				"type":  "content_block_delta",
				"index": delta.ContentBlockIndex,
				"delta": map[string]any{"type": "text_delta", "text": delta.Delta.Text},
			})
		}
		if delta.Delta.ToolUse != nil {
			return s.writeRaw(map[string]any{
				"type":  "content_block_delta",
				"index": delta.ContentBlockIndex,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": delta.Delta.ToolUse.Input},
			})
		}
		return nil

	case "contentBlockStop":
		var stop struct {
			ContentBlockIndex int `json:"contentBlockIndex"`
		}
		_ = json.Unmarshal(payload, &stop)
		if !s.started[stop.ContentBlockIndex] {
			return nil
		}
		return s.writeRaw(map[string]any{"type": "content_block_stop", "index": stop.ContentBlockIndex})

	case "messageStop":
		var stop struct {
			StopReason string `json:"stopReason"`
		}
		_ = json.Unmarshal(payload, &stop)
		s.stopReason = stopReasonToAnthropic(stop.StopReason)
		s.haveStopReason = true
		return nil

	case "metadata":
		var meta struct {
			Usage struct {
				InputTokens  int `json:"inputTokens"`
				OutputTokens int `json:"outputTokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal(payload, &meta)
		s.inputTokens = meta.Usage.InputTokens
		s.outputTokens = meta.Usage.OutputTokens
		return s.finish()

	case "error":
		s.errored = true
		var exc struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(payload, &exc)
		msg := exc.Message
		if msg == "" {
			msg = "stream error"
		}
		return s.writeRaw(map[string]any{
			"type":  "error",
			"error": map[string]any{"type": "api_error", "message": msg},
		})

	default:
		return nil
	}
}

// finish emits Anthropic's terminal message_delta (carrying the final
// stop_reason + output token usage) followed by message_stop, matching
// Bedrock's own event order (messageStop then metadata) by deferring
// both until the metadata frame — the one place usage is known — arrives.
func (s *StreamSink) finish() error {
	if s.closed {
		return nil
	}
	s.closed = true
	stopReason := s.stopReason
	if !s.haveStopReason {
		stopReason = "end_turn"
	}
	if err := s.writeRaw(map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]any{"output_tokens": s.outputTokens},
	}); err != nil {
		return err
	}
	return s.writeRaw(map[string]any{"type": "message_stop"})
}

func (s *StreamSink) Close(streamErr bool) error {
	if !s.closed && !s.errored {
		return s.finish()
	}
	return nil
}
