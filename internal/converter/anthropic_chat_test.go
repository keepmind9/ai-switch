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

// --- ResponsesToAnthropic direct conversion tests ---

func TestResponsesToAnthropic_StringInput(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model:  "gpt-4o",
		Input:  "Hello",
		Stream: true,
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4o", anthReq.Model)
	assert.Equal(t, true, anthReq.Stream)
	assert.Nil(t, anthReq.System)
	require.Len(t, anthReq.Messages, 1)
	assert.Equal(t, "user", anthReq.Messages[0].Role)
	assert.Equal(t, "Hello", anthReq.Messages[0].Content)
}

func TestResponsesToAnthropic_WithInstructions(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model:        "gpt-4o",
		Input:        "Hi",
		Instructions: "Be concise.",
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	assert.Equal(t, "Be concise.", anthReq.System)
}

func TestResponsesToAnthropic_ArrayInput(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{"First", "Second"},
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	require.Len(t, anthReq.Messages, 2)
	assert.Equal(t, "First", anthReq.Messages[0].Content)
	assert.Equal(t, "Second", anthReq.Messages[1].Content)
}

func TestResponsesToAnthropic_NilInput(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	assert.Nil(t, anthReq.System)
	assert.Empty(t, anthReq.Messages)
}

func TestResponsesToAnthropic_DefaultMaxTokens(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model:     "gpt-4o",
		Input:     "Hi",
		MaxTokens: 0,
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	assert.Equal(t, 4096, anthReq.MaxTokens)
}

func TestResponsesToAnthropic_PreservesMaxTokens(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model:     "gpt-4o",
		Input:     "Hi",
		MaxTokens: 1024,
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	assert.Equal(t, 1024, anthReq.MaxTokens)
}

func TestResponsesToAnthropic_Parameters(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model:       "gpt-4o",
		Input:       "Hi",
		MaxTokens:   512,
		Temperature: 0.7,
		TopP:        0.9,
		Metadata:    map[string]any{"key": "value"},
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	assert.Equal(t, 512, anthReq.MaxTokens)
	assert.Equal(t, 0.7, anthReq.Temperature)
	assert.Equal(t, 0.9, anthReq.TopP)
	assert.Equal(t, map[string]any{"key": "value"}, anthReq.Metadata)
	assert.Nil(t, anthReq.Tools)
	assert.Nil(t, anthReq.ToolChoice)
}

// --- AnthropicResponseToResponses direct conversion tests ---

func TestAnthropicResponseToResponses_Basic(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:    "msg_123",
		Type:  "message",
		Role:  "assistant",
		Model: "claude-sonnet-4-5",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Hello world"},
		},
		StopReason: "end_turn",
		Usage:      AnthropicUsage{InputTokens: 10, OutputTokens: 5},
	}

	result, err := c.AnthropicResponseToResponses(resp, "claude-sonnet-4-5", "")
	require.NoError(t, err)

	assert.Equal(t, "msg_123", result.ID)
	assert.Equal(t, "response", result.Object)
	assert.Equal(t, "claude-sonnet-4-5", result.Model)
	require.Len(t, result.Responses, 1)
	assert.Equal(t, "assistant", result.Responses[0].Role)
	assert.Equal(t, "completed", result.Responses[0].Status)
	require.Len(t, result.Responses[0].Content, 1)
	assert.Equal(t, "output_text", result.Responses[0].Content[0].Type)
	assert.Equal(t, "Hello world", result.Responses[0].Content[0].Text)
	require.NotNil(t, result.Usage)
	assert.Equal(t, 10, result.Usage.InputTokens)
	assert.Equal(t, 5, result.Usage.OutputTokens)
	assert.Equal(t, 15, result.Usage.TotalTokens)
}

// --- Tool support tests for Responses↔Anthropic ---

func TestResponsesToAnthropic_FunctionCallInput(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{
			map[string]any{"type": "message", "role": "user", "content": "What's the weather?"},
			map[string]any{
				"type":      "function_call",
				"call_id":   "call_abc",
				"name":      "get_weather",
				"arguments": `{"location":"SF"}`,
			},
		},
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	require.Len(t, anthReq.Messages, 2)
	// First message: user text
	assert.Equal(t, "user", anthReq.Messages[0].Role)
	assert.Equal(t, "What's the weather?", anthReq.Messages[0].Content)

	// Second message: assistant tool_use
	assert.Equal(t, "assistant", anthReq.Messages[1].Role)
	blocks, ok := anthReq.Messages[1].Content.([]any)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	block, _ := blocks[0].(map[string]any)
	assert.Equal(t, "tool_use", block["type"])
	assert.Equal(t, "call_abc", block["id"])
	assert.Equal(t, "get_weather", block["name"])
}

func TestResponsesToAnthropic_FunctionCallOutputInput(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{
			map[string]any{
				"type":    "function_call_output",
				"call_id": "call_abc",
				"output":  "Sunny, 72F",
			},
		},
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	require.Len(t, anthReq.Messages, 1)
	assert.Equal(t, "user", anthReq.Messages[0].Role)
	blocks, ok := anthReq.Messages[0].Content.([]any)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	block, _ := blocks[0].(map[string]any)
	assert.Equal(t, "tool_result", block["type"])
	assert.Equal(t, "call_abc", block["tool_use_id"])
	assert.Equal(t, "Sunny, 72F", block["content"])
}

func TestResponsesToAnthropic_ToolsMapping(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: "Hi",
		Tools: []types.ResponsesTool{
			{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		},
	}

	anthReq, err := c.ResponsesToAnthropic(req)
	require.NoError(t, err)

	require.Len(t, anthReq.Tools, 1)
	assert.Equal(t, "get_weather", anthReq.Tools[0].Name)
	assert.Equal(t, "Get weather", anthReq.Tools[0].Description)
	assert.Contains(t, anthReq.Tools[0].InputSchema, "type")
}

func TestResponsesToAnthropic_ToolChoiceMapping(t *testing.T) {
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
			req := &types.ResponsesRequest{
				Model:      "gpt-4o",
				Input:      "Hi",
				ToolChoice: tt.choice,
			}
			anthReq, err := c.ResponsesToAnthropic(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, anthReq.ToolChoice)
		})
	}
}

func TestAnthropicResponseToResponses_ToolUse(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:   "msg_tool",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Let me check."},
			{Type: "tool_use", ID: "toolu_abc", Name: "get_weather", Input: map[string]any{"location": "SF"}},
		},
		StopReason: "tool_use",
		Usage:      AnthropicUsage{InputTokens: 100, OutputTokens: 50},
	}

	result, err := c.AnthropicResponseToResponses(resp, "model", "")
	require.NoError(t, err)

	require.Len(t, result.Responses, 2)

	// First: message item
	assert.Equal(t, "message", result.Responses[0].Type)
	assert.Equal(t, "assistant", result.Responses[0].Role)
	assert.Equal(t, "Let me check.", result.Responses[0].Content[0].Text)

	// Second: function_call item
	assert.Equal(t, "function_call", result.Responses[1].Type)
	assert.Equal(t, "fc_toolu_abc", result.Responses[1].ID)
	assert.Equal(t, "toolu_abc", result.Responses[1].CallID)
	assert.Equal(t, "get_weather", result.Responses[1].Name)
	assert.Contains(t, result.Responses[1].Arguments, "SF")
	assert.Equal(t, "completed", result.Responses[1].Status)
}

func TestAnthropicResponseToResponses_ToolUseOnly(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:   "msg_toolonly",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "tool_use", ID: "toolu_1", Name: "calc", Input: map[string]any{"x": 1}},
		},
		StopReason: "tool_use",
		Usage:      AnthropicUsage{InputTokens: 50, OutputTokens: 25},
	}

	result, err := c.AnthropicResponseToResponses(resp, "model", "")
	require.NoError(t, err)

	require.Len(t, result.Responses, 1)
	assert.Equal(t, "function_call", result.Responses[0].Type)
	assert.Equal(t, "calc", result.Responses[0].Name)
}

func TestAnthropicResponseToResponses_MultipleTextBlocks(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:   "msg_multi",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Hello "},
			{Type: "text", Text: "world"},
		},
		Usage: AnthropicUsage{InputTokens: 10, OutputTokens: 10},
	}

	result, err := c.AnthropicResponseToResponses(resp, "model", "")
	require.NoError(t, err)

	assert.Equal(t, "Hello world", result.Responses[0].Content[0].Text)
}

func TestAnthropicResponseToResponses_ThinkTag(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:   "msg_think",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "<think_>some reasoning</think_>The answer is 42"},
		},
		Usage: AnthropicUsage{InputTokens: 10, OutputTokens: 10},
	}

	result, err := c.AnthropicResponseToResponses(resp, "model", "think_")
	require.NoError(t, err)

	assert.Equal(t, "The answer is 42", result.Responses[0].Content[0].Text)
}

func TestAnthropicResponseToResponses_IgnoresNonTextBlocks(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:   "msg_tool",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Let me check."},
			{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: map[string]any{"location": "SF"}},
		},
		StopReason: "tool_use",
		Usage:      AnthropicUsage{InputTokens: 100, OutputTokens: 50},
	}

	result, err := c.AnthropicResponseToResponses(resp, "model", "")
	require.NoError(t, err)

	assert.Equal(t, "Let me check.", result.Responses[0].Content[0].Text)
}

func TestAnthropicResponseToResponses_UsageWithCache(t *testing.T) {
	c := NewConverter()
	resp := &AnthropicResponse{
		ID:   "msg_cache",
		Type: "message",
		Content: []AnthropicContentBlock{
			{Type: "text", Text: "Hi"},
		},
		Usage: AnthropicUsage{
			InputTokens:              100,
			OutputTokens:             50,
			CacheCreationInputTokens: 200,
			CacheReadInputTokens:     300,
		},
	}

	result, err := c.AnthropicResponseToResponses(resp, "model", "")
	require.NoError(t, err)

	assert.Equal(t, 100, result.Usage.InputTokens)
	assert.Equal(t, 50, result.Usage.OutputTokens)
	assert.Equal(t, 150, result.Usage.TotalTokens)
}

func TestAnthropicToResponses_SimpleMessage(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model:    "claude-3",
		Messages: []AnthropicMessage{{Role: "user", Content: "Hello"}},
	}

	respReq, err := c.AnthropicToResponses(req)
	require.NoError(t, err)
	assert.Equal(t, "Hello", respReq.Input)
	assert.Equal(t, "claude-3", respReq.Model)
}

func TestAnthropicToResponses_WithSystem(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model:    "claude-3",
		System:   "You are helpful",
		Messages: []AnthropicMessage{{Role: "user", Content: "Hi"}},
	}

	respReq, err := c.AnthropicToResponses(req)
	require.NoError(t, err)
	assert.Equal(t, "You are helpful", respReq.Instructions)
}

func TestAnthropicToResponses_Tools(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model:    "claude-3",
		Messages: []AnthropicMessage{{Role: "user", Content: "Weather?"}},
		Tools: []AnthropicTool{
			{Name: "get_weather", Description: "Get weather", InputSchema: map[string]any{"type": "object"}},
		},
		ToolChoice: map[string]any{"type": "auto"},
	}

	respReq, err := c.AnthropicToResponses(req)
	require.NoError(t, err)
	require.Len(t, respReq.Tools, 1)
	assert.Equal(t, "function", respReq.Tools[0].Type)
	assert.Equal(t, "get_weather", respReq.Tools[0].Name)
	assert.Equal(t, "auto", respReq.ToolChoice)
}

func TestAnthropicToResponses_ToolNilSchema(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "claude-3",
		Tools: []AnthropicTool{{Name: "search"}},
	}

	respReq, err := c.AnthropicToResponses(req)
	require.NoError(t, err)
	require.Len(t, respReq.Tools, 1)
	assert.Equal(t, map[string]any{"type": "object"}, respReq.Tools[0].Parameters)
}

func TestAnthropicToResponses_ToolUseMessages(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "claude-3",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Weather?"},
			{Role: "assistant", Content: []any{
				map[string]any{"type": "tool_use", "id": "call_1", "name": "get_weather", "input": map[string]any{"city": "NYC"}},
			}},
			{Role: "user", Content: []any{
				map[string]any{"type": "tool_result", "tool_use_id": "call_1", "content": `{"temp":72}`},
			}},
		},
	}

	respReq, err := c.AnthropicToResponses(req)
	require.NoError(t, err)
	arr, ok := respReq.Input.([]any)
	require.True(t, ok)
	require.Len(t, arr, 3)

	assert.Equal(t, "Weather?", arr[0])

	fc, ok := arr[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "function_call", fc["type"])
	assert.Equal(t, "call_1", fc["call_id"])
	assert.Equal(t, "get_weather", fc["name"])

	fco, ok := arr[2].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "function_call_output", fco["type"])
	assert.Equal(t, "call_1", fco["call_id"])
	assert.Equal(t, `{"temp":72}`, fco["output"])
}

func TestAnthropicToResponses_ToolChoiceMapping(t *testing.T) {
	tests := []struct {
		name   string
		choice any
		want   any
	}{
		{"auto", map[string]any{"type": "auto"}, "auto"},
		{"any_to_required", map[string]any{"type": "any"}, "required"},
		{"none", map[string]any{"type": "none"}, map[string]any{"type": "none"}},
		{"tool", map[string]any{"type": "tool", "name": "search"}, map[string]any{"type": "function", "function": map[string]any{"name": "search"}}},
		{"nil", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewConverter()
			req := &AnthropicRequest{
				Model:      "claude-3",
				Messages:   []AnthropicMessage{{Role: "user", Content: "test"}},
				ToolChoice: tt.choice,
			}
			respReq, err := c.AnthropicToResponses(req)
			require.NoError(t, err)
			assert.Equal(t, tt.want, respReq.ToolChoice)
		})
	}
}

func TestAnthropicToResponses_MultiTurnMessages(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
			{Role: "user", Content: "Bye"},
		},
	}

	respReq, err := c.AnthropicToResponses(req)
	require.NoError(t, err)
	arr, ok := respReq.Input.([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"Hello", "Hi", "Bye"}, arr)
}

func TestAnthropicToChat_MultiTurnToolUse(t *testing.T) {
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "test",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "What's the weather?"},
			{
				Role: "assistant",
				Content: []any{
					map[string]any{"type": "thinking", "thinking": "I need to check the weather..."},
					map[string]any{
						"type":  "tool_use",
						"id":    "call_abc",
						"name":  "get_weather",
						"input": map[string]any{"city": "NYC"},
					},
				},
			},
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": "call_abc",
						"content":     "72°F sunny",
					},
				},
			},
			{
				Role: "assistant",
				Content: []any{
					map[string]any{"type": "thinking", "thinking": "Good weather..."},
					map[string]any{"type": "text", "text": "The weather is 72°F."},
				},
			},
			{Role: "user", Content: "What about London?"},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	// Verify message sequence
	msgs := chatReq.Messages
	require.Len(t, msgs, 5)

	// 1. user: "What's the weather?"
	assert.Equal(t, "user", msgs[0].Role)
	assert.Equal(t, "What's the weather?", msgs[0].Content)

	// 2. assistant with reasoning_content + tool_calls
	assert.Equal(t, "assistant", msgs[1].Role)
	require.NotNil(t, msgs[1].ReasoningContent)
	assert.Equal(t, "I need to check the weather...", *msgs[1].ReasoningContent)
	require.Len(t, msgs[1].ToolCalls, 1)
	assert.Equal(t, "call_abc", msgs[1].ToolCalls[0].ID)
	assert.Equal(t, "get_weather", msgs[1].ToolCalls[0].Function.Name)

	// 3. tool result (MUST immediately follow assistant with tool_calls)
	assert.Equal(t, "tool", msgs[2].Role)
	assert.Equal(t, "call_abc", msgs[2].ToolCallID)
	assert.Equal(t, "72°F sunny", msgs[2].Content)

	// 4. assistant with reasoning_content + text
	assert.Equal(t, "assistant", msgs[3].Role)
	require.NotNil(t, msgs[3].ReasoningContent)
	assert.Equal(t, "Good weather...", *msgs[3].ReasoningContent)
	assert.Equal(t, "The weather is 72°F.", msgs[3].Content)

	// 5. user follow-up
	assert.Equal(t, "user", msgs[4].Role)
	assert.Equal(t, "What about London?", msgs[4].Content)

	// Verify JSON serialization preserves reasoning_content on assistant messages
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		require.NoError(t, err)
		if msg.Role == "assistant" {
			assert.Contains(t, string(data), "reasoning_content",
				"assistant message must include reasoning_content: %s", string(data))
		}
	}
}

func TestAnthropicToChat_ToolResultWithTextOrdering(t *testing.T) {
	// When a user message has both tool_result and text blocks,
	// tool messages must come BEFORE user text to satisfy Chat API ordering.
	c := NewConverter()
	req := &AnthropicRequest{
		Model: "test",
		Messages: []AnthropicMessage{
			{Role: "user", Content: "Check weather"},
			{
				Role: "assistant",
				Content: []any{
					map[string]any{"type": "tool_use", "id": "call_1", "name": "get_weather", "input": map[string]any{}},
				},
			},
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "tool_result", "tool_use_id": "call_1", "content": "72°F"},
					map[string]any{"type": "text", "text": "Also tell me about NYC"},
				},
			},
		},
	}

	chatReq, err := c.AnthropicToChat(req)
	require.NoError(t, err)

	// assistant with tool_calls, then tool message, then user text
	require.Len(t, chatReq.Messages, 4)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "assistant", chatReq.Messages[1].Role)
	assert.Len(t, chatReq.Messages[1].ToolCalls, 1)

	// Tool message MUST come before user text
	assert.Equal(t, "tool", chatReq.Messages[2].Role)
	assert.Equal(t, "call_1", chatReq.Messages[2].ToolCallID)

	assert.Equal(t, "user", chatReq.Messages[3].Role)
	assert.Equal(t, "Also tell me about NYC", chatReq.Messages[3].Content)
}
