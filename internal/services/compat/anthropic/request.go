// Package anthropic translates between Anthropic's Messages API wire
// format and Bedrock Converse's neutral shape (inferenceService.
// ConverseRequest/ConverseResponse), mirroring the OpenAI adapter in
// internal/services/compat/openai. Anthropic's own content-block/tool_use
// concepts map almost directly onto Bedrock Converse's, since Bedrock
// modeled Converse on Anthropic's original Messages API.
package anthropic

import (
	"encoding/json"
	"strings"

	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

// MessagesRequest is the subset of Anthropic's POST /v1/messages request
// body this adapter understands. Unsupported fields (image content
// blocks, prompt caching hints, extended thinking) are accepted but
// ignored — out of scope for this pass per the plan.
type MessagesRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	System        json.RawMessage    `json:"system"`
	Messages      []requestMessage   `json:"messages"`
	Tools         []requestTool      `json:"tools"`
	ToolChoice    *requestToolChoice `json:"tool_choice"`
	Temperature   *float64           `json:"temperature"`
	TopP          *float64           `json:"top_p"`
	StopSequences []string           `json:"stop_sequences"`
	Stream        bool               `json:"stream"`
}

type requestMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type requestContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     map[string]any  `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type requestTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type requestToolChoice struct {
	Type string `json:"type"` // "auto" | "any" | "tool" | "none"
	Name string `json:"name,omitempty"`
}

// parseContentBlocks handles Anthropic's "content" field, which is either
// a plain string (shorthand for a single text block) or an array of typed
// blocks.
func parseContentBlocks(raw json.RawMessage) []requestContentBlock {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return []requestContentBlock{{Type: "text", Text: s}}
	}
	var blocks []requestContentBlock
	_ = json.Unmarshal(raw, &blocks)
	return blocks
}

// blocksText concatenates the text of any "text"-typed blocks, used both
// for the top-level "system" field and for a tool_result block's own
// "content" (which has the same string-or-blocks shape).
func blocksText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	blocks := parseContentBlocks(raw)
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// IsValid reports whether the request has the minimum shape Anthropic
// itself requires: a model, a positive max_tokens, and at least one
// message.
func (req MessagesRequest) IsValid() bool {
	return strings.TrimSpace(req.Model) != "" && req.MaxTokens > 0 && len(req.Messages) > 0
}

// Prompt returns the last user message's text, for the billing job's
// human-readable Prompt field (best-effort, truncated).
func (req MessagesRequest) Prompt() string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			text := blocksText(req.Messages[i].Content)
			if len(text) > 4000 {
				text = text[:4000]
			}
			return text
		}
	}
	return ""
}

// ToConverse translates an Anthropic Messages request into Bedrock
// Converse's neutral shape.
func (req MessagesRequest) ToConverse() inferenceService.ConverseRequest {
	out := inferenceService.ConverseRequest{Stream: req.Stream, System: blocksText(req.System)}

	messages := make([]inferenceService.ConverseMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		var blocks []inferenceService.ContentBlock
		for _, b := range parseContentBlocks(m.Content) {
			switch b.Type {
			case "text":
				if b.Text != "" {
					blocks = append(blocks, inferenceService.ContentBlock{Text: b.Text})
				}
			case "tool_use":
				blocks = append(blocks, inferenceService.ContentBlock{
					ToolUse: &inferenceService.ToolUseBlock{ToolUseID: b.ID, Name: b.Name, Input: b.Input},
				})
			case "tool_result":
				status := "success"
				if b.IsError {
					status = "error"
				}
				blocks = append(blocks, inferenceService.ContentBlock{
					ToolResult: &inferenceService.ToolResultBlock{
						ToolUseID: b.ToolUseID,
						Content:   []inferenceService.ContentBlock{{Text: blocksText(b.Content)}},
						Status:    status,
					},
				})
			}
			// Other block types (image, document, ...) are out of scope
			// for this pass and silently skipped rather than erroring.
		}
		if len(blocks) == 0 {
			continue
		}
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, inferenceService.ConverseMessage{Role: role, Content: blocks})
	}
	out.Messages = messages

	dropTools := req.ToolChoice != nil && req.ToolChoice.Type == "none"
	if len(req.Tools) > 0 && !dropTools {
		tools := make([]inferenceService.ToolSpec, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = inferenceService.ToolSpec{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema}
		}
		out.Tools = tools

		if req.ToolChoice != nil {
			switch req.ToolChoice.Type {
			case "any":
				out.ToolChoice = &inferenceService.ToolChoice{Mode: "any"}
			case "tool":
				out.ToolChoice = &inferenceService.ToolChoice{Mode: "tool", ToolName: req.ToolChoice.Name}
			default:
				out.ToolChoice = &inferenceService.ToolChoice{Mode: "auto"}
			}
		}
	}

	if req.MaxTokens > 0 {
		mt := req.MaxTokens
		out.MaxTokens = &mt
	}
	out.Temperature = req.Temperature
	out.TopP = req.TopP
	out.StopSequences = req.StopSequences

	return out
}
