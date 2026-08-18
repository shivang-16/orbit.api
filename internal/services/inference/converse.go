package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ContentBlock is a tagged union mirroring Bedrock Converse's own content
// block shape closely enough to serve both as the internal representation
// shared by the OpenAI/Anthropic compat adapters and, via these same JSON
// tags, literally Bedrock's wire format for request messages and tool
// results. Exactly one field is set.
type ContentBlock struct {
	Text       string           `json:"text,omitempty"`
	ToolUse    *ToolUseBlock    `json:"toolUse,omitempty"`
	ToolResult *ToolResultBlock `json:"toolResult,omitempty"`
}

// ToolUseBlock is a model-issued (response) or replayed (request history)
// tool call: a tool name plus its arguments, already decoded to a JSON
// object (Bedrock's buffered Converse response gives us this directly;
// the streamed response instead delivers Input as partial JSON string
// fragments — see the OpenAI/Anthropic stream sinks for that assembly).
type ToolUseBlock struct {
	ToolUseID string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
}

// ToolResultBlock carries a tool's output back to the model, keyed by the
// ToolUseID from the ToolUseBlock it answers.
type ToolResultBlock struct {
	ToolUseID string         `json:"toolUseId"`
	Content   []ContentBlock `json:"content"`
	// Status is "success" or "error"; omitted (defaults to success on
	// Bedrock's side) when the caller doesn't distinguish.
	Status string `json:"status,omitempty"`
}

// ConverseMessage is one turn of conversation history. Role is "user" or
// "assistant" — system prompts are carried separately on ConverseRequest,
// matching Bedrock/Anthropic's convention (OpenAI's "system"/"developer"
// role messages are folded into that field by the OpenAI adapter).
type ConverseMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ToolSpec describes one callable tool, JSON-schema based like both
// OpenAI's and Anthropic's tool definitions.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// ToolChoice controls whether/which tool the model must use. Mode is one
// of "auto" (default if Tools is non-empty and ToolChoice is nil), "any"
// (must call some tool), or "tool" (must call ToolName specifically).
type ToolChoice struct {
	Mode     string
	ToolName string
}

// ConverseRequest is the neutral intermediate representation both the
// OpenAI and Anthropic compat adapters translate their dialect's request
// into, and the shape Service.Converse forwards to Bedrock. Pointer
// fields distinguish "not provided" from a meaningful zero value (e.g.
// Temperature: 0 is a valid, deterministic request).
type ConverseRequest struct {
	Messages      []ConverseMessage
	System        string
	Tools         []ToolSpec
	ToolChoice    *ToolChoice
	MaxTokens     *int
	Temperature   *float64
	TopP          *float64
	StopSequences []string
	Stream        bool
}

func (r ConverseRequest) isValid() bool {
	if len(r.Messages) == 0 {
		return false
	}
	for _, m := range r.Messages {
		if m.Role != "user" && m.Role != "assistant" {
			return false
		}
		if len(m.Content) == 0 {
			return false
		}
	}
	return true
}

// ConverseResponse is Bedrock's buffered Converse response, decoded into
// the same neutral shape, for the OpenAI/Anthropic adapters to format
// into their own dialect's response body.
type ConverseResponse struct {
	Role         string
	Content      []ContentBlock
	StopReason   string
	InputTokens  int
	OutputTokens int
	LatencyMS    int
}

// ParseConverseResponse decodes a buffered Bedrock Converse JSON response
// body (the 200 OK case; non-200 bodies are Bedrock's own error shape and
// should not be passed here).
func ParseConverseResponse(body []byte) (*ConverseResponse, error) {
	var parsed struct {
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
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return &ConverseResponse{
		Role:         parsed.Output.Message.Role,
		Content:      parsed.Output.Message.Content,
		StopReason:   parsed.StopReason,
		InputTokens:  parsed.Usage.InputTokens,
		OutputTokens: parsed.Usage.OutputTokens,
		LatencyMS:    parsed.Metrics.LatencyMS,
	}, nil
}

// BedrockErrorMessage best-effort extracts a human-readable message from
// a non-2xx Bedrock response body ({"message": "..."}), for the
// OpenAI/Anthropic compat controllers to embed in their own dialect's
// error envelope.
func BedrockErrorMessage(body []byte) string {
	var parsed struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Message == "" {
		return "upstream model provider error"
	}
	return parsed.Message
}

// Converse is the entry point used by the OpenAI/Anthropic compat
// controllers: like Chat, it runs the credit gate and model resolution,
// but accepts the richer Converse-shaped request (system prompt, tool
// use/result content blocks) instead of Chat's plain {role, content}
// messages. sink is only used when req.Stream is true; pass nil for a
// buffered call.
func (s *Service) Converse(ctx context.Context, modelIdentifier string, req ConverseRequest, w http.ResponseWriter, sink StreamSink) (*ChatResult, error) {
	if !req.isValid() {
		return nil, ErrInvalid
	}
	if err := s.requireCredits(ctx); err != nil {
		return nil, err
	}
	entry, err := s.resolveModel(ctx, modelIdentifier)
	if err != nil {
		return nil, err
	}

	payload, err := converseBody(req)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	if req.Stream {
		return s.chatStream(ctx, entry, payload, sink, w)
	}
	return s.chatOnce(ctx, entry, payload)
}

type toolChoiceEmpty struct{}

type toolChoiceNamed struct {
	Name string `json:"name"`
}

type toolChoiceWire struct {
	Auto *toolChoiceEmpty `json:"auto,omitempty"`
	Any  *toolChoiceEmpty `json:"any,omitempty"`
	Tool *toolChoiceNamed `json:"tool,omitempty"`
}

// converseBody maps a ConverseRequest onto Bedrock's actual Converse API
// wire body: messages/content blocks marshal directly via ContentBlock's
// JSON tags, while system/tools/tool_choice/inference params are Bedrock-
// specific wrapper shapes assembled here.
func converseBody(req ConverseRequest) ([]byte, error) {
	type systemBlock struct {
		Text string `json:"text"`
	}
	type toolSpecWire struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		InputSchema struct {
			JSON map[string]any `json:"json"`
		} `json:"inputSchema"`
	}
	type toolWire struct {
		ToolSpec toolSpecWire `json:"toolSpec"`
	}
	type toolConfigWire struct {
		Tools      []toolWire      `json:"tools"`
		ToolChoice *toolChoiceWire `json:"toolChoice,omitempty"`
	}
	type inferenceConfig struct {
		MaxTokens     int      `json:"maxTokens,omitempty"`
		Temperature   float64  `json:"temperature,omitempty"`
		TopP          float64  `json:"topP,omitempty"`
		StopSequences []string `json:"stopSequences,omitempty"`
	}

	payload := struct {
		Messages        []ConverseMessage `json:"messages"`
		System          []systemBlock     `json:"system,omitempty"`
		ToolConfig      *toolConfigWire   `json:"toolConfig,omitempty"`
		InferenceConfig *inferenceConfig  `json:"inferenceConfig,omitempty"`
	}{Messages: req.Messages}

	if req.System != "" {
		payload.System = []systemBlock{{Text: req.System}}
	}

	if len(req.Tools) > 0 {
		tools := make([]toolWire, len(req.Tools))
		for i, t := range req.Tools {
			tools[i].ToolSpec.Name = t.Name
			tools[i].ToolSpec.Description = t.Description
			tools[i].ToolSpec.InputSchema.JSON = t.InputSchema
		}
		toolConfig := &toolConfigWire{Tools: tools}
		if req.ToolChoice != nil {
			switch req.ToolChoice.Mode {
			case "any":
				toolConfig.ToolChoice = &toolChoiceWire{Any: &toolChoiceEmpty{}}
			case "tool":
				toolConfig.ToolChoice = &toolChoiceWire{Tool: &toolChoiceNamed{Name: req.ToolChoice.ToolName}}
			default:
				toolConfig.ToolChoice = &toolChoiceWire{Auto: &toolChoiceEmpty{}}
			}
		}
		payload.ToolConfig = toolConfig
	}

	var ic inferenceConfig
	hasIC := false
	if req.MaxTokens != nil {
		ic.MaxTokens = *req.MaxTokens
		hasIC = true
	}
	if req.Temperature != nil {
		ic.Temperature = *req.Temperature
		hasIC = true
	}
	if req.TopP != nil {
		ic.TopP = *req.TopP
		hasIC = true
	}
	if len(req.StopSequences) > 0 {
		ic.StopSequences = req.StopSequences
		hasIC = true
	}
	if hasIC {
		payload.InferenceConfig = &ic
	}

	return json.Marshal(payload)
}
