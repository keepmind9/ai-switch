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
	assert.Equal(t, "Hello, how are you?", chatReq.Messages[0].Content)
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
	assert.Equal(t, "You are a helpful math tutor.", chatReq.Messages[0].Content)
	assert.Equal(t, "user", chatReq.Messages[1].Role)
	assert.Equal(t, "What is 2+2?", chatReq.Messages[1].Content)
}

func TestResponsesToChat_ArrayInput(t *testing.T) {
	c := NewConverter()

	req := &types.ResponsesRequest{
		Model: "gpt-4o",
		Input: []any{"First part", "Second part"},
	}

	chatReq, err := c.ResponsesToChat(req)
	require.NoError(t, err)

	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "First part\nSecond part", chatReq.Messages[0].Content)
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
	assert.Equal(t, "Hello from Codex", chatReq.Messages[0].Content)
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
				Message:      types.ChatMessage{Role: "assistant", Content: "Hello! How can I help?"},
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
			{Index: 0, Message: types.ChatMessage{Role: "assistant", Content: "First"}, FinishReason: "stop"},
			{Index: 1, Message: types.ChatMessage{Role: "assistant", Content: "Second"}, FinishReason: "stop"},
		},
		Usage: types.ChatUsage{PromptTokens: 10, CompletionTokens: 10, TotalTokens: 20},
	}

	resp, err := c.ChatToResponses(chatResp, "model", "")
	require.NoError(t, err)

	require.Len(t, resp.Responses, 2)
	assert.Equal(t, "First", resp.Responses[0].Content[0].Text)
	assert.Equal(t, "Second", resp.Responses[1].Content[0].Text)
}
