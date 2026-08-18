// Package openai translates between OpenAI's Chat Completions API wire
// format and Bedrock Converse's neutral shape (inferenceService.
// ConverseRequest/ConverseResponse), so the compat controller only has to
// glue parsing/formatting to inferenceService.Converse. See the plan doc
// for why Bedrock Converse works well as the shared intermediate
// representation: its toolConfig/toolUse concepts map almost 1:1 onto
// OpenAI's tools/tool_calls.
package openai

import (
	"encoding/json"
	"strings"

	inferenceService "github.com/shivang-16/orbit.api/internal/services/inference"
)

// ChatCompletionRequest is the subset of OpenAI's POST /v1/chat/completions
// request body this adapter understands. Unsupported fields
// (response_format/JSON mode, n>1, logprobs, image/vision content parts)
// are accepted but ignored — out of scope for this pass per the plan,
// rather than silently misbehaving in a way that's hard to notice.
type ChatCompletionRequest struct {
	Model               string           `json:"model"`
	Messages            []requestMessage `json:"messages"`
	Tools               []requestTool    `json:"tools"`
	ToolChoice          json.RawMessage  `json:"tool_choice"`
	MaxTokens           *int             `json:"max_tokens"`
	MaxCompletionTokens *int             `json:"max_completion_tokens"`
	Temperature         *float64         `json:"temperature"`
	TopP                *float64         `json:"top_p"`
	Stop                json.RawMessage  `json:"stop"`
	Stream              bool             `json:"stream"`
	StreamOptions       *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options"`
}

type requestMessage struct {
	Role       string            `json:"role"`
	Content    json.RawMessage   `json:"content"`
	ToolCalls  []requestToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
}

type requestToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type requestTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

// contentText extracts plain text from an OpenAI "content" field, which
// can be a JSON string, null, or an array of typed parts ({"type":"text",
// "text": "..."} plus others we don't support yet, e.g. image_url).
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" && p.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n")
				}
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

func stopSequences(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s == "" {
			return nil
		}
		return []string{s}
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list
	}
	return nil
}

// toolChoice parses OpenAI's tool_choice field. dropTools reports whether
// tool_choice was explicitly "none", in which case ToConverse omits
// toolConfig entirely (Bedrock has no direct "tools defined but disabled
// this turn" concept, so we best-effort drop them for that one call).
func toolChoice(raw json.RawMessage) (choice *inferenceService.ToolChoice, dropTools bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "none":
			return nil, true
		case "required":
			return &inferenceService.ToolChoice{Mode: "any"}, false
		default:
			return &inferenceService.ToolChoice{Mode: "auto"}, false
		}
	}
	var obj struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Type == "function" && obj.Function.Name != "" {
		return &inferenceService.ToolChoice{Mode: "tool", ToolName: obj.Function.Name}, false
	}
	return nil, false
}

// IsValid reports whether the request has the minimum shape needed to
// forward to Bedrock: a model identifier and at least one message.
func (req ChatCompletionRequest) IsValid() bool {
	return strings.TrimSpace(req.Model) != "" && len(req.Messages) > 0
}

// WantsUsage reports whether the streamed response should include a
// trailing usage-only chunk, per OpenAI's stream_options.include_usage.
func (req ChatCompletionRequest) WantsUsage() bool {
	return req.StreamOptions != nil && req.StreamOptions.IncludeUsage
}

// Prompt returns the last user message's text, for the billing job's
// human-readable Prompt field (best-effort, truncated).
func (req ChatCompletionRequest) Prompt() string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			text := contentText(req.Messages[i].Content)
			if len(text) > 4000 {
				text = text[:4000]
			}
			return text
		}
	}
	return ""
}

// ToConverse translates an OpenAI chat.completions request into Bedrock
// Converse's neutral shape.
func (req ChatCompletionRequest) ToConverse() inferenceService.ConverseRequest {
	out := inferenceService.ConverseRequest{Stream: req.Stream}

	var systemParts []string
	var messages []inferenceService.ConverseMessage
	var pendingToolResults []inferenceService.ContentBlock

	flushToolResults := func() {
		if len(pendingToolResults) > 0 {
			messages = append(messages, inferenceService.ConverseMessage{Role: "user", Content: pendingToolResults})
			pendingToolResults = nil
		}
	}

	for _, m := range req.Messages {
		role := strings.TrimSpace(m.Role)
		switch role {
		case "system", "developer":
			if text := contentText(m.Content); text != "" {
				systemParts = append(systemParts, text)
			}
		case "tool":
			// OpenAI sends one "tool" message per tool_call_id; Bedrock
			// requires all toolResult blocks answering a parallel set of
			// toolUse blocks to land in a single "user" message, so we
			// accumulate consecutive tool messages and flush them
			// together the next time a non-tool message is seen.
			pendingToolResults = append(pendingToolResults, inferenceService.ContentBlock{
				ToolResult: &inferenceService.ToolResultBlock{
					ToolUseID: m.ToolCallID,
					Content:   []inferenceService.ContentBlock{{Text: contentText(m.Content)}},
				},
			})
		default:
			flushToolResults()
			var blocks []inferenceService.ContentBlock
			if text := contentText(m.Content); text != "" {
				blocks = append(blocks, inferenceService.ContentBlock{Text: text})
			}
			for _, tc := range m.ToolCalls {
				var input map[string]any
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
				blocks = append(blocks, inferenceService.ContentBlock{
					ToolUse: &inferenceService.ToolUseBlock{
						ToolUseID: tc.ID,
						Name:      tc.Function.Name,
						Input:     input,
					},
				})
			}
			if len(blocks) == 0 {
				continue
			}
			msgRole := role
			if msgRole != "user" && msgRole != "assistant" {
				msgRole = "user"
			}
			messages = append(messages, inferenceService.ConverseMessage{Role: msgRole, Content: blocks})
		}
	}
	flushToolResults()

	out.Messages = messages
	out.System = strings.Join(systemParts, "\n\n")

	choice, dropTools := toolChoice(req.ToolChoice)
	if !dropTools && len(req.Tools) > 0 {
		tools := make([]inferenceService.ToolSpec, 0, len(req.Tools))
		for _, t := range req.Tools {
			if t.Type != "" && t.Type != "function" {
				continue
			}
			tools = append(tools, inferenceService.ToolSpec{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: t.Function.Parameters,
			})
		}
		out.Tools = tools
		out.ToolChoice = choice
	}

	if req.MaxCompletionTokens != nil {
		out.MaxTokens = req.MaxCompletionTokens
	} else {
		out.MaxTokens = req.MaxTokens
	}
	out.Temperature = req.Temperature
	out.TopP = req.TopP
	out.StopSequences = stopSequences(req.Stop)

	return out
}
