package converter

import (
	"fmt"
	"strings"
	"time"

	"github.com/keepmind9/llm-gateway/internal/types"
)

type Converter struct{}

func NewConverter() *Converter {
	return &Converter{}
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

	// Handle input - can be string, array of strings, or array of message objects
	promptText := extractInputText(req.Input)

	// Add instructions as system prompt if present
	var systemPrompt string
	if req.Instructions != "" {
		systemPrompt = req.Instructions
	}

	// Build messages
	if systemPrompt != "" {
		chatReq.Messages = append(chatReq.Messages, types.ChatMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	if promptText != "" {
		chatReq.Messages = append(chatReq.Messages, types.ChatMessage{
			Role:    "user",
			Content: promptText,
		})
	}

	return chatReq, nil
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
func (c *Converter) ChatToResponses(chatResp *types.ChatResponse, model string) (*types.ResponsesResponse, error) {
	now := time.Now().Unix()
	var responseItems []types.ResponseItem
	for _, choice := range chatResp.Choices {
		responseItems = append(responseItems, types.ResponseItem{
			ID:      fmt.Sprintf("resp_%d", now),
			Object:  "response",
			Created: now,
			Role:    "assistant",
			Content: []types.ContentBlock{
				{Type: "output_text", Text: choice.Message.Content},
			},
			Status: "completed",
		})
	}

	resp := &types.ResponsesResponse{
		ID:        chatResp.ID,
		Object:    "list",
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
