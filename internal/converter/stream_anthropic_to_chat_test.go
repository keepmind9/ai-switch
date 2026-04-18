package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
)

func anthropicEventJSON(eventType string, extra map[string]any) string {
	raw := map[string]any{"type": eventType}
	for k, v := range extra {
		raw[k] = v
	}
	data, _ := json.Marshal(raw)
	return "data: " + string(data)
}

func assertChatChunk(t *testing.T, result any) *types.ChatStreamResponse {
	t.Helper()
	chunk, ok := result.(*types.ChatStreamResponse)
	assert.True(t, ok, "expected *types.ChatStreamResponse, got %T", result)
	return chunk
}

func TestConvertAnthropicLineToChat_MessageStart(t *testing.T) {
	state := &AnthropicToChatState{}
	line := anthropicEventJSON("message_start", map[string]any{
		"message": map[string]any{
			"id":    "msg-001",
			"model": "claude-3-sonnet",
			"usage": map[string]any{"input_tokens": 100},
		},
	})

	result := ConvertAnthropicLineToChat(state, line)
	chunk := assertChatChunk(t, result)
	assert.Equal(t, "msg-001", chunk.ID)
	assert.Equal(t, "assistant", chunk.Choices[0].Delta.Role)
	assert.True(t, state.Started)
	assert.Equal(t, "msg-001", state.ID)
	assert.Equal(t, "claude-3-sonnet", state.Model)
	assert.Equal(t, 100, state.InputTokens)
}

func TestConvertAnthropicLineToChat_FullStream(t *testing.T) {
	state := &AnthropicToChatState{}

	// message_start
	result := ConvertAnthropicLineToChat(state, anthropicEventJSON("message_start", map[string]any{
		"message": map[string]any{
			"id":    "msg-abc",
			"model": "claude-3",
			"usage": map[string]any{"input_tokens": 50},
		},
	}))
	assert.NotNil(t, result)
	assert.True(t, state.Started)

	// content_block_delta
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"delta": map[string]any{"text": "Hello "},
	}))
	chunk := assertChatChunk(t, result)
	assert.Equal(t, "Hello ", chunk.Choices[0].Delta.Content)

	// content_block_delta 2
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"delta": map[string]any{"text": "world"},
	}))
	assert.NotNil(t, result)
	assert.Equal(t, "Hello world", state.AccText)

	// message_delta with stop
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": "end_turn"},
		"usage": map[string]any{"output_tokens": 10},
	}))
	chunk = assertChatChunk(t, result)
	assert.Equal(t, "stop", chunk.Choices[0].FinishReason)
	assert.Equal(t, 10, state.OutputTokens)

	// message_stop
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("message_stop", nil))
	assert.Equal(t, "[DONE]", result)
}

func TestConvertAnthropicLineToChat_SkipsEventLines(t *testing.T) {
	state := &AnthropicToChatState{}

	assert.Nil(t, ConvertAnthropicLineToChat(state, "event: message_start"))
	assert.Nil(t, ConvertAnthropicLineToChat(state, ""))
}

func TestConvertAnthropicLineToChat_SkipsInvalidJSON(t *testing.T) {
	state := &AnthropicToChatState{}
	assert.Nil(t, ConvertAnthropicLineToChat(state, "data: not-json"))
}

func TestConvertAnthropicLineToChat_SkipsEmptyDelta(t *testing.T) {
	state := &AnthropicToChatState{Started: true}
	assert.Nil(t, ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"delta": map[string]any{"type": "text_delta"},
	})))
}

func TestConvertAnthropicLineToChat_SkipsNilDelta(t *testing.T) {
	state := &AnthropicToChatState{Started: true}
	assert.Nil(t, ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", nil)))
}

func TestConvertAnthropicLineToChat_SkipsUnknownEvent(t *testing.T) {
	state := &AnthropicToChatState{}
	assert.Nil(t, ConvertAnthropicLineToChat(state, anthropicEventJSON("ping", nil)))
}

func TestConvertAnthropicLineToChat_StopReasonMapping(t *testing.T) {
	tests := []struct {
		name     string
		reason   string
		expected string
	}{
		{"end_turn → stop", "end_turn", "stop"},
		{"max_tokens → length", "max_tokens", "length"},
		{"tool_use passthrough", "tool_use", "tool_use"},
		{"stop_sequence passthrough", "stop_sequence", "stop_sequence"},
		{"empty → empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &AnthropicToChatState{Started: true, ID: "msg-1", Model: "test", Created: 1000}
			result := ConvertAnthropicLineToChat(state, anthropicEventJSON("message_delta", map[string]any{
				"delta": map[string]any{"stop_reason": tt.reason},
			}))
			chunk := assertChatChunk(t, result)
			assert.Equal(t, tt.expected, chunk.Choices[0].FinishReason)
		})
	}
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float64
	}{
		{"float64", float64(3.14), 3.14},
		{"int", 42, 42},
		{"int64", int64(100), 100},
		{"json.Number", json.Number("3.14"), 3.14},
		{"nil", nil, 0},
		{"string", "abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, toFloat64(tt.input))
		})
	}
}
