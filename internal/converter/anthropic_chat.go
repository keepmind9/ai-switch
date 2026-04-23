package converter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/keepmind9/ai-switch/internal/types"
)

// Anthropic → Chat conversions (for clients sending Anthropic format)

// AnthropicRequest represents an Anthropic Messages API request.
type AnthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []AnthropicMessage `json:"messages"`
	System      any                `json:"system,omitempty"` // string or []AnthropicSystemBlock
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	TopP        float64            `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Metadata    map[string]any     `json:"metadata,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice  any                `json:"tool_choice,omitempty"`
}

type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []AnthropicContentBlock
}

type AnthropicContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any    `json:"content,omitempty"`
}

type AnthropicSystemBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// AnthropicResponse represents an Anthropic Messages API response.
type AnthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []AnthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      AnthropicUsage          `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// AnthropicToChat converts an Anthropic Messages request to a Chat Completions request.
func (c *Converter) AnthropicToChat(req *AnthropicRequest) (*types.ChatRequest, error) {
	chatReq := &types.ChatRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	// Convert tools
	for _, t := range req.Tools {
		chatReq.Tools = append(chatReq.Tools, types.Tool{
			Type: "function",
			Function: types.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	// Convert tool_choice
	chatReq.ToolChoice = anthropicToolChoiceToChat(req.ToolChoice)

	// Extract system prompt
	if req.System != nil {
		systemText := extractSystemText(req.System)
		if systemText != "" {
			chatReq.Messages = append(chatReq.Messages, types.ChatMessage{
				Role:    "system",
				Content: systemText,
			})
		}
	}

	// Convert messages
	for _, msg := range req.Messages {
		chatMsgs := anthropicMessageToChat(msg)
		chatReq.Messages = append(chatReq.Messages, chatMsgs...)
	}

	return chatReq, nil
}

// ChatToAnthropic converts a Chat Completions response to an Anthropic Messages response.
func (c *Converter) ChatToAnthropic(chatResp *types.ChatResponse, model, thinkTag string) (*AnthropicResponse, error) {
	var content []AnthropicContentBlock
	var stopReason string

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]

		text := StripThinkTag(choice.Message.Content, thinkTag)
		if text != "" {
			content = append(content, AnthropicContentBlock{
				Type: "text",
				Text: text,
			})
		}

		for _, tc := range choice.Message.ToolCalls {
			var input any
			if tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &input)
			}
			content = append(content, AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}

		if len(content) == 0 {
			content = append(content, AnthropicContentBlock{Type: "text", Text: ""})
		}

		stopReason = chatStopToAnthropic(choice.FinishReason)
	}

	return &AnthropicResponse{
		ID:         chatResp.ID,
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      model,
		StopReason: stopReason,
		Usage: AnthropicUsage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
		},
	}, nil
}

// Chat request → Anthropic request (when upstream is Anthropic format)

// ChatRequestToAnthropic converts a Chat Completions request to an Anthropic Messages request.
func (c *Converter) ChatRequestToAnthropic(req *types.ChatRequest) (*AnthropicRequest, error) {
	anthReq := &AnthropicRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	if anthReq.MaxTokens == 0 {
		anthReq.MaxTokens = 4096
	}

	// Convert tools
	for _, t := range req.Tools {
		anthReq.Tools = append(anthReq.Tools, AnthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	// Convert tool_choice
	anthReq.ToolChoice = chatToolChoiceToAnthropic(req.ToolChoice)

	// Convert messages
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			anthReq.System = msg.Content
			continue
		}
		anthMsgs := chatMessageToAnthropic(msg)
		anthReq.Messages = append(anthReq.Messages, anthMsgs...)
	}

	return anthReq, nil
}

// AnthropicResponseToChat converts an Anthropic Messages response to a Chat Completions response.
func (c *Converter) AnthropicResponseToChat(resp *AnthropicResponse) (*types.ChatResponse, error) {
	var contentText string
	var toolCalls []types.ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			contentText += block.Text
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: types.FunctionCall{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		}
	}

	msg := types.ChatMessage{Role: "assistant", Content: contentText}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	return &types.ChatResponse{
		ID:     resp.ID,
		Object: "chat.completion",
		Model:  resp.Model,
		Choices: []types.ChatChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: anthropicStopToChat(resp.StopReason),
			},
		},
		Usage: types.ChatUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}, nil
}

// extractSystemText extracts system text from string or []AnthropicSystemBlock.
func extractSystemText(system any) string {
	switch v := system.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return fmt.Sprintf("%v", system)
}

// extractContentText extracts text from string or []AnthropicContentBlock content.
func extractContentText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// anthropicMessageToChat converts a single Anthropic message to one or more Chat messages.
func anthropicMessageToChat(msg AnthropicMessage) []types.ChatMessage {
	if s, ok := msg.Content.(string); ok {
		return []types.ChatMessage{{Role: msg.Role, Content: s}}
	}

	blocks, ok := msg.Content.([]any)
	if !ok {
		return []types.ChatMessage{{Role: msg.Role, Content: extractContentText(msg.Content)}}
	}

	var textParts []string
	var toolCalls []types.ToolCall
	var toolResults []types.ChatMessage

	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch block["type"] {
		case "text":
			if text, ok := block["text"].(string); ok {
				textParts = append(textParts, text)
			}
		case "tool_use":
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			argsJSON, _ := json.Marshal(block["input"])
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   id,
				Type: "function",
				Function: types.FunctionCall{
					Name:      name,
					Arguments: string(argsJSON),
				},
			})
		case "tool_result":
			toolUseID, _ := block["tool_use_id"].(string)
			toolResults = append(toolResults, types.ChatMessage{
				Role:       "tool",
				Content:    extractToolResultContent(block["content"]),
				ToolCallID: toolUseID,
			})
		}
	}

	if msg.Role == "assistant" && len(toolCalls) > 0 {
		m := types.ChatMessage{Role: "assistant", ToolCalls: toolCalls}
		if len(textParts) > 0 {
			m.Content = strings.Join(textParts, "\n")
		}
		return []types.ChatMessage{m}
	}

	var result []types.ChatMessage
	if len(textParts) > 0 {
		result = append(result, types.ChatMessage{
			Role:    msg.Role,
			Content: strings.Join(textParts, "\n"),
		})
	}
	result = append(result, toolResults...)
	return result
}

// chatMessageToAnthropic converts a single Chat message to one or more Anthropic messages.
func chatMessageToAnthropic(msg types.ChatMessage) []AnthropicMessage {
	if msg.Role == "tool" {
		return []AnthropicMessage{{
			Role: "user",
			Content: []any{map[string]any{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     msg.Content,
			}},
		}}
	}

	if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
		var blocks []any
		if msg.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": msg.Content})
		}
		for _, tc := range msg.ToolCalls {
			var input any
			if tc.Function.Arguments != "" {
				json.Unmarshal([]byte(tc.Function.Arguments), &input)
			}
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}
		return []AnthropicMessage{{Role: "assistant", Content: blocks}}
	}

	return []AnthropicMessage{{Role: msg.Role, Content: msg.Content}}
}

func extractToolResultContent(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return fmt.Sprintf("%v", content)
}

func anthropicToolChoiceToChat(choice any) any {
	if choice == nil {
		return nil
	}
	m, ok := choice.(map[string]any)
	if !ok {
		return choice
	}
	t, _ := m["type"].(string)
	switch t {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		name, _ := m["name"].(string)
		return map[string]any{
			"type":     "function",
			"function": map[string]any{"name": name},
		}
	}
	return choice
}

func chatToolChoiceToAnthropic(choice any) any {
	if choice == nil {
		return nil
	}
	switch v := choice.(type) {
	case string:
		switch v {
		case "auto":
			return map[string]any{"type": "auto"}
		case "required":
			return map[string]any{"type": "any"}
		case "none":
			return nil
		}
	case map[string]any:
		if v["type"] == "function" {
			fn, _ := v["function"].(map[string]any)
			name, _ := fn["name"].(string)
			return map[string]any{"type": "tool", "name": name}
		}
	}
	return choice
}

func chatStopToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return reason
	}
}

func anthropicStopToChat(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return reason
	}
}
