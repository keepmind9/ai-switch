package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- AnthropicToChat ---

func TestAnthropicToChat_SimpleMessages(t *testing.T) {
	c := NewConverter()

	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	assert.Equal(t, "claude-sonnet-4-5", chatReq.Model)
	assert.Equal(t, 1024, chatReq.MaxTokens)
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "Hello", chatReq.Messages[0].Content)
}

func TestAnthropicToChat_WithSystemString(t *testing.T) {
	c := NewConverter()

	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 2048,
		System:    "You are a helpful assistant.",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hi"},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "You are a helpful assistant.", chatReq.Messages[0].Content)
	assert.Equal(t, "user", chatReq.Messages[1].Role)
}

func TestAnthropicToChat_WithSystemBlocks(t *testing.T) {
	c := NewConverter()

	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 2048,
		System: []any{
			map[string]any{"type": "text", "text": "Part one."},
			map[string]any{"type": "text", "text": "Part two."},
		},
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hi"},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "Part one.\nPart two.", chatReq.Messages[0].Content)
}

func TestAnthropicToChat_MultiTurn(t *testing.T) {
	c := NewConverter()

	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{Role: "user", Content: "What is 2+2?"},
			{Role: "assistant", Content: "2+2 equals 4."},
			{Role: "user", Content: "And 3+3?"},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 3)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	assert.Equal(t, "user", chatReq.Messages[2].Role)
}

func TestAnthropicToChat_ContentBlocks(t *testing.T) {
	c := NewConverter()

	req := &AnthropicRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1024,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "First block"},
					map[string]any{"type": "text", "text": "Second block"},
				},
			},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "First block\nSecond block", chatReq.Messages[0].Content)
}

func TestAnthropicToChat_Parameters(t *testing.T) {
	c := NewConverter()

	req := &AnthropicRequest{
		Model:       "claude-sonnet-4-5",
		MaxTokens:   512,
		Temperature: 0.5,
		TopP:        0.8,
		Stream:      true,
		Messages:    []AnthropicMessage{{Role: "user", Content: "test"}},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	assert.Equal(t, 512, chatReq.MaxTokens)
	assert.InDelta(t, 0.5, chatReq.Temperature, 0.001)
	assert.InDelta(t, 0.8, chatReq.TopP, 0.001)
	assert.True(t, chatReq.Stream)
}

// --- ChatToAnthropic ---

func TestChatToAnthropic_Basic(t *testing.T) {
	c := NewConverter()

	chatResp := &types.ChatResponse{
		ID:    "chatcmpl-abc",
		Model: "upstream-model",
		Choices: []types.ChatChoice{
			{
				Index:        0,
				Message:      types.ChatMessage{Role: "assistant", Content: "Hello from assistant"},
				FinishReason: "stop",
			},
		},
		Usage: types.ChatUsage{
			PromptTokens:     20,
			CompletionTokens: 10,
			TotalTokens:      30,
		},
	}

	resp, err := c.ChatToAnthropic(chatResp, "claude-sonnet-4-5", "")
	require.NoError(t, err)

	assert.Equal(t, "chatcmpl-abc", resp.ID)
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "assistant", resp.Role)
	assert.Equal(t, "claude-sonnet-4-5", resp.Model)
	assert.Equal(t, "end_turn", resp.StopReason)

	require.Len(t, resp.Content, 1)
	assert.Equal(t, "text", resp.Content[0].Type)
	assert.Equal(t, "Hello from assistant", resp.Content[0].Text)

	assert.Equal(t, 20, resp.Usage.InputTokens)
	assert.Equal(t, 10, resp.Usage.OutputTokens)
}

func TestChatToAnthropic_StopReasonMapping(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		expectedStop string
	}{
		{"stop to end_turn", "stop", "end_turn"},
		{"length to max_tokens", "length", "max_tokens"},
		{"tool_calls to tool_use", "tool_calls", "tool_use"},
		{"empty passthrough", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewConverter()
			chatResp := &types.ChatResponse{
				Choices: []types.ChatChoice{
					{FinishReason: tt.finishReason, Message: types.ChatMessage{Content: "text"}},
				},
			}

			resp, err := c.ChatToAnthropic(chatResp, "model", "")
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStop, resp.StopReason)
		})
	}
}

func TestChatToAnthropic_EmptyChoices(t *testing.T) {
	c := NewConverter()

	chatResp := &types.ChatResponse{
		ID:      "chatcmpl-empty",
		Choices: []types.ChatChoice{},
	}

	resp, err := c.ChatToAnthropic(chatResp, "model", "")
	require.NoError(t, err)

	assert.Empty(t, resp.Content)
	assert.Empty(t, resp.StopReason)
}

// --- ChatRequestToAnthropic ---

func TestChatRequestToAnthropic_Basic(t *testing.T) {
	c := NewConverter()

	chatReq := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens:   2048,
		Temperature: 0.7,
	}

	anthReq, err := c.ChatRequestToAnthropic(chatReq)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4o", anthReq.Model)
	assert.Equal(t, 2048, anthReq.MaxTokens)
	assert.InDelta(t, 0.7, anthReq.Temperature, 0.001)
	require.Len(t, anthReq.Messages, 1)
	assert.Equal(t, "user", anthReq.Messages[0].Role)
	assert.Equal(t, "Hello", anthReq.Messages[0].Content)
}

func TestChatRequestToAnthropic_SystemExtraction(t *testing.T) {
	c := NewConverter()

	chatReq := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello!"},
			{Role: "user", Content: "How are you?"},
		},
	}

	anthReq, err := c.ChatRequestToAnthropic(chatReq)
	require.NoError(t, err)

	assert.Equal(t, "Be helpful", anthReq.System)
	require.Len(t, anthReq.Messages, 3)
	assert.Equal(t, "user", anthReq.Messages[0].Role)
	assert.Equal(t, "assistant", anthReq.Messages[1].Role)
	assert.Equal(t, "user", anthReq.Messages[2].Role)
}

func TestChatRequestToAnthropic_DefaultMaxTokens(t *testing.T) {
	c := NewConverter()

	chatReq := &types.ChatRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: "test"}},
	}

	anthReq, err := c.ChatRequestToAnthropic(chatReq)
	require.NoError(t, err)

	assert.Equal(t, 4096, anthReq.MaxTokens)
}

func TestChatRequestToAnthropic_PreservesMaxTokens(t *testing.T) {
	c := NewConverter()

	chatReq := &types.ChatRequest{
		Model:     "gpt-4o",
		Messages:  []types.ChatMessage{{Role: "user", Content: "test"}},
		MaxTokens: 8192,
	}

	anthReq, err := c.ChatRequestToAnthropic(chatReq)
	require.NoError(t, err)

	assert.Equal(t, 8192, anthReq.MaxTokens)
}

// --- AnthropicResponseToChat ---

func TestAnthropicResponseToChat_Basic(t *testing.T) {
	c := NewConverter()

	anthResp := &AnthropicResponse{
		ID:   "msg_123",
		Type: "message",
		Role: "assistant",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Hello! I can help."},
		},
		Model:      "claude-sonnet-4-5",
		StopReason: "end_turn",
		Usage: AnthropicUsage{
			InputTokens:  50,
			OutputTokens: 25,
		},
	}

	chatResp, err := c.AnthropicResponseToChat(anthResp)
	require.NoError(t, err)

	assert.Equal(t, "msg_123", chatResp.ID)
	assert.Equal(t, "chat.completion", chatResp.Object)
	assert.Equal(t, "claude-sonnet-4-5", chatResp.Model)

	require.Len(t, chatResp.Choices, 1)
	assert.Equal(t, 0, chatResp.Choices[0].Index)
	assert.Equal(t, "assistant", chatResp.Choices[0].Message.Role)
	assert.Equal(t, "Hello! I can help.", chatResp.Choices[0].Message.Content)
	assert.Equal(t, "stop", chatResp.Choices[0].FinishReason)

	assert.Equal(t, 50, chatResp.Usage.PromptTokens)
	assert.Equal(t, 25, chatResp.Usage.CompletionTokens)
	assert.Equal(t, 75, chatResp.Usage.TotalTokens)
}

func TestAnthropicResponseToChat_StopReasonMapping(t *testing.T) {
	tests := []struct {
		name       string
		stopReason string
		expected   string
	}{
		{"end_turn to stop", "end_turn", "stop"},
		{"max_tokens to length", "max_tokens", "length"},
		{"tool_use to tool_calls", "tool_use", "tool_calls"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewConverter()
			anthResp := &AnthropicResponse{
				Content:    []AnthropicContentBlock{{Type: "text", Text: "ok"}},
				StopReason: tt.stopReason,
			}

			chatResp, err := c.AnthropicResponseToChat(anthResp)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, chatResp.Choices[0].FinishReason)
		})
	}
}

func TestAnthropicResponseToChat_MultipleContentBlocks(t *testing.T) {
	c := NewConverter()

	anthResp := &AnthropicResponse{
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Part one. "},
			{Type: "text", Text: "Part two."},
			{Type: "thinking", Text: "should be skipped"},
		},
		StopReason: "end_turn",
	}

	chatResp, err := c.AnthropicResponseToChat(anthResp)
	require.NoError(t, err)

	assert.Equal(t, "Part one. Part two.", chatResp.Choices[0].Message.Content)
}

func TestAnthropicResponseToChat_EmptyContent(t *testing.T) {
	c := NewConverter()

	anthResp := &AnthropicResponse{
		Content:    []AnthropicContentBlock{},
		StopReason: "end_turn",
	}

	chatResp, err := c.AnthropicResponseToChat(anthResp)
	require.NoError(t, err)
	assert.Empty(t, chatResp.Choices[0].Message.Content)
}

func TestAnthropicResponseToChat_UsageWithCache(t *testing.T) {
	c := NewConverter()

	anthResp := &AnthropicResponse{
		Content:    []AnthropicContentBlock{{Type: "text", Text: "ok"}},
		StopReason: "end_turn",
		Usage: AnthropicUsage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheCreationInputTokens: 30,
			CacheReadInputTokens:     70,
		},
	}

	chatResp, err := c.AnthropicResponseToChat(anthResp)
	require.NoError(t, err)

	assert.Equal(t, 100, chatResp.Usage.PromptTokens)
	assert.Equal(t, 50, chatResp.Usage.CompletionTokens)
	assert.Equal(t, 150, chatResp.Usage.TotalTokens)
}

// --- Round-trip tests ---

func TestRoundTrip_AnthropicToChatAndBack(t *testing.T) {
	c := NewConverter()

	origReq := &AnthropicRequest{
		Model:       "claude-sonnet-4-5",
		MaxTokens:   1024,
		Temperature: 0.7,
		TopP:        0.9,
		System:      "You are helpful.",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!"},
			{Role: "user", Content: "How are you?"},
		},
	}

	// Anthropic → Chat
	_, err := c.AnthropicToChat(origReq)
	require.NoError(t, err)

	// Simulate upstream Chat response
	chatResp := &types.ChatResponse{
		ID:    "chatcmpl-rt",
		Model: "upstream-model",
		Choices: []types.ChatChoice{
			{
				Index:        0,
				Message:      types.ChatMessage{Role: "assistant", Content: "I'm doing well, thanks!"},
				FinishReason: "stop",
			},
		},
		Usage: types.ChatUsage{PromptTokens: 30, CompletionTokens: 15, TotalTokens: 45},
	}

	// Chat → Anthropic
	anthResp, err := c.ChatToAnthropic(chatResp, "claude-sonnet-4-5", "")
	require.NoError(t, err)

	assert.Equal(t, "message", anthResp.Type)
	assert.Equal(t, "assistant", anthResp.Role)
	assert.Equal(t, "end_turn", anthResp.StopReason)
	assert.Equal(t, "I'm doing well, thanks!", anthResp.Content[0].Text)
}

func TestRoundTrip_ChatToAnthropicAndBack(t *testing.T) {
	c := NewConverter()

	origReq := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "system", Content: "Be concise."},
			{Role: "user", Content: "What is 2+2?"},
		},
		MaxTokens: 512,
	}

	// Chat → Anthropic
	anthReq, err := c.ChatRequestToAnthropic(origReq)
	require.NoError(t, err)

	assert.Equal(t, "Be concise.", anthReq.System)
	require.Len(t, anthReq.Messages, 1)
	assert.Equal(t, "user", anthReq.Messages[0].Role)

	// Simulate Anthropic response
	anthResp := &AnthropicResponse{
		ID:   "msg_rt",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "2+2 equals 4."},
		},
		StopReason: "end_turn",
		Usage:      AnthropicUsage{InputTokens: 15, OutputTokens: 8},
	}

	// Anthropic → Chat
	chatResp, err := c.AnthropicResponseToChat(anthResp)
	require.NoError(t, err)

	assert.Equal(t, "stop", chatResp.Choices[0].FinishReason)
	assert.Equal(t, "2+2 equals 4.", chatResp.Choices[0].Message.Content)
	assert.Equal(t, 23, chatResp.Usage.TotalTokens)
}

// --- Tool use conversion tests ---

func TestAnthropicToChat_ToolDefinitions(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "claude-sonnet-4-5",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "What's the weather?"},
		},
		Tools: []AnthropicTool{
			{
				Name:        "get_weather",
				Description: "Get current weather",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Tools, 1)
	assert.Equal(t, "function", chatReq.Tools[0].Type)
	assert.Equal(t, "get_weather", chatReq.Tools[0].Function.Name)
	assert.Equal(t, "Get current weather", chatReq.Tools[0].Function.Description)
	assert.Contains(t, chatReq.Tools[0].Function.Parameters, "type")
}

func TestAnthropicToChat_ToolChoice(t *testing.T) {
	tests := []struct {
		name     string
		choice   any
		expected any
	}{
		{"auto", map[string]any{"type": "auto"}, "auto"},
		{"any to required", map[string]any{"type": "any"}, "required"},
		{"tool", map[string]any{"type": "tool", "name": "get_weather"}, map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}},
		{"nil", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewConverter()
			req := &AnthropicRequest{
				Model:      "test",
				Messages:   []AnthropicMessage{{Role: "user", Content: "hi"}},
				ToolChoice: tt.choice,
			}
			chatReq, err := c.AnthropicToChat(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, chatReq.ToolChoice)
		})
	}
}

func TestAnthropicToChat_ToolUseMessages(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "test",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "What's the weather?"},
			{
				Role: "assistant",
				Content: []any{
					map[string]any{"type": "text", "text": "Let me check."},
					map[string]any{
						"type":  "tool_use",
						"id":    "toolu_123",
						"name":  "get_weather",
						"input": map[string]any{"location": "SF"},
					},
				},
			},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	// user message + assistant with tool_calls
	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	assert.Equal(t, "Let me check.", chatReq.Messages[1].Content)
	require.Len(t, chatReq.Messages[1].ToolCalls, 1)
	assert.Equal(t, "toolu_123", chatReq.Messages[1].ToolCalls[0].ID)
	assert.Equal(t, "function", chatReq.Messages[1].ToolCalls[0].Type)
	assert.Equal(t, "get_weather", chatReq.Messages[1].ToolCalls[0].Function.Name)
	assert.Contains(t, chatReq.Messages[1].ToolCalls[0].Function.Arguments, "SF")
}

func TestAnthropicToChat_ToolResultMessages(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "test",
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_123",
						"content":     "Sunny, 72F",
					},
				},
			},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "tool", chatReq.Messages[0].Role)
	assert.Equal(t, "toolu_123", chatReq.Messages[0].ToolCallID)
	assert.Equal(t, "Sunny, 72F", chatReq.Messages[0].Content)
}

func TestChatRequestToAnthropic_ToolDefinitions(t *testing.T) {
	c := NewConverter()
	req := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "hi"},
		},
		Tools: []types.Tool{
			{
				Type: "function",
				Function: types.FunctionDef{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	}

	anthReq, err := c.ChatRequestToAnthropic(req)
	require.NoError(t, err)

	require.Len(t, anthReq.Tools, 1)
	assert.Equal(t, "get_weather", anthReq.Tools[0].Name)
	assert.Equal(t, "Get weather", anthReq.Tools[0].Description)
	assert.Contains(t, anthReq.Tools[0].InputSchema, "type")
}

func TestChatRequestToAnthropic_ToolChoice(t *testing.T) {
	tests := []struct {
		name     string
		choice   any
		expected any
	}{
		{"auto", "auto", map[string]any{"type": "auto"}},
		{"required", "required", map[string]any{"type": "any"}},
		{"none", "none", nil},
		{"function", map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}, map[string]any{"type": "tool", "name": "get_weather"}},
		{"nil", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewConverter()
			req := &types.ChatRequest{
				Model:      "test",
				Messages:   []types.ChatMessage{{Role: "user", Content: "hi"}},
				ToolChoice: tt.choice,
			}
			anthReq, err := c.ChatRequestToAnthropic(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, anthReq.ToolChoice)
		})
	}
}

func TestChatRequestToAnthropic_ToolCallMessages(t *testing.T) {
	c := NewConverter()
	args, _ := json.Marshal(map[string]any{"location": "SF"})

	req := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "user", Content: "weather?"},
			{
				Role: "assistant",
				ToolCalls: []types.ToolCall{
					{ID: "call_abc", Type: "function", Function: types.FunctionCall{Name: "get_weather", Arguments: string(args)}},
				},
			},
			{Role: "tool", ToolCallID: "call_abc", Content: "Sunny"},
		},
	}

	anthReq, err := c.ChatRequestToAnthropic(req)
	require.NoError(t, err)

	// user + assistant(tool_use) + user(tool_result)
	require.Len(t, anthReq.Messages, 3)

	// assistant message has tool_use content block
	assistantMsg := anthReq.Messages[1]
	assert.Equal(t, "assistant", assistantMsg.Role)
	blocks, ok := assistantMsg.Content.([]any)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	block, _ := blocks[0].(map[string]any)
	assert.Equal(t, "tool_use", block["type"])
	assert.Equal(t, "call_abc", block["id"])
	assert.Equal(t, "get_weather", block["name"])

	// tool result message
	toolMsg := anthReq.Messages[2]
	assert.Equal(t, "user", toolMsg.Role)
	toolBlocks, ok := toolMsg.Content.([]any)
	require.True(t, ok)
	require.Len(t, toolBlocks, 1)
	toolBlock, _ := toolBlocks[0].(map[string]any)
	assert.Equal(t, "tool_result", toolBlock["type"])
	assert.Equal(t, "call_abc", toolBlock["tool_use_id"])
	assert.Equal(t, "Sunny", toolBlock["content"])
}

func TestAnthropicResponseToChat_ToolUse(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:   "msg_tool",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Let me check."},
			{
				Type:  "tool_use",
				ID:    "toolu_abc",
				Name:  "get_weather",
				Input: map[string]any{"location": "SF"},
			},
		},
		StopReason: "tool_use",
		Usage:      AnthropicUsage{InputTokens: 100, OutputTokens: 50},
	}

	chatResp, err := c.AnthropicResponseToChat(resp)
	require.NoError(t, err)

	assert.Equal(t, "Let me check.", chatResp.Choices[0].Message.Content)
	require.Len(t, chatResp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "toolu_abc", chatResp.Choices[0].Message.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", chatResp.Choices[0].Message.ToolCalls[0].Function.Name)
	assert.Contains(t, chatResp.Choices[0].Message.ToolCalls[0].Function.Arguments, "SF")
	assert.Equal(t, "tool_calls", chatResp.Choices[0].FinishReason)
}

func TestChatToAnthropic_ToolCalls(t *testing.T) {
	c := NewConverter()
	args, _ := json.Marshal(map[string]any{"location": "SF"})

	chatResp := &types.ChatResponse{
		ID: "chatcmpl-tool",
		Choices: []types.ChatChoice{
			{
				Index: 0,
				Message: types.ChatMessage{
					Role:    "assistant",
					Content: "Let me check.",
					ToolCalls: []types.ToolCall{
						{ID: "call_abc", Type: "function", Function: types.FunctionCall{Name: "get_weather", Arguments: string(args)}},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: types.ChatUsage{PromptTokens: 50, CompletionTokens: 25},
	}

	resp, err := c.ChatToAnthropic(chatResp, "model", "")
	require.NoError(t, err)

	assert.Equal(t, "tool_use", resp.StopReason)
	require.Len(t, resp.Content, 2)
	assert.Equal(t, "text", resp.Content[0].Type)
	assert.Equal(t, "Let me check.", resp.Content[0].Text)
	assert.Equal(t, "tool_use", resp.Content[1].Type)
	assert.Equal(t, "call_abc", resp.Content[1].ID)
	assert.Equal(t, "get_weather", resp.Content[1].Name)
}

func TestChatToAnthropic_ToolCallsOnly(t *testing.T) {
	c := NewConverter()
	args, _ := json.Marshal(map[string]any{"x": 1})

	chatResp := &types.ChatResponse{
		Choices: []types.ChatChoice{
			{
				Message: types.ChatMessage{
					Role: "assistant",
					ToolCalls: []types.ToolCall{
						{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "calc", Arguments: string(args)}},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	resp, err := c.ChatToAnthropic(chatResp, "model", "")
	require.NoError(t, err)

	assert.Equal(t, "tool_use", resp.StopReason)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, "tool_use", resp.Content[0].Type)
}

func TestAnthropicToChat_ToolResultArrayContent(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "test",
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "toolu_1",
						"content": []any{
							map[string]any{"type": "text", "text": "Result line 1"},
							map[string]any{"type": "text", "text": "Result line 2"},
						},
					},
				},
			},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "tool", chatReq.Messages[0].Role)
	assert.Equal(t, "Result line 1\nResult line 2", chatReq.Messages[0].Content)
}
