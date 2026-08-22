package inference

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"

	"github.com/shivang-16/orbit.api/internal/logger"
)

// relayResponsesStream reads OpenAI Responses SSE from Mantle and emits
// the same ConverseStream frames the existing OpenAI/Anthropic/native
// sinks already understand (messageStart, contentBlockDelta, ...).
func relayResponsesStream(ctx context.Context, body io.Reader, sink StreamSink) (inputTokens, outputTokens int, streamErr, cancelled bool) {
	tracker := &writeTrackingSink{inner: sink}
	adapter := &responsesStreamAdapter{ctx: ctx, sink: tracker}
	err := scanSSE(body, adapter.handle)
	if adapter.failed {
		streamErr = true
	} else if tracker.writeErr != nil {
		if isClientWriteAbort(tracker.writeErr) {
			logger.Info(ctx, "inference: stream stopped by client", "error", tracker.writeErr)
			cancelled = true
		} else {
			streamErr = true
		}
	} else if err != nil && !errors.Is(err, io.EOF) {
		if isRequestCanceled(err) {
			logger.Info(ctx, "inference: stream stopped by client", "error", err)
			cancelled = true
		} else {
			logger.Error(ctx, "inference: decode mantle event-stream", "error", err)
			streamErr = true
		}
	}
	if adapter.outputTokens == 0 && adapter.streamed.Len() > 0 {
		adapter.outputTokens = EstimateTokens(adapter.streamed.String())
	}
	if streamErr {
		cancelled = false
	}
	_ = sink.Close(streamErr)
	return adapter.inputTokens, adapter.outputTokens, streamErr, cancelled
}

// writeTrackingSink records the first HandleFrame write error so we can
// tell a client hang-up (broken pipe) apart from a Mantle read failure
// (connection reset) — scanSSE surfaces both as a single error.
type writeTrackingSink struct {
	inner    StreamSink
	writeErr error
}

func (s *writeTrackingSink) HandleFrame(eventType string, payload []byte) error {
	err := s.inner.HandleFrame(eventType, payload)
	if err != nil && s.writeErr == nil {
		s.writeErr = err
	}
	return err
}

func (s *writeTrackingSink) Close(streamErr bool) error {
	return s.inner.Close(streamErr)
}

type responsesStreamAdapter struct {
	ctx          context.Context
	sink         StreamSink
	started      bool
	nextBlock    int
	lastToolIdx  int
	toolBlocks   map[string]int
	openBlocks   map[int]struct{}
	stopReason   string
	inputTokens  int
	outputTokens int
	streamed     strings.Builder
	failed       bool
}

func (a *responsesStreamAdapter) handle(event string, data []byte) error {
	var envelope struct {
		Type     string          `json:"type"`
		Delta    json.RawMessage `json:"delta"`
		Item     json.RawMessage `json:"item"`
		Response json.RawMessage `json:"response"`
		Error    *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil
	}
	eventType := envelope.Type
	if eventType == "" {
		eventType = event
	}

	switch eventType {
	case "response.output_text.delta":
		text := responsesDeltaText(envelope.Delta)
		if text == "" {
			return nil
		}
		if err := a.ensureStarted(); err != nil {
			return err
		}
		a.streamed.WriteString(text)
		a.markOpen(0)
		return a.sink.HandleFrame("contentBlockDelta", mustJSON(map[string]any{
			"contentBlockIndex": 0,
			"delta":             map[string]any{"text": text},
		}))

	case "response.output_item.added":
		var item struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			CallID string `json:"call_id"`
			Name   string `json:"name"`
		}
		if err := json.Unmarshal(envelope.Item, &item); err != nil || item.Type != "function_call" {
			return nil
		}
		if err := a.ensureStarted(); err != nil {
			return err
		}
		idx := a.nextBlock
		if idx == 0 {
			idx = 1
			a.nextBlock = 2
		} else {
			a.nextBlock++
		}
		a.lastToolIdx = idx
		a.rememberTool(item.ID, idx)
		a.rememberTool(item.CallID, idx)
		a.markOpen(idx)
		a.stopReason = "tool_use"
		return a.sink.HandleFrame("contentBlockStart", mustJSON(map[string]any{
			"contentBlockIndex": idx,
			"start": map[string]any{
				"toolUse": map[string]any{
					"toolUseId": item.CallID,
					"name":      item.Name,
				},
			},
		}))

	case "response.function_call_arguments.delta":
		text := responsesDeltaText(envelope.Delta)
		if text == "" {
			return nil
		}
		var extra struct {
			ItemID string `json:"item_id"`
			CallID string `json:"call_id"`
		}
		_ = json.Unmarshal(data, &extra)
		idx := a.lookupTool(extra.ItemID, extra.CallID)
		a.markOpen(idx)
		return a.sink.HandleFrame("contentBlockDelta", mustJSON(map[string]any{
			"contentBlockIndex": idx,
			"delta":             map[string]any{"toolUse": map[string]any{"input": text}},
		}))

	case "response.completed", "response.incomplete":
		var completed struct {
			Status            string `json:"status"`
			IncompleteDetails *struct {
				Reason string `json:"reason"`
			} `json:"incomplete_details"`
			Usage *struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		src := envelope.Response
		if len(src) == 0 {
			src = data
		}
		_ = json.Unmarshal(src, &completed)
		if completed.Usage != nil {
			a.inputTokens = completed.Usage.InputTokens
			a.outputTokens = completed.Usage.OutputTokens
		}
		if a.stopReason == "" {
			a.stopReason = "end_turn"
			if completed.Status == "incomplete" || eventType == "response.incomplete" {
				a.stopReason = "max_tokens"
				if completed.IncompleteDetails != nil && !strings.Contains(completed.IncompleteDetails.Reason, "max") {
					a.stopReason = "end_turn"
				}
			}
		}
		if err := a.ensureStarted(); err != nil {
			return err
		}
		if err := a.closeOpenBlocks(); err != nil {
			return err
		}
		if err := a.sink.HandleFrame("messageStop", mustJSON(map[string]any{"stopReason": a.stopReason})); err != nil {
			return err
		}
		return a.sink.HandleFrame("metadata", mustJSON(map[string]any{
			"usage": map[string]any{
				"inputTokens":  a.inputTokens,
				"outputTokens": a.outputTokens,
			},
		}))

	case "response.failed", "error":
		a.failed = true
		msg := "stream interrupted"
		if envelope.Error != nil && envelope.Error.Message != "" {
			msg = envelope.Error.Message
		}
		logger.Error(a.ctx, "inference: mantle mid-stream error", "error", msg)
		return a.sink.HandleFrame("error", mustJSON(map[string]any{"message": msg}))
	}
	return nil
}

func (a *responsesStreamAdapter) ensureStarted() error {
	if a.started {
		return nil
	}
	a.started = true
	a.nextBlock = 1
	return a.sink.HandleFrame("messageStart", []byte(`{"role":"assistant"}`))
}

func (a *responsesStreamAdapter) rememberTool(id string, idx int) {
	if id == "" {
		return
	}
	if a.toolBlocks == nil {
		a.toolBlocks = make(map[string]int)
	}
	a.toolBlocks[id] = idx
}

func (a *responsesStreamAdapter) lookupTool(itemID, callID string) int {
	if a.toolBlocks != nil {
		if itemID != "" {
			if idx, ok := a.toolBlocks[itemID]; ok {
				return idx
			}
		}
		if callID != "" {
			if idx, ok := a.toolBlocks[callID]; ok {
				return idx
			}
		}
	}
	if a.lastToolIdx > 0 {
		return a.lastToolIdx
	}
	return 1
}

func (a *responsesStreamAdapter) markOpen(idx int) {
	if a.openBlocks == nil {
		a.openBlocks = make(map[int]struct{})
	}
	a.openBlocks[idx] = struct{}{}
}

func (a *responsesStreamAdapter) closeOpenBlocks() error {
	if len(a.openBlocks) == 0 {
		return nil
	}
	idxs := make([]int, 0, len(a.openBlocks))
	for idx := range a.openBlocks {
		idxs = append(idxs, idx)
	}
	sort.Ints(idxs)
	for _, idx := range idxs {
		if err := a.sink.HandleFrame("contentBlockStop", mustJSON(map[string]any{"contentBlockIndex": idx})); err != nil {
			return err
		}
	}
	a.openBlocks = nil
	return nil
}

func responsesDeltaText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Text
	}
	return ""
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func scanSSE(r io.Reader, handle func(event string, data []byte) error) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var event string
	var data []string
	flush := func() error {
		if len(data) == 0 {
			event = ""
			return nil
		}
		payload := strings.Join(data, "\n")
		ev := event
		event = ""
		data = data[:0]
		if payload == "" || payload == "[DONE]" {
			return nil
		}
		return handle(ev, []byte(payload))
	}

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			trimmed := string(bytes.TrimRight(line, "\r\n"))
			switch {
			case trimmed == "":
				if ferr := flush(); ferr != nil {
					return ferr
				}
			case strings.HasPrefix(trimmed, "event:"):
				event = strings.TrimSpace(trimmed[6:])
			case strings.HasPrefix(trimmed, "data:"):
				data = append(data, strings.TrimSpace(trimmed[5:]))
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return flush()
			}
			return err
		}
	}
}
