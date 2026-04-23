package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{"tool_use → tool_calls", "tool_use", "tool_calls"},
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

// --- Tool use streaming tests ---

func TestConvertAnthropicLineToChat_ToolUseBlockStart(t *testing.T) {
	state := &AnthropicToChatState{}

	// message_start first
	ConvertAnthropicLineToChat(state, anthropicEventJSON("message_start", map[string]any{
		"message": map[string]any{"id": "msg-1", "model": "test"},
	}))

	// tool_use content_block_start
	result := ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_start", map[string]any{
		"index": float64(1),
		"content_block": map[string]any{
			"type": "tool_use",
			"id":   "toolu_abc",
			"name": "get_weather",
		},
	}))

	chunk := assertChatChunk(t, result)
	require.Len(t, chunk.Choices, 1)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 1)
	tc := chunk.Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, 0, tc.Index) // first tool call
	assert.Equal(t, "toolu_abc", tc.ID)
	assert.Equal(t, "function", tc.Type)
	assert.Equal(t, "get_weather", tc.Function.Name)
}

func TestConvertAnthropicLineToChat_InputJsonDelta(t *testing.T) {
	state := &AnthropicToChatState{}

	// Setup: message_start + tool block
	ConvertAnthropicLineToChat(state, anthropicEventJSON("message_start", map[string]any{
		"message": map[string]any{"id": "msg-1", "model": "test"},
	}))
	ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_start", map[string]any{
		"index": float64(1),
		"content_block": map[string]any{
			"type": "tool_use",
			"id":   "toolu_abc",
			"name": "get_weather",
		},
	}))

	// input_json_delta
	result := ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"index": float64(1),
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": `{"location":`,
		},
	}))

	chunk := assertChatChunk(t, result)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 1)
	tc := chunk.Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, 0, tc.Index)
	assert.Equal(t, `{"location":`, tc.Function.Arguments)
}

func TestConvertAnthropicLineToChat_FullToolUseStream(t *testing.T) {
	state := &AnthropicToChatState{}

	// message_start
	ConvertAnthropicLineToChat(state, anthropicEventJSON("message_start", map[string]any{
		"message": map[string]any{"id": "msg-tool", "model": "claude-3", "usage": map[string]any{"input_tokens": 50}},
	}))

	// text content_block_delta
	result := ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"delta": map[string]any{"text": "Checking..."},
	}))
	chunk := assertChatChunk(t, result)
	assert.Equal(t, "Checking...", chunk.Choices[0].Delta.Content)

	// tool_use content_block_start
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_start", map[string]any{
		"index": float64(1),
		"content_block": map[string]any{
			"type": "tool_use",
			"id":   "toolu_1",
			"name": "get_weather",
		},
	}))
	chunk = assertChatChunk(t, result)
	assert.Equal(t, "toolu_1", chunk.Choices[0].Delta.ToolCalls[0].ID)

	// input_json_delta
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"index": float64(1),
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": `{"location":"SF"}`,
		},
	}))
	chunk = assertChatChunk(t, result)
	assert.Equal(t, `{"location":"SF"}`, chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)

	// message_delta with tool_use stop
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("message_delta", map[string]any{
		"delta": map[string]any{"stop_reason": "tool_use"},
		"usage": map[string]any{"output_tokens": 20},
	}))
	chunk = assertChatChunk(t, result)
	assert.Equal(t, "tool_calls", chunk.Choices[0].FinishReason)

	// message_stop
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("message_stop", nil))
	assert.Equal(t, "[DONE]", result)
}

func TestConvertAnthropicLineToChat_MultipleToolUseBlocks(t *testing.T) {
	state := &AnthropicToChatState{}

	ConvertAnthropicLineToChat(state, anthropicEventJSON("message_start", map[string]any{
		"message": map[string]any{"id": "msg-multi", "model": "test"},
	}))

	// First tool
	result := ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_start", map[string]any{
		"index": float64(0),
		"content_block": map[string]any{
			"type": "tool_use",
			"id":   "toolu_1",
			"name": "get_weather",
		},
	}))
	chunk := assertChatChunk(t, result)
	assert.Equal(t, 0, chunk.Choices[0].Delta.ToolCalls[0].Index)

	// Second tool
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_start", map[string]any{
		"index": float64(1),
		"content_block": map[string]any{
			"type": "tool_use",
			"id":   "toolu_2",
			"name": "get_time",
		},
	}))
	chunk = assertChatChunk(t, result)
	assert.Equal(t, 1, chunk.Choices[0].Delta.ToolCalls[0].Index)
	assert.Equal(t, "toolu_2", chunk.Choices[0].Delta.ToolCalls[0].ID)

	// Arguments for first tool
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"index": float64(0),
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": `{"loc":"SF"}`,
		},
	}))
	chunk = assertChatChunk(t, result)
	assert.Equal(t, 0, chunk.Choices[0].Delta.ToolCalls[0].Index)
	assert.Equal(t, `{"loc":"SF"}`, chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)

	// Arguments for second tool
	result = ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_delta", map[string]any{
		"index": float64(1),
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": `{"tz":"PST"}`,
		},
	}))
	chunk = assertChatChunk(t, result)
	assert.Equal(t, 1, chunk.Choices[0].Delta.ToolCalls[0].Index)
	assert.Equal(t, `{"tz":"PST"}`, chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)
}

func TestConvertAnthropicLineToChat_SkipsUnknownBlockTypes(t *testing.T) {
	state := &AnthropicToChatState{}

	// content_block_start with unknown type should be nil
	result := ConvertAnthropicLineToChat(state, anthropicEventJSON("content_block_start", map[string]any{
		"index": float64(0),
		"content_block": map[string]any{
			"type": "thinking",
		},
	}))
	assert.Nil(t, result)
}
