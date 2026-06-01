package converter

import (
	"fmt"
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesToChat_SimpleInput(t *testing.T) {
	c := NewConverter()

	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: "Hello, how are you?",
	}

	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4o", chatReq.Model)
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "Hello, how are you?", derefStr(chatReq.Messages[0].Content))
}

func TestResponsesToChat_WithInstructions(t *testing.T) {
	c := NewConverter()

	req := &types.ResponsesRequest{
		Model:        "gpt-4o",
		Input:        "What is 2+2?",
		Instructions: "You are a helpful math tutor.",
	}

	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "You are a helpful math tutor.", derefStr(chatReq.Messages[0].Content))
	assert.Equal(t, "user", chatReq.Messages[1].Role)
	assert.Equal(t, "What is 2+2?", derefStr(chatReq.Messages[1].Content))
}

func TestResponsesToChat_ArrayInput(t *testing.T) {
	c := NewConverter()

	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{"First part", "Second part"},
	}

	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "First part", derefStr(chatReq.Messages[0].Content))
	assert.Equal(t, "Second part", derefStr(chatReq.Messages[1].Content))
}

func TestResponsesToChat_MessageArrayInput(t *testing.T) {
	c := NewConverter()

	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "Hello from Codex"},
				},
			},
		},
	}

	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "Hello from Codex", derefStr(chatReq.Messages[0].Content))
}

func TestResponsesToChat_NilInput(t *testing.T) {
	c := NewConverter()

	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: nil,
	}

	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)
	assert.Empty(t, chatReq.Messages)
}

func TestResponsesToChat_Parameters(t *testing.T) {
	c := NewConverter()

	req := &types.ResponsesRequest{
		Model:       "gpt-4o",
		Input:       "test",
		MaxTokens:   100,
		Temperature: 0.7,
		TopP:        0.9,
		Stream:      true,
	}

	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	assert.Equal(t, 100, chatReq.MaxTokens)
	assert.Equal(t, 0.7, chatReq.Temperature)
	assert.Equal(t, 0.9, chatReq.TopP)
	assert.True(t, chatReq.Stream)
}

func TestChatToResponses(t *testing.T) {
	c := NewConverter()

	chatResp := &types.ChatResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4o",
		Choices: []types.ChatChoice{
			{
				Index:        0,
				Message:      types.ChatMessage{Role: "assistant", Content: strPtr("Hello! How can I help?")},
				FinishReason: "stop",
			},
		},
		Usage: types.ChatUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	resp, err := c.ChatToResponses(chatResp, "test-model", "")
	require.NoError(t, err)

	assert.Equal(t, "chatcmpl-123", resp.ID)
	assert.Equal(t, "test-model", resp.Model)
	require.Len(t, resp.Output, 1)
	assert.Equal(t, "completed", resp.Output[0].Status)
	require.Len(t, resp.Output[0].Content, 1)
	assert.Equal(t, "output_text", resp.Output[0].Content[0].Type)
	assert.Equal(t, "Hello! How can I help?", resp.Output[0].Content[0].Text)

	require.NotNil(t, resp.Usage)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestChatToResponses_MultipleChoices(t *testing.T) {
	c := NewConverter()

	chatResp := &types.ChatResponse{
		ID:      "chatcmpl-456",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4o",
		Choices: []types.ChatChoice{
			{Index: 0, Message: types.ChatMessage{Role: "assistant", Content: strPtr("First")}, FinishReason: "stop"},
			{Index: 1, Message: types.ChatMessage{Role: "assistant", Content: strPtr("Second")}, FinishReason: "stop"},
		},
		Usage: types.ChatUsage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
	}

	resp, err := c.ChatToResponses(chatResp, "model", "")
	require.NoError(t, err)

	require.Len(t, resp.Output, 2)
	assert.Equal(t, "First", resp.Output[0].Content[0].Text)
	assert.Equal(t, "Second", resp.Output[1].Content[0].Text)
}

func TestBuildResponsesFromChat_MultiTurn(t *testing.T) {
	chatReq := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "system", Content: strPtr("Be helpful")},
			{Role: "user", Content: strPtr("Hello")},
			{Role: "assistant", Content: strPtr("Hi there")},
			{Role: "user", Content: strPtr("How are you?")},
		},
	}

	respReq := BuildResponsesFromChat(chatReq, false)
	assert.Equal(t, "Be helpful", respReq.Instructions)
	arr, ok := respReq.Input.([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"Hello", "Hi there", "How are you?"}, arr)
}

func TestBuildResponsesFromChat_SingleTurn(t *testing.T) {
	chatReq := &types.ChatRequest{
		Model:    "gpt-4o",
		Messages: []types.ChatMessage{{Role: "user", Content: strPtr("Hello")}},
	}

	respReq := BuildResponsesFromChat(chatReq, false)
	// Single turn: input should be plain string
	assert.Equal(t, "Hello", respReq.Input)
}

func TestBuildResponsesFromChat_NoMessages(t *testing.T) {
	chatReq := &types.ChatRequest{Model: "gpt-4o"}
	respReq := BuildResponsesFromChat(chatReq, false)
	assert.Nil(t, respReq.Input)
}

func TestBuildResponsesFromChat_WithTools(t *testing.T) {
	chatReq := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "user", Content: strPtr("What's the weather?")},
		},
		Tools: []types.Tool{
			{Type: "function", Function: types.FunctionDef{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}},
			}},
		},
		ToolChoice: "auto",
	}

	respReq := BuildResponsesFromChat(chatReq, false)
	require.Len(t, respReq.Tools, 1)
	assert.Equal(t, "function", respReq.Tools[0].Type)
	assert.Equal(t, "get_weather", respReq.Tools[0].Name)
	assert.Equal(t, "Get weather", respReq.Tools[0].Description)
	assert.Equal(t, "auto", respReq.ToolChoice)
}

func TestBuildResponsesFromChat_ToolMessages(t *testing.T) {
	chatReq := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "user", Content: strPtr("What's the weather?")},
			{Role: "assistant", ToolCalls: []types.ToolCall{
				{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "get_weather", Arguments: `{"city":"NYC"}`}},
			}},
			{Role: "tool", ToolCallID: "call_1", Content: strPtr(`{"temp":72}`)},
		},
	}

	respReq := BuildResponsesFromChat(chatReq, false)
	arr, ok := respReq.Input.([]any)
	require.True(t, ok)
	require.Len(t, arr, 3)

	// First: user text
	assert.Equal(t, "What's the weather?", arr[0])

	// Second: function_call
	fc, ok := arr[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "function_call", fc["type"])
	assert.Equal(t, "call_1", fc["call_id"])
	assert.Equal(t, "get_weather", fc["name"])
	assert.Equal(t, `{"city":"NYC"}`, fc["arguments"])

	// Third: function_call_output
	fco, ok := arr[2].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "function_call_output", fco["type"])
	assert.Equal(t, "call_1", fco["call_id"])
	assert.Equal(t, `{"temp":72}`, fco["output"])
}

func TestNormalizeRole(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// developer → system: this is critical for Codex CLI which sends developer role
		// for system-level instructions. If mapped to "user", instruction-following degrades.
		{"developer", "system"},
		// empty → user: default fallback for items without explicit role
		{"", "user"},
		// known roles pass through unchanged
		{"user", "user"},
		{"assistant", "assistant"},
		{"system", "system"},
		{"tool", "tool"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("role_%q→%q", tt.input, tt.expected), func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeRole(tt.input))
		})
	}
}

func TestNormalizeInstructions(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil returns empty", nil, ""},
		{"string passes through", "You are helpful", "You are helpful"},
		{"empty string passes through", "", ""},
		{"array of text blocks joined with double newline", []any{
			map[string]any{"type": "text", "text": "Rule 1"},
			map[string]any{"type": "text", "text": "Rule 2"},
		}, "Rule 1\n\nRule 2"},
		{"array skips items without text field", []any{
			map[string]any{"type": "text", "text": "Keep me"},
			map[string]any{"type": "image"},
			map[string]any{"type": "text", "text": "Also keep"},
		}, "Keep me\n\nAlso keep"},
		{"empty array returns empty", []any{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, NormalizeInstructions(tt.input))
		})
	}
}

func TestResponsesToChat_DeveloperRoleMappedToSystem(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{
			map[string]any{
				"type": "message",
				"role": "developer",
				"content": []any{
					map[string]any{"type": "input_text", "text": "Always respond concisely"},
				},
			},
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "Hello"},
				},
			},
		},
	}
	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 2)
	// developer → system, not user. If this becomes "user", Codex instructions
	// get treated as regular user messages and instruction-following breaks.
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "Always respond concisely", derefStr(chatReq.Messages[0].Content))
	assert.Equal(t, "user", chatReq.Messages[1].Role)
}

func TestResponsesToChat_InputItemWithoutRoleDoesNotPanic(t *testing.T) {
	c := NewConverter()
	// Item has type but no role key — previously caused panic via direct type assertion
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{
			map[string]any{
				"type": "message",
				// no "role" key
				"content": []any{
					map[string]any{"type": "input_text", "text": "Hello"},
				},
			},
		},
	}
	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)
	// Empty role normalizes to "user"
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
}

func TestResponsesToChat_InstructionsAsArray(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: "Hello",
		Instructions: []any{
			map[string]any{"type": "text", "text": "Rule one"},
			map[string]any{"type": "text", "text": "Rule two"},
		},
	}
	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "Rule one\n\nRule two", derefStr(chatReq.Messages[0].Content))
}

func TestResponsesToChat_NullInstructionsNoSystemMessage(t *testing.T) {
	c := NewConverter()
	req := &types.ResponsesRequest{
		Model:        "gpt-4o",
		Input:        "Hello",
		Instructions: nil,
	}
	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)
	// No instructions → no system message, only user message
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
}

func TestBuildResponsesFromChat_AssistantWithTextAndToolCalls(t *testing.T) {
	chatReq := &types.ChatRequest{
		Model: "gpt-4o",
		Messages: []types.ChatMessage{
			{Role: "user", Content: strPtr("Check weather")},
			{Role: "assistant", Content: strPtr("Let me check"), ToolCalls: []types.ToolCall{
				{ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "get_weather", Arguments: `{}`}},
			}},
		},
	}

	respReq := BuildResponsesFromChat(chatReq, false)
	arr, ok := respReq.Input.([]any)
	require.True(t, ok)
	require.Len(t, arr, 3)
	assert.Equal(t, "Check weather", arr[0])
	assert.Equal(t, "Let me check", arr[1])
	fc := arr[2].(map[string]any)
	assert.Equal(t, "function_call", fc["type"])
}
