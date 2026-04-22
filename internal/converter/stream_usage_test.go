package converter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConvertChatChunkToResponsesSSE_CapturesUsage(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesStreamState{Created: 1234, Model: "test-model"}

	// First chunk: creates response
	done := ConvertChatChunkToResponsesSSE(w, state, `{"id":"resp_1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`)
	assert.False(t, done)

	// Content chunk
	done = ConvertChatChunkToResponsesSSE(w, state, `{"id":"resp_1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":"stop"}]}`)
	assert.False(t, done)

	// Final chunk with usage
	done = ConvertChatChunkToResponsesSSE(w, state, `{"id":"resp_1","object":"chat.completion.chunk","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`)
	assert.False(t, done)

	assert.Equal(t, 100, state.InputTokens)
	assert.Equal(t, 50, state.OutputTokens)

	// [DONE] triggers emitResponseCompleted
	done = ConvertChatChunkToResponsesSSE(w, state, "[DONE]")
	assert.True(t, done)
	assert.Contains(t, w.eventTypes(), "response.completed")
}

func TestConvertResponsesLineToChat_CapturesUsage(t *testing.T) {
	state := &ResponsesToChatState{}

	// response.created
	result := ConvertResponsesLineToChat(state, `data: {"type":"response.created","response":{"id":"resp_1","model":"test-model"}}`)
	assert.NotNil(t, result)
	assert.True(t, state.Started)

	// response.completed with usage
	result = ConvertResponsesLineToChat(state, `data: {"type":"response.completed","response":{"id":"resp_1","model":"test-model","usage":{"input_tokens":75,"output_tokens":30,"total_tokens":105}}}`)
	assert.NotNil(t, result)

	assert.Equal(t, 75, state.InputTokens)
	assert.Equal(t, 30, state.OutputTokens)
}

func TestConvertAnthropicLineToChat_CapturesUsage(t *testing.T) {
	state := &AnthropicToChatState{}

	// message_start with input_tokens
	result := ConvertAnthropicLineToChat(state, `data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":200}}}`)
	assert.NotNil(t, result)
	assert.Equal(t, 200, state.InputTokens)

	// message_delta with output_tokens
	result = ConvertAnthropicLineToChat(state, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":80}}`)
	assert.NotNil(t, result)
	assert.Equal(t, 80, state.OutputTokens)
}

func TestConvertAnthropicLineToResponses_CapturesUsage(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicToResponsesState{}

	// message_start
	done := ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":150}}}`)
	assert.False(t, done)
	assert.Equal(t, 150, state.InputTokens)
	assert.Equal(t, "claude-sonnet-4-5", state.Model)

	// message_delta with output_tokens - triggers response.completed and returns true
	done = ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":60}}`)
	assert.True(t, done)
	assert.Equal(t, 60, state.OutputTokens)
}

func TestConvertChatChunkToAnthropicSSE_CapturesUsage(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{Model: "test-model"}

	// First chunk with role
	done := ConvertChatChunkToAnthropicSSE(w, state, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":""}]}`)
	assert.False(t, done)

	// Content chunk
	done = ConvertChatChunkToAnthropicSSE(w, state, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":""}]}`)
	assert.False(t, done)

	// Final chunk with usage from stream_options.include_usage
	done = ConvertChatChunkToAnthropicSSE(w, state, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`)
	assert.False(t, done)

	assert.Equal(t, 100, state.InputTokens)
	assert.Equal(t, 50, state.OutputTokens)
}

func TestConvertChatChunkToAnthropicSSE_OutputTokenTracking(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{Model: "test-model"}

	// First chunk with role
	done := ConvertChatChunkToAnthropicSSE(w, state, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":""}]}`)
	assert.False(t, done)
	assert.True(t, state.ContentSent)

	// Content chunk increments OutputTokens
	done = ConvertChatChunkToAnthropicSSE(w, state, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hello world"},"finish_reason":""}]}`)
	assert.False(t, done)
	assert.Equal(t, 1, state.OutputTokens)

	// Finish
	done = ConvertChatChunkToAnthropicSSE(w, state, `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
	assert.False(t, done)
	assert.Contains(t, w.eventTypes(), "message_delta")
}

func TestResponsesStreamState_NoUsage(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesStreamState{Created: 1234, Model: "test-model"}

	// Stream without usage data
	done := ConvertChatChunkToResponsesSSE(w, state, `{"id":"resp_1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`)
	assert.False(t, done)

	done = ConvertChatChunkToResponsesSSE(w, state, `{"id":"resp_1","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}`)
	assert.False(t, done)

	done = ConvertChatChunkToResponsesSSE(w, state, "[DONE]")
	assert.True(t, done)

	// Without usage data, tokens remain zero
	assert.Equal(t, 0, state.InputTokens)
	assert.Equal(t, 0, state.OutputTokens)
}

func TestAnthropicToChatState_ChatStreamUsage(t *testing.T) {
	state := &AnthropicToChatState{ID: "msg_1", Model: "claude-sonnet-4-5", InputTokens: 200, OutputTokens: 80}
	id, model, in, out := state.ChatStreamUsage()
	assert.Equal(t, "msg_1", id)
	assert.Equal(t, "claude-sonnet-4-5", model)
	assert.Equal(t, 200, in)
	assert.Equal(t, 80, out)
}

func TestResponsesToChatState_ChatStreamUsage(t *testing.T) {
	state := &ResponsesToChatState{ID: "resp_1", Model: "gpt-4o", InputTokens: 75, OutputTokens: 30}
	id, model, in, out := state.ChatStreamUsage()
	assert.Equal(t, "resp_1", id)
	assert.Equal(t, "gpt-4o", model)
	assert.Equal(t, 75, in)
	assert.Equal(t, 30, out)
}

func TestResponsesToChatState_NoUsage(t *testing.T) {
	state := &ResponsesToChatState{}

	// response.completed without usage field
	result := ConvertResponsesLineToChat(state, `data: {"type":"response.completed","response":{"id":"resp_1","model":"test"}}`)
	assert.NotNil(t, result)

	assert.Equal(t, 0, state.InputTokens)
	assert.Equal(t, 0, state.OutputTokens)
}
