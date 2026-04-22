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

	chatReq, err := c.ResponsesToChat(&req)
	if err != nil {
		return nil, err
	}

	switch upstreamFormat {
	case FormatChat, "":
		chatReq.Model = resolveModel(defaultModel)
		chatBody, err := json.Marshal(chatReq)
		if err != nil {
			return nil, err
		}
		return &ConvertedRequest{UpstreamBody: chatBody, Model: model, IsStreaming: req.Stream}, nil

	case FormatAnthropic:
		anthReq, err := c.ChatRequestToAnthropic(chatReq)
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
		chatReq, err := c.AnthropicToChat(&req)
		if err != nil {
			return nil, err
		}
		chatReq.Model = resolveModel(model)
		respReq := BuildResponsesFromChat(chatReq, req.Stream)
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
	var input any
	for _, msg := range chatReq.Messages {
		if msg.Role == "system" {
			instructions += msg.Content + "\n"
		} else {
			input = msg.Content
		}
	}
	return &types.ResponsesRequest{
		Model:        chatReq.Model,
		Input:        input,
		Instructions: strings.TrimSpace(instructions),
		Stream:       stream,
		MaxTokens:    chatReq.MaxTokens,
		Temperature:  chatReq.Temperature,
		TopP:         chatReq.TopP,
	}
}

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
func (c *Converter) ChatToResponses(chatResp *types.ChatResponse, model, thinkTag string) (*types.ResponsesResponse, error) {
	now := time.Now().Unix()
	var responseItems []types.ResponseItem
	for _, choice := range chatResp.Choices {
		responseItems = append(responseItems, types.ResponseItem{
			ID:      fmt.Sprintf("resp_%d", now),
			Object:  "response",
			Created: now,
			Role:    "assistant",
			Content: []types.ContentBlock{
				{Type: "output_text", Text: StripThinkTag(choice.Message.Content, thinkTag)},
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
