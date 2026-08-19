package inference

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// usesMantleResponses reports whether this Bedrock model ID is an OpenAI
// GPT-5.x frontier model. Those are served only on the Mantle Responses
// API, not Converse / InvokeModel. GPT-OSS (and every other vendor) stay
// on Converse.
func usesMantleResponses(modelID string) bool {
	id := mantleModelID(modelID)
	if strings.HasPrefix(id, "openai.gpt-oss") {
		return false
	}
	return strings.HasPrefix(id, "openai.gpt-5")
}

// mantleModelID turns a catalogue model_id (foundation id, geo inference
// profile, or inference-profile ARN) into the id Mantle's Responses API
// expects, e.g. "openai.gpt-5.6-sol".
func mantleModelID(raw string) string {
	id := strings.TrimSpace(raw)
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	for _, prefix := range []string{"us.", "eu.", "apac.", "global.", "us-gov."} {
		if strings.HasPrefix(id, prefix+"openai.") {
			return strings.TrimPrefix(id, prefix)
		}
	}
	return id
}

func chatRequestToConverse(req ChatRequest) ConverseRequest {
	messages := make([]ConverseMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = ConverseMessage{Role: m.Role, Content: []ContentBlock{{Text: m.Content}}}
	}
	out := ConverseRequest{Messages: messages, Stream: req.WantsStream()}
	if req.MaxTokens > 0 {
		maxTokens := req.MaxTokens
		out.MaxTokens = &maxTokens
	}
	if req.Temperature > 0 {
		temp := req.Temperature
		out.Temperature = &temp
	}
	return out
}

type responsesInputItem struct {
	Type      string `json:"type,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   any    `json:"content,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type responsesRequest struct {
	Model           string               `json:"model"`
	Input           []responsesInputItem `json:"input"`
	Instructions    string               `json:"instructions,omitempty"`
	MaxOutputTokens *int                 `json:"max_output_tokens,omitempty"`
	Temperature     *float64             `json:"temperature,omitempty"`
	TopP            *float64             `json:"top_p,omitempty"`
	Tools           []responsesTool      `json:"tools,omitempty"`
	ToolChoice      any                  `json:"tool_choice,omitempty"`
	Stream          bool                 `json:"stream,omitempty"`
	Store           bool                 `json:"store"`
}

// responsesBody maps a ConverseRequest onto OpenAI's Responses API shape
// for Bedrock Mantle. store is always false so Orbit keeps its own history
// and Bedrock does not retain the turn.
func responsesBody(modelID string, req ConverseRequest) ([]byte, error) {
	payload := responsesRequest{
		Model:        mantleModelID(modelID),
		Input:        converseToResponsesInput(req),
		Instructions: req.System,
		Stream:       req.Stream,
		Store:        false,
	}
	payload.MaxOutputTokens = req.MaxTokens
	payload.Temperature = req.Temperature
	payload.TopP = req.TopP

	if len(req.Tools) > 0 {
		tools := make([]responsesTool, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = responsesTool{
				Type:        "function",
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			}
		}
		payload.Tools = tools
		if req.ToolChoice != nil {
			switch req.ToolChoice.Mode {
			case "any":
				payload.ToolChoice = "required"
			case "tool":
				payload.ToolChoice = map[string]string{"type": "function", "name": req.ToolChoice.ToolName}
			default:
				payload.ToolChoice = "auto"
			}
		}
	}

	return json.Marshal(payload)
}

func converseToResponsesInput(req ConverseRequest) []responsesInputItem {
	items := make([]responsesInputItem, 0, len(req.Messages)*2)
	for _, msg := range req.Messages {
		var textParts []string
		for _, block := range msg.Content {
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
			if block.ToolUse != nil {
				if text := strings.Join(textParts, "\n"); text != "" {
					items = append(items, responsesInputItem{Role: msg.Role, Content: text})
					textParts = nil
				}
				args, err := json.Marshal(block.ToolUse.Input)
				if err != nil {
					args = []byte("{}")
				}
				items = append(items, responsesInputItem{
					Type:      "function_call",
					CallID:    block.ToolUse.ToolUseID,
					Name:      block.ToolUse.Name,
					Arguments: string(args),
				})
			}
			if block.ToolResult != nil {
				if text := strings.Join(textParts, "\n"); text != "" {
					items = append(items, responsesInputItem{Role: msg.Role, Content: text})
					textParts = nil
				}
				items = append(items, responsesInputItem{
					Type:   "function_call_output",
					CallID: block.ToolResult.ToolUseID,
					Output: toolResultText(block.ToolResult),
				})
			}
		}
		if text := strings.Join(textParts, "\n"); text != "" {
			items = append(items, responsesInputItem{Role: msg.Role, Content: text})
		}
	}
	return items
}

func toolResultText(result *ToolResultBlock) string {
	if result == nil {
		return ""
	}
	parts := make([]string, 0, len(result.Content))
	for _, block := range result.Content {
		if block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

type responsesAPIResponse struct {
	Status string `json:"status"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []responsesOutputItem `json:"output"`
	Usage  *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type responsesOutputItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responsesStatusError struct {
	Status       string
	Message      string
	InputTokens  int
	OutputTokens int
}

func (e *responsesStatusError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "model response failed"
}

func responsesToConverseJSON(body []byte, latencyMS int) ([]byte, int, int, error) {
	var parsed responsesAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, 0, 0, err
	}

	inputTokens, outputTokens := 0, 0
	if parsed.Usage != nil {
		inputTokens = parsed.Usage.InputTokens
		outputTokens = parsed.Usage.OutputTokens
	}

	if parsed.Status == "failed" || parsed.Status == "cancelled" {
		msg := "model response failed"
		if parsed.Error != nil && parsed.Error.Message != "" {
			msg = parsed.Error.Message
		}
		return nil, inputTokens, outputTokens, &responsesStatusError{
			Status:       parsed.Status,
			Message:      msg,
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		}
	}

	var content []ContentBlock
	stopReason := "end_turn"
	for _, item := range parsed.Output {
		switch item.Type {
		case "message", "":
			for _, part := range item.Content {
				if part.Text != "" {
					content = append(content, ContentBlock{Text: part.Text})
				}
			}
		case "function_call":
			var input map[string]any
			if item.Arguments != "" {
				_ = json.Unmarshal([]byte(item.Arguments), &input)
			}
			if input == nil {
				input = map[string]any{}
			}
			content = append(content, ContentBlock{ToolUse: &ToolUseBlock{
				ToolUseID: item.CallID,
				Name:      item.Name,
				Input:     input,
			}})
			stopReason = "tool_use"
		}
	}
	if parsed.Status == "incomplete" {
		if parsed.IncompleteDetails != nil && strings.Contains(parsed.IncompleteDetails.Reason, "max") {
			stopReason = "max_tokens"
		}
	}

	wire := struct {
		Output struct {
			Message struct {
				Role    string         `json:"role"`
				Content []ContentBlock `json:"content"`
			} `json:"message"`
		} `json:"output"`
		StopReason string `json:"stopReason"`
		Usage      struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		} `json:"usage"`
		Metrics struct {
			LatencyMS int `json:"latencyMs"`
		} `json:"metrics"`
	}{}
	wire.Output.Message.Role = "assistant"
	wire.Output.Message.Content = content
	wire.StopReason = stopReason
	wire.Usage.InputTokens = inputTokens
	wire.Usage.OutputTokens = outputTokens
	wire.Metrics.LatencyMS = latencyMS

	encoded, err := json.Marshal(wire)
	return encoded, inputTokens, outputTokens, err
}

func failedProviderBody(err error) []byte {
	msg := "failed to parse model response"
	var failed *responsesStatusError
	if errors.As(err, &failed) && failed.Message != "" {
		msg = failed.Message
	}
	return normalizeProviderError([]byte(fmt.Sprintf(`{"message":%q}`, msg)))
}

func normalizeProviderError(body []byte) []byte {
	msg := BedrockErrorMessage(body)
	encoded, err := json.Marshal(map[string]string{"message": msg})
	if err != nil {
		return []byte(`{"message":"upstream model provider error"}`)
	}
	return encoded
}
