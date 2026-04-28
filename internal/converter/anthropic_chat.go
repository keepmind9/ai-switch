package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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

// ResponsesToAnthropic converts a Responses API request directly to an Anthropic Messages request.
func (c *Converter) ResponsesToAnthropic(req *types.ResponsesRequest) (*AnthropicRequest, error) {
	anthReq := &AnthropicRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Metadata:    req.Metadata,
	}

	if anthReq.MaxTokens == 0 {
		anthReq.MaxTokens = 4096
	}

	if req.Instructions != "" {
		anthReq.System = req.Instructions
	}

	// Convert input items to Anthropic messages
	anthReq.Messages = convertResponsesInputToAnthropicMessages(req.Input)

	// Convert tools (skip built-in tools without name)
	for _, t := range filterFunctionTools(req.Tools) {
		schema := t.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		anthReq.Tools = append(anthReq.Tools, AnthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}

	// Convert tool_choice
	anthReq.ToolChoice = responsesToolChoiceToAnthropic(req.ToolChoice)

	return anthReq, nil
}

// convertResponsesInputToAnthropicMessages converts Responses API input items to Anthropic messages.
func convertResponsesInputToAnthropicMessages(input any) []AnthropicMessage {
	if input == nil {
		return nil
	}

	switch v := input.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []AnthropicMessage{{Role: "user", Content: v}}
	case []any:
		var msgs []AnthropicMessage
		for _, item := range v {
			switch val := item.(type) {
			case string:
				if val != "" {
					msgs = append(msgs, AnthropicMessage{Role: "user", Content: val})
				}
			case map[string]any:
				itemType, _ := val["type"].(string)
				switch itemType {
				case "function_call":
					callID, _ := val["call_id"].(string)
					name, _ := val["name"].(string)
					argsStr, _ := val["arguments"].(string)
					var input any = map[string]any{}
					if argsStr != "" {
						json.Unmarshal([]byte(argsStr), &input)
					}
					msgs = append(msgs, AnthropicMessage{
						Role: "assistant",
						Content: []any{map[string]any{
							"type":  "tool_use",
							"id":    callID,
							"name":  name,
							"input": input,
						}},
					})
				case "function_call_output":
					callID, _ := val["call_id"].(string)
					output, _ := val["output"].(string)
					msgs = append(msgs, AnthropicMessage{
						Role: "user",
						Content: []any{map[string]any{
							"type":        "tool_result",
							"tool_use_id": callID,
							"content":     output,
						}},
					})
				default:
					// message format: extract text from content array
					role := NormalizeRole(val["role"].(string))
					text := extractMessageContentText(val)
					if text != "" {
						msgs = append(msgs, AnthropicMessage{Role: role, Content: text})
					}
				}
			}
		}
		return msgs
	}
	return nil
}

// extractMessageContentText extracts text from a message object's content array.
func extractMessageContentText(msg map[string]any) string {
	content, ok := msg["content"]
	if !ok {
		return ""
	}
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, item := range c {
			if cMap, ok := item.(map[string]any); ok {
				if text, ok := cMap["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func responsesToolChoiceToAnthropic(choice any) any {
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

// AnthropicResponseToResponses converts an Anthropic Messages response directly to a Responses API response.
func (c *Converter) AnthropicResponseToResponses(resp *AnthropicResponse, model, thinkTag string) (*types.ResponsesResponse, error) {
	now := time.Now().Unix()

	var items []types.ResponseItem

	// Collect text blocks into a message item
	var textParts []string
	for _, block := range resp.Content {
		if block.Type == "text" {
			textParts = append(textParts, block.Text)
		}
	}
	combinedText := StripThinkTag(strings.Join(textParts, ""), thinkTag)
	if combinedText != "" || len(resp.Content) == 0 {
		items = append(items, types.ResponseItem{
			ID:      fmt.Sprintf("resp_%d", now),
			Type:    "message",
			Object:  "response",
			Created: now,
			Role:    "assistant",
			Content: []types.ContentBlock{
				{Type: "output_text", Text: combinedText},
			},
			Status: "completed",
		})
	}

	// Convert tool_use blocks to function_call items
	for _, block := range resp.Content {
		if block.Type == "tool_use" {
			argsJSON, _ := json.Marshal(block.Input)
			items = append(items, types.ResponseItem{
				ID:        fmt.Sprintf("fc_%s", block.ID),
				Type:      "function_call",
				Object:    "response",
				Created:   now,
				Status:    "completed",
				CallID:    block.ID,
				Name:      block.Name,
				Arguments: string(argsJSON),
			})
		}
	}

	if len(items) == 0 {
		items = append(items, types.ResponseItem{
			ID:      fmt.Sprintf("resp_%d", now),
			Type:    "message",
			Object:  "response",
			Created: now,
			Role:    "assistant",
			Content: []types.ContentBlock{{Type: "output_text", Text: ""}},
			Status:  "completed",
		})
	}

	return &types.ResponsesResponse{
		ID:        resp.ID,
		Object:    "response",
		Created:   now,
		Model:     model,
		Responses: items,
		Usage: &types.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}, nil
}

// filterFunctionTools returns only tools that have a name (function tools),
// skipping built-in tools like web_search that lack name/parameters.
func filterFunctionTools(tools []types.ResponsesTool) []types.ResponsesTool {
	var result []types.ResponsesTool
	for _, t := range tools {
		if t.Name == "" {
			continue
		}
		// Expand namespace tools (Codex MCP) into flat function tools.
		if t.Type == "namespace" && len(t.Tools) > 0 {
			for _, sub := range t.Tools {
				if sub.Name == "" {
					continue
				}
				desc := sub.Description
				if desc == "" {
					desc = t.Description
				}
				result = append(result, types.ResponsesTool{
					Type:        "function",
					Name:        sub.Name,
					Description: desc,
					Parameters:  sub.Parameters,
				})
			}
			continue
		}
		result = append(result, t)
	}
	return result
}

// AnthropicToResponses converts an Anthropic Messages request directly to a Responses API request.
func (c *Converter) AnthropicToResponses(req *AnthropicRequest) (*types.ResponsesRequest, error) {
	respReq := &types.ResponsesRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Metadata:    req.Metadata,
	}

	if req.MaxTokens == 0 {
		respReq.MaxTokens = 4096
	}

	if req.System != nil {
		respReq.Instructions = extractSystemText(req.System)
	}

	// Convert messages → input items
	respReq.Input = convertAnthropicMessagesToResponsesInput(req.Messages)

	// Convert tools
	for _, t := range req.Tools {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		respReq.Tools = append(respReq.Tools, types.ResponsesTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
		})
	}

	// Convert tool_choice
	respReq.ToolChoice = anthropicToolChoiceToResponses(req.ToolChoice)

	return respReq, nil
}

func convertAnthropicMessagesToResponsesInput(msgs []AnthropicMessage) any {
	var items []any
	for _, msg := range msgs {
		converted := anthropicMessageToResponsesItems(msg)
		items = append(items, converted...)
	}
	if len(items) == 0 {
		return nil
	}
	if len(items) == 1 {
		if s, ok := items[0].(string); ok {
			return s
		}
		return items
	}
	return items
}

func anthropicMessageToResponsesItems(msg AnthropicMessage) []any {
	if s, ok := msg.Content.(string); ok {
		if s == "" {
			return nil
		}
		return []any{s}
	}

	blocks, ok := msg.Content.([]any)
	if !ok {
		text := extractContentText(msg.Content)
		if text == "" {
			return nil
		}
		return []any{text}
	}

	var items []any
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch block["type"] {
		case "text":
			if text, ok := block["text"].(string); ok && text != "" {
				items = append(items, text)
			}
		case "tool_use":
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			argsJSON, _ := json.Marshal(block["input"])
			items = append(items, map[string]any{
				"type":      "function_call",
				"call_id":   id,
				"name":      name,
				"arguments": string(argsJSON),
			})
		case "tool_result":
			toolUseID, _ := block["tool_use_id"].(string)
			content := extractToolResultContent(block["content"])
			items = append(items, map[string]any{
				"type":    "function_call_output",
				"call_id": toolUseID,
				"output":  content,
			})
		}
	}
	return items
}

func anthropicToolChoiceToResponses(choice any) any {
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
