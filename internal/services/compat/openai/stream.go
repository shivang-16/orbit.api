package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	billingService "github.com/shivang-16/orbit.api/internal/services/billing"
)

// StreamSink implements inferenceService.StreamSink, translating Bedrock
// ConverseStream frames into OpenAI's chat.completion.chunk SSE format.
// One instance is used per request; it is not safe for concurrent use.
type StreamSink struct {
	w       io.Writer
	flusher http.Flusher
	id      string
	model   string
	created int64

	includeUsage bool
	roleSent     bool
	finished     bool

	// toolIndexByBlock maps a Bedrock contentBlockIndex to the sequential
	// index OpenAI expects in choices[].delta.tool_calls[].index (OpenAI
	// numbers tool calls independently of any interleaved text blocks).
	toolIndexByBlock map[int]int
	nextToolIndex    int

	inputTokens  int
	outputTokens int
}

func NewStreamSink(w io.Writer, flusher http.Flusher, model string, includeUsage bool) *StreamSink {
	return &StreamSink{
		w:                w,
		flusher:          flusher,
		id:               "chatcmpl-" + billingService.NewIdempotencyKey(),
		model:            model,
		created:          time.Now().Unix(),
		includeUsage:     includeUsage,
		toolIndexByBlock: make(map[int]int),
	}
}

type chunkDelta struct {
	Role      string             `json:"role,omitempty"`
	Content   string             `json:"content,omitempty"`
	ToolCalls []chunkToolCallDel `json:"tool_calls,omitempty"`
}

type chunkToolCallDel struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function *struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

type chunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

type chunk struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []chunkChoice  `json:"choices"`
	Usage   *responseUsage `json:"usage,omitempty"`
}

func (s *StreamSink) write(c chunk) error {
	body, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "data: %s\n\n", body); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

func (s *StreamSink) baseChunk(delta chunkDelta, finishReason *string) chunk {
	return chunk{
		ID:      s.id,
		Object:  "chat.completion.chunk",
		Created: s.created,
		Model:   s.model,
		Choices: []chunkChoice{{Index: 0, Delta: delta, FinishReason: finishReason}},
	}
}

func (s *StreamSink) HandleFrame(eventType string, payload []byte) error {
	switch eventType {
	case "messageStart":
		s.roleSent = true
		return s.write(s.baseChunk(chunkDelta{Role: "assistant"}, nil))

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
		idx := s.nextToolIndex
		s.toolIndexByBlock[start.ContentBlockIndex] = idx
		s.nextToolIndex++
		tc := chunkToolCallDel{Index: idx, ID: start.Start.ToolUse.ToolUseID, Type: "function"}
		tc.Function = &struct {
			Name      string `json:"name,omitempty"`
			Arguments string `json:"arguments,omitempty"`
		}{Name: start.Start.ToolUse.Name, Arguments: ""}
		return s.ensureRoleThen(chunkDelta{ToolCalls: []chunkToolCallDel{tc}})

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
			return s.ensureRoleThen(chunkDelta{Content: delta.Delta.Text})
		}
		if delta.Delta.ToolUse != nil && delta.Delta.ToolUse.Input != "" {
			idx, ok := s.toolIndexByBlock[delta.ContentBlockIndex]
			if !ok {
				return nil
			}
			tc := chunkToolCallDel{Index: idx}
			tc.Function = &struct {
				Name      string `json:"name,omitempty"`
				Arguments string `json:"arguments,omitempty"`
			}{Arguments: delta.Delta.ToolUse.Input}
			return s.ensureRoleThen(chunkDelta{ToolCalls: []chunkToolCallDel{tc}})
		}
		return nil

	case "contentBlockStop":
		return nil

	case "messageStop":
		var stop struct {
			StopReason string `json:"stopReason"`
		}
		_ = json.Unmarshal(payload, &stop)
		reason := stopReasonToFinishReason(stop.StopReason)
		s.finished = true
		return s.write(s.baseChunk(chunkDelta{}, &reason))

	case "metadata":
		var meta struct {
			Usage struct {
				InputTokens  int `json:"inputTokens"`
				OutputTokens int `json:"outputTokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(payload, &meta); err == nil {
			s.inputTokens = meta.Usage.InputTokens
			s.outputTokens = meta.Usage.OutputTokens
		}
		if s.includeUsage {
			c := s.baseChunk(chunkDelta{}, nil)
			c.Choices = nil
			c.Usage = &responseUsage{
				PromptTokens:     s.inputTokens,
				CompletionTokens: s.outputTokens,
				TotalTokens:      s.inputTokens + s.outputTokens,
			}
			return s.write(c)
		}
		return nil

	default:
		return nil
	}
}

// ensureRoleThen emits the assistant-role chunk first if Bedrock's
// messageStart frame hasn't arrived yet (defensive; it always should
// have), then writes the given delta.
func (s *StreamSink) ensureRoleThen(delta chunkDelta) error {
	if !s.roleSent {
		s.roleSent = true
		if err := s.write(s.baseChunk(chunkDelta{Role: "assistant"}, nil)); err != nil {
			return err
		}
	}
	return s.write(s.baseChunk(delta, nil))
}

func (s *StreamSink) Close(streamErr bool) error {
	if !s.finished {
		reason := "stop"
		if streamErr {
			reason = "stop"
		}
		if err := s.write(s.baseChunk(chunkDelta{}, &reason)); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(s.w, "data: [DONE]\n\n")
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return err
}
