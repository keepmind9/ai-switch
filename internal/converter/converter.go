package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

type Converter struct{}

// Format constants for protocol identification.
const (
	FormatChat      = "chat"
	FormatResponses = "responses"
	FormatAnthropic = "anthropic"
)

// ConvertedRequest holds the result of request conversion.
type ConvertedRequest struct {
	UpstreamBody []byte
	Model        string
	IsStreaming  bool
}

// ConvertRequest converts a client request to upstream format.
// clientFormat is "responses", "anthropic", or "chat".
// upstreamFormat is from config. body is raw JSON.
func (c *Converter) ConvertRequest(clientFormat, upstreamFormat string, body []byte, defaultModel string, modelMap map[string]string) (*ConvertedRequest, error) {
	resolveModel := func(m string) string {
		if modelMap != nil {
			if mapped, ok := modelMap[m]; ok {
				return mapped
			}
		}
		return m
	}

	// Same format: passthrough with model resolution
	if clientFormat == upstreamFormat || (upstreamFormat == "" && clientFormat == FormatChat) {
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, err
		}

		model := defaultModel
		if m, ok := raw["model"].(string); ok && m != "" {
			model = m
		}
		model = resolveModel(model)
		raw["model"] = model

		isStreaming := false
		if s, ok := raw["stream"].(bool); ok {
			isStreaming = s
		}

		newBody, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}

		return &ConvertedRequest{
			UpstreamBody: newBody,
			Model:        model,
			IsStreaming:  isStreaming,
		}, nil
	}

	// Cross-format conversion through Chat hub
	switch clientFormat {
	case FormatResponses:
		return c.convertResponsesRequest(upstreamFormat, body, defaultModel, resolveModel)
	case FormatAnthropic:
		return c.convertAnthropicRequest(upstreamFormat, body, defaultModel, resolveModel)
	case FormatChat:
		return c.convertChatRequest(upstreamFormat, body, defaultModel, resolveModel)
	}

	return nil, fmt.Errorf("unsupported client format: %s", clientFormat)
}

func (c *Converter) convertResponsesRequest(upstreamFormat string, body []byte, defaultModel string, resolveModel func(string) string) (*ConvertedRequest, error) {
	var req types.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = defaultModel
	}

	switch upstreamFormat {
	case FormatChat, "":
		chatReq, err := c.ResponsesToChat(&req)
		if err != nil {
			return nil, err
		}
		chatReq.Model = resolveModel(defaultModel)
		chatBody, err := json.Marshal(chatReq)
		if err != nil {
			return nil, err
		}
		return &ConvertedRequest{UpstreamBody: chatBody, Model: model, IsStreaming: req.Stream}, nil

	case FormatAnthropic:
		anthReq, err := c.ResponsesToAnthropic(&req)
		if err != nil {
			return nil, err
		}
		anthReq.Model = resolveModel(model)
		anthReq.Stream = req.Stream
		anthBody, err := json.Marshal(anthReq)
		if err != nil {
			return nil, err
		}
		return &ConvertedRequest{UpstreamBody: anthBody, Model: model, IsStreaming: req.Stream}, nil

	case FormatResponses:
		// Passthrough handled above, shouldn't reach here
	}

	return nil, fmt.Errorf("unsupported upstream format for responses client: %s", upstreamFormat)
}

func (c *Converter) convertAnthropicRequest(upstreamFormat string, body []byte, defaultModel string, resolveModel func(string) string) (*ConvertedRequest, error) {
	var req AnthropicRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = defaultModel
	}

	switch upstreamFormat {
	case FormatChat, "":
		chatReq, err := c.AnthropicToChat(&req)
		if err != nil {
			return nil, err
		}
		chatReq.Model = resolveModel(defaultModel)
		chatBody, err := json.Marshal(chatReq)
		if err != nil {
			return nil, err
		}
		return &ConvertedRequest{UpstreamBody: chatBody, Model: model, IsStreaming: req.Stream}, nil

	case FormatResponses:
		respReq, err := c.AnthropicToResponses(&req)
		if err != nil {
			return nil, err
		}
		respReq.Model = resolveModel(model)
		respBody, err := json.Marshal(respReq)
		if err != nil {
			return nil, err
		}
		return &ConvertedRequest{UpstreamBody: respBody, Model: model, IsStreaming: req.Stream}, nil

	case FormatAnthropic:
		// Passthrough handled above
	}

	return nil, fmt.Errorf("unsupported upstream format for anthropic client: %s", upstreamFormat)
}

func (c *Converter) convertChatRequest(upstreamFormat string, body []byte, defaultModel string, resolveModel func(string) string) (*ConvertedRequest, error) {
	var req types.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = defaultModel
	}

	switch upstreamFormat {
	case FormatAnthropic:
		anthReq, err := c.ChatRequestToAnthropic(&req)
		if err != nil {
			return nil, err
		}
		anthReq.Model = resolveModel(model)
		anthReq.Stream = req.Stream
		anthBody, err := json.Marshal(anthReq)
		if err != nil {
			return nil, err
		}
		return &ConvertedRequest{UpstreamBody: anthBody, Model: model, IsStreaming: req.Stream}, nil

	case FormatChat, "":
	case FormatResponses:
		// Passthrough for both
	}

	return nil, fmt.Errorf("unsupported upstream format for chat client: %s", upstreamFormat)
}

func BuildResponsesFromChat(chatReq *types.ChatRequest, stream bool) *types.ResponsesRequest {
	var instructions string
	var inputItems []any
	for _, msg := range chatReq.Messages {
		if msg.Role == "system" {
			instructions += msg.Content + "\n"
			continue
		}
		if msg.Role == "tool" {
			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  msg.Content,
			})
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			if msg.Content != "" {
				inputItems = append(inputItems, msg.Content)
			}
			for _, tc := range msg.ToolCalls {
				inputItems = append(inputItems, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
			continue
		}
		if msg.Content != "" {
			inputItems = append(inputItems, msg.Content)
		}
	}

	var tools []types.ResponsesTool
	for _, t := range chatReq.Tools {
		tools = append(tools, types.ResponsesTool{
			Type:        "function",
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}

	var input any
	if len(inputItems) == 1 {
		if s, ok := inputItems[0].(string); ok {
			input = s
		} else {
			input = inputItems
		}
	} else if len(inputItems) > 1 {
		input = inputItems
	}

	return &types.ResponsesRequest{
		Model:        chatReq.Model,
		Input:        input,
		Instructions: strings.TrimSpace(instructions),
		Stream:       stream,
		MaxTokens:    chatReq.MaxTokens,
		Temperature:  chatReq.Temperature,
		TopP:         chatReq.TopP,
		Tools:        tools,
		ToolChoice:   chatReq.ToolChoice,
	}
}

// ResponsesToChatResponse converts a Responses API response back to Chat Completions format.
func (c *Converter) ResponsesToChatResponse(resp *types.ResponsesResponse) (*types.ChatResponse, error) {
	chatResp := &types.ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: resp.Created,
		Model:   resp.Model,
	}
	if resp.Usage != nil {
		chatResp.Usage = types.ChatUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	var contentText string
	var toolCalls []types.ToolCall
	for _, item := range resp.Responses {
		switch item.Type {
		case "message", "":
			for _, block := range item.Content {
				if block.Type == "output_text" {
					contentText += block.Text
				}
			}
		case "function_call":
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   item.CallID,
				Type: "function",
				Function: types.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	if contentText != "" || len(toolCalls) > 0 {
		chatResp.Choices = []types.ChatChoice{{
			Index:        0,
			Message:      types.ChatMessage{Role: "assistant", Content: contentText, ToolCalls: toolCalls},
			FinishReason: finishReason,
		}}
	}
	return chatResp, nil
}

func NewConverter() *Converter {
	return &Converter{}
}

// NormalizeRole maps unsupported roles to "user".
func NormalizeRole(role string) string {
	if role == "" || role == "system" || role == "developer" {
		return "user"
	}
	return role
}

// ResponsesToChat converts OpenAI Responses API request to Chat Completions API request
func (c *Converter) ResponsesToChat(req *types.ResponsesRequest) (*types.ChatRequest, error) {
	chatReq := &types.ChatRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}

	// Convert tools (skip built-in tools without name)
	for _, t := range filterFunctionTools(req.Tools) {
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object"}
		}
		chatReq.Tools = append(chatReq.Tools, types.Tool{
			Type: "function",
			Function: types.FunctionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	chatReq.ToolChoice = req.ToolChoice

	// Add instructions as system prompt if present
	if req.Instructions != "" {
		chatReq.Messages = append(chatReq.Messages, types.ChatMessage{
			Role:    "system",
			Content: req.Instructions,
		})
	}

	// Handle input - can be string, array of items, or array of message objects
	chatReq.Messages = append(chatReq.Messages, convertResponsesInputToChatMessages(req.Input)...)

	return chatReq, nil
}

// convertResponsesInputToChatMessages converts Responses API input items to Chat messages.
func convertResponsesInputToChatMessages(input any) []types.ChatMessage {
	if input == nil {
		return nil
	}

	switch v := input.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []types.ChatMessage{{Role: "user", Content: v}}
	case []any:
		var msgs []types.ChatMessage
		for _, item := range v {
			switch val := item.(type) {
			case string:
				if val != "" {
					msgs = append(msgs, types.ChatMessage{Role: "user", Content: val})
				}
			case map[string]any:
				itemType, _ := val["type"].(string)
				switch itemType {
				case "function_call":
					callID, _ := val["call_id"].(string)
					name, _ := val["name"].(string)
					args, _ := val["arguments"].(string)
					msgs = append(msgs, types.ChatMessage{
						Role: "assistant",
						ToolCalls: []types.ToolCall{
							{ID: callID, Type: "function", Function: types.FunctionCall{Name: name, Arguments: args}},
						},
					})
				case "function_call_output":
					callID, _ := val["call_id"].(string)
					output, _ := val["output"].(string)
					msgs = append(msgs, types.ChatMessage{
						Role:       "tool",
						ToolCallID: callID,
						Content:    output,
					})
				default:
					// message format
					role := NormalizeRole(val["role"].(string))
					text := extractInputTextMessage(val)
					if text != "" {
						msgs = append(msgs, types.ChatMessage{Role: role, Content: text})
					}
				}
			}
		}
		return msgs
	}
	return nil
}

// extractInputTextMessage extracts text from a message object's content array.
func extractInputTextMessage(msg map[string]any) string {
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

// extractInputText extracts text from various input formats
// Handles: string, []string, or []message objects (Codex format)
func extractInputText(input any) string {
	if input == nil {
		return ""
	}

	switch v := input.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			switch msg := item.(type) {
			case map[string]any:
				// Handle message format: {"type": "message", "role": "...", "content": [...]}
				if content, ok := msg["content"].([]any); ok {
					for _, c := range content {
						if cMap, ok := c.(map[string]any); ok {
							if text, ok := cMap["text"].(string); ok {
								parts = append(parts, text)
							}
						}
					}
				}
			case string:
				parts = append(parts, msg)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// ChatToResponses converts Chat Completions API response back to OpenAI Responses API format
func (c *Converter) ChatToResponses(chatResp *types.ChatResponse, model, thinkTag string) (*types.ResponsesResponse, error) {
	now := time.Now().Unix()
	var responseItems []types.ResponseItem
	for _, choice := range chatResp.Choices {
		// Text message item
		text := StripThinkTag(choice.Message.Content, thinkTag)
		if text != "" || len(choice.Message.ToolCalls) == 0 {
			responseItems = append(responseItems, types.ResponseItem{
				ID:      fmt.Sprintf("resp_%d", now),
				Type:    "message",
				Object:  "response",
				Created: now,
				Role:    "assistant",
				Content: []types.ContentBlock{
					{Type: "output_text", Text: text},
				},
				Status: "completed",
			})
		}
		// Function call items
		for _, tc := range choice.Message.ToolCalls {
			responseItems = append(responseItems, types.ResponseItem{
				ID:        fmt.Sprintf("fc_%s", tc.ID),
				Type:      "function_call",
				Object:    "response",
				Created:   now,
				Status:    "completed",
				CallID:    tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
	}

	resp := &types.ResponsesResponse{
		ID:        chatResp.ID,
		Object:    "response",
		Created:   chatResp.Created,
		Model:     model,
		Responses: responseItems,
		Usage: &types.Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		},
	}

	return resp, nil
}
