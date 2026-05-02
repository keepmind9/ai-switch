package converter

import (
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
	FormatGemini    = "gemini"
)

// ConvertedRequest holds the result of request conversion.
type ConvertedRequest struct {
	UpstreamBody []byte
	Model        string
	IsStreaming  bool
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

func BuildResponsesFromChat(chatReq *types.ChatRequest, stream bool) *types.ResponsesRequest {
	var instructions string
	var inputItems []any
	for _, msg := range chatReq.Messages {
		if msg.Role == "system" {
			instructions += derefStr(msg.Content) + "\n"
			continue
		}
		if msg.Role == "tool" {
			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  derefStr(msg.Content),
			})
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			if msg.Content != nil && *msg.Content != "" {
				inputItems = append(inputItems, *msg.Content)
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
		if msg.Content != nil && *msg.Content != "" {
			inputItems = append(inputItems, *msg.Content)
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
			Content: strPtr(req.Instructions),
		})
	}

	// Handle input - can be string, array of items, or array of message objects
	chatReq.Messages = append(chatReq.Messages, convertResponsesInputToChatMessages(req.Input)...)

	return chatReq, nil
}

// convertResponsesInputToChatMessages converts Responses API input items to Chat messages.
// Consecutive function_call items are merged into one assistant message (multiple tool_calls),
// as required by the Chat Completions API.
func convertResponsesInputToChatMessages(input any) []types.ChatMessage {
	if input == nil {
		return nil
	}

	switch v := input.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []types.ChatMessage{{Role: "user", Content: strPtr(v)}}
	case []any:
		var msgs []types.ChatMessage
		var pendingToolCalls []types.ToolCall

		flushToolCalls := func() {
			if len(pendingToolCalls) > 0 {
				msgs = append(msgs, types.ChatMessage{
					Role:      "assistant",
					Content:   strPtr(""),
					ToolCalls: pendingToolCalls,
				})
				pendingToolCalls = nil
			}
		}

		for _, item := range v {
			switch val := item.(type) {
			case string:
				flushToolCalls()
				if val != "" {
					msgs = append(msgs, types.ChatMessage{Role: "user", Content: strPtr(val)})
				}
			case map[string]any:
				itemType, _ := val["type"].(string)
				switch itemType {
				case "function_call":
					callID, _ := val["call_id"].(string)
					name, _ := val["name"].(string)
					args, _ := val["arguments"].(string)
					pendingToolCalls = append(pendingToolCalls, types.ToolCall{
						ID:   callID,
						Type: "function",
						Function: types.FunctionCall{
							Name:      name,
							Arguments: args,
						},
					})
				case "function_call_output":
					flushToolCalls()
					callID, _ := val["call_id"].(string)
					output, _ := val["output"].(string)
					msgs = append(msgs, types.ChatMessage{
						Role:       "tool",
						ToolCallID: callID,
						Content:    strPtr(output),
					})
				default:
					flushToolCalls()
					role := NormalizeRole(val["role"].(string))
					text := extractInputTextMessage(val)
					if text != "" {
						msgs = append(msgs, types.ChatMessage{Role: role, Content: strPtr(text)})
					}
				}
			}
		}
		flushToolCalls()
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

// ChatToResponses converts Chat Completions API response back to OpenAI Responses API format
func (c *Converter) ChatToResponses(chatResp *types.ChatResponse, model, thinkTag string) (*types.ResponsesResponse, error) {
	now := time.Now().Unix()
	var responseItems []types.ResponseItem
	for _, choice := range chatResp.Choices {
		// Text message item
		text := StripThinkTag(derefStr(choice.Message.Content), thinkTag)
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
