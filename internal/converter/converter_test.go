package converter

import (
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
	require.Len(t, resp.Responses, 1)
	assert.Equal(t, "completed", resp.Responses[0].Status)
	require.Len(t, resp.Responses[0].Content, 1)
	assert.Equal(t, "output_text", resp.Responses[0].Content[0].Type)
	assert.Equal(t, "Hello! How can I help?", resp.Responses[0].Content[0].Text)

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

	require.Len(t, resp.Responses, 2)
	assert.Equal(t, "First", resp.Responses[0].Content[0].Text)
	assert.Equal(t, "Second", resp.Responses[1].Content[0].Text)
}

func TestResponsesToChatResponse(t *testing.T) {
	resp := &types.ResponsesResponse{
		ID:      "resp-123",
		Object:  "response",
		Created: 1234567890,
		Model:   "gpt-4o",
		Responses: []types.ResponseItem{
			{
				ID:     "item-1",
				Object: "response",
				Role:   "assistant",
				Content: []types.ContentBlock{
					{Type: "output_text", Text: "Hello world"},
				},
				Status: "completed",
			},
		},
		Usage: &types.Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
	}

	c := NewConverter()
	chatResp, err := c.ResponsesToChatResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, "resp-123", chatResp.ID)
	assert.Equal(t, "chat.completion", chatResp.Object)
	assert.Equal(t, "gpt-4o", chatResp.Model)
	require.Len(t, chatResp.Choices, 1)
	assert.Equal(t, "Hello world", derefStr(chatResp.Choices[0].Message.Content))
	assert.Equal(t, "stop", chatResp.Choices[0].FinishReason)
	assert.Equal(t, 10, chatResp.Usage.PromptTokens)
	assert.Equal(t, 20, chatResp.Usage.CompletionTokens)
}

func TestResponsesToChatResponse_Empty(t *testing.T) {
	resp := &types.ResponsesResponse{ID: "r1", Model: "m1"}
	c := NewConverter()
	chatResp, err := c.ResponsesToChatResponse(resp)
	require.NoError(t, err)
	assert.Empty(t, chatResp.Choices)
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

func TestResponsesToChatResponse_WithFunctionCall(t *testing.T) {
	resp := &types.ResponsesResponse{
		ID:     "resp-001",
		Object: "response",
		Model:  "gpt-4o",
		Responses: []types.ResponseItem{
			{
				ID:   "item-1",
				Type: "message",
				Content: []types.ContentBlock{
					{Type: "output_text", Text: "Let me check"},
				},
			},
			{
				ID:        "fc-1",
				Type:      "function_call",
				CallID:    "call_abc",
				Name:      "get_weather",
				Arguments: `{"city":"NYC"}`,
			},
		},
		Usage: &types.Usage{InputTokens: 10, OutputTokens: 20, TotalTokens: 30},
	}

	c := NewConverter()
	chatResp, err := c.ResponsesToChatResponse(resp)
	require.NoError(t, err)
	require.Len(t, chatResp.Choices, 1)
	assert.Equal(t, "Let me check", derefStr(chatResp.Choices[0].Message.Content))
	require.Len(t, chatResp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "call_abc", chatResp.Choices[0].Message.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", chatResp.Choices[0].Message.ToolCalls[0].Function.Name)
	assert.Equal(t, `{"city":"NYC"}`, chatResp.Choices[0].Message.ToolCalls[0].Function.Arguments)
	assert.Equal(t, "tool_calls", chatResp.Choices[0].FinishReason)
}

func TestResponsesToChatResponse_MultipleFunctionCalls(t *testing.T) {
	resp := &types.ResponsesResponse{
		ID:     "resp-002",
		Object: "response",
		Model:  "gpt-4o",
		Responses: []types.ResponseItem{
			{
				ID:        "fc-1",
				Type:      "function_call",
				CallID:    "call_a",
				Name:      "get_weather",
				Arguments: `{"city":"NYC"}`,
			},
			{
				ID:        "fc-2",
				Type:      "function_call",
				CallID:    "call_b",
				Name:      "get_stock",
				Arguments: `{"sym":"AAPL"}`,
			},
		},
	}

	c := NewConverter()
	chatResp, err := c.ResponsesToChatResponse(resp)
	require.NoError(t, err)
	require.Len(t, chatResp.Choices, 1)
	require.Len(t, chatResp.Choices[0].Message.ToolCalls, 2)
	assert.Equal(t, "call_a", chatResp.Choices[0].Message.ToolCalls[0].ID)
	assert.Equal(t, "call_b", chatResp.Choices[0].Message.ToolCalls[1].ID)
	assert.Equal(t, "tool_calls", chatResp.Choices[0].FinishReason)
}
