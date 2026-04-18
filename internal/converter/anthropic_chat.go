package converter

import (
	"fmt"
	"strings"

	"github.com/keepmind9/llm-gateway/internal/types"
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
}

type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []AnthropicContentBlock
}

type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
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
		text := extractContentText(msg.Content)
		chatReq.Messages = append(chatReq.Messages, types.ChatMessage{
			Role:    msg.Role,
			Content: text,
		})
	}

	return chatReq, nil
}

// ChatToAnthropic converts a Chat Completions response to an Anthropic Messages response.
func (c *Converter) ChatToAnthropic(chatResp *types.ChatResponse, model string) (*AnthropicResponse, error) {
	var content []AnthropicContentBlock
	var stopReason string

	if len(chatResp.Choices) > 0 {
		choice := chatResp.Choices[0]
		content = append(content, AnthropicContentBlock{
			Type: "text",
			Text: choice.Message.Content,
		})
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

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			anthReq.System = msg.Content
			continue
		}
		anthReq.Messages = append(anthReq.Messages, AnthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	return anthReq, nil
}

// AnthropicResponseToChat converts an Anthropic Messages response to a Chat Completions response.
func (c *Converter) AnthropicResponseToChat(resp *AnthropicResponse) (*types.ChatResponse, error) {
	var contentText string
	if len(resp.Content) > 0 {
		var parts []string
		for _, block := range resp.Content {
			if block.Type == "text" {
				parts = append(parts, block.Text)
			}
		}
		contentText = strings.Join(parts, "")
	}

	return &types.ChatResponse{
		ID:     resp.ID,
		Object: "chat.completion",
		Model:  resp.Model,
		Choices: []types.ChatChoice{
			{
				Index:        0,
				Message:      types.ChatMessage{Role: "assistant", Content: contentText},
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

func chatStopToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
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
	default:
		return reason
	}
}
