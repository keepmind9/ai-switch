package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSSEWriter captures SSE events for testing
type mockSSEWriter struct {
	events []sseEvent
}

type sseEvent struct {
	eventType string
	data      any
}

func (m *mockSSEWriter) WriteEvent(eventType string, data any) {
	m.events = append(m.events, sseEvent{eventType: eventType, data: data})
}

func (m *mockSSEWriter) eventTypes() []string {
	var result []string
	for _, e := range m.events {
		result = append(result, e.eventType)
	}
	return result
}

func chatChunkJSON(id string, role, content, finishReason string) string {
	chunk := types.ChatStreamResponse{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "test-model",
		Choices: []types.StreamChoice{
			{
				Index:        0,
				Delta:        types.ChatMessage{Role: role, Content: content},
				FinishReason: finishReason,
			},
		},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func chatChunkWithUsage(id string, promptTokens, completionTokens int) string {
	chunk := types.ChatStreamResponse{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "test-model",
		Usage:   &types.ChatUsage{PromptTokens: promptTokens, CompletionTokens: completionTokens, TotalTokens: promptTokens + completionTokens},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func chatChunkWithToolCalls(id string, toolCalls []types.ToolCall, finishReason string) string {
	chunk := types.ChatStreamResponse{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: 1234567890,
		Model:   "test-model",
		Choices: []types.StreamChoice{
			{
				Index:        0,
				Delta:        types.ChatMessage{ToolCalls: toolCalls},
				FinishReason: finishReason,
			},
		},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

// --- Chat → Responses SSE ---

func TestConvertChatChunkToResponsesSSE_FullStream(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesStreamState{
		Created: 1234567890,
		Model:   "test-model",
	}

	// Chunk 1: role + content
	done := ConvertChatChunkToResponsesSSE(w, state, chatChunkJSON("chatcmpl-1", "assistant", "Hello", ""))
	assert.False(t, done)
	assert.Contains(t, w.eventTypes(), "response.created")
	assert.Contains(t, w.eventTypes(), "response.output_item.added")
	assert.Contains(t, w.eventTypes(), "response.content_part.added")
	assert.Contains(t, w.eventTypes(), "response.output_text.delta")

	// Chunk 2: more content
	done = ConvertChatChunkToResponsesSSE(w, state, chatChunkJSON("chatcmpl-1", "", " world", ""))
	assert.False(t, done)

	// Chunk 3: finish
	done = ConvertChatChunkToResponsesSSE(w, state, chatChunkJSON("chatcmpl-1", "", "", "stop"))
	assert.False(t, done)
	assert.Contains(t, w.eventTypes(), "response.output_text.done")
	assert.Contains(t, w.eventTypes(), "response.content_part.done")
	assert.Contains(t, w.eventTypes(), "response.output_item.done")

	// [DONE]
	done = ConvertChatChunkToResponsesSSE(w, state, "[DONE]")
	assert.True(t, done)
	assert.Contains(t, w.eventTypes(), "response.completed")

	assert.Equal(t, "Hello world", state.AccText)
}

func TestConvertChatChunkToResponsesSSE_ImmediateDone(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesStreamState{Model: "model"}

	done := ConvertChatChunkToResponsesSSE(w, state, "[DONE]")
	assert.True(t, done)
	assert.Empty(t, w.events) // no content was sent, so no completed event
}

func TestConvertChatChunkToResponsesSSE_InvalidJSON(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesStreamState{Model: "model"}

	done := ConvertChatChunkToResponsesSSE(w, state, "not json")
	assert.False(t, done)
	assert.Empty(t, w.events)
}

func TestConvertChatChunkToResponsesSSE_SingleChunkWithFinish(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesStreamState{Created: 1000, Model: "model"}

	done := ConvertChatChunkToResponsesSSE(w, state, chatChunkJSON("id-1", "assistant", "Hi", "stop"))
	assert.False(t, done)

	done = ConvertChatChunkToResponsesSSE(w, state, "[DONE]")
	assert.True(t, done)

	types := w.eventTypes()
	assert.Equal(t, "response.created", types[0])
	assert.Equal(t, "response.completed", types[len(types)-1])
}

// --- Chat → Anthropic SSE ---

func TestConvertChatChunkToAnthropicSSE_FullStream(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	// Chunk 1: role announcement
	done := ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "assistant", "", ""))
	assert.False(t, done)
	assert.Equal(t, []string{"message_start"}, w.eventTypes())

	// Chunk 2: content delta — triggers content_block_start + content_block_delta
	done = ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "", "Hello ", ""))
	assert.False(t, done)
	assert.Contains(t, w.eventTypes(), "content_block_start")
	assert.Contains(t, w.eventTypes(), "content_block_delta")

	// Chunk 3: more content
	done = ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "", "world", ""))
	assert.False(t, done)

	// Chunk 4: finish
	done = ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "", "", "stop"))
	assert.False(t, done)

	// Chunk 5: usage — triggers deferred message_delta
	done = ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithUsage("chatcmpl-a", 25, 2))
	assert.False(t, done)
	assert.Contains(t, w.eventTypes(), "content_block_stop")
	assert.Contains(t, w.eventTypes(), "message_delta")

	// [DONE]
	done = ConvertChatChunkToAnthropicSSE(w, state, "[DONE]")
	assert.True(t, done)
	assert.Contains(t, w.eventTypes(), "message_stop")

	assert.Equal(t, "Hello world", state.AccText)
}

func TestConvertChatChunkToAnthropicSSE_ContentWithoutRole(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	// Content arrives without explicit role chunk
	done := ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id-x", "", "Direct content", ""))
	assert.False(t, done)

	// Should auto-emit message_start + content_block_start
	assert.Contains(t, w.eventTypes(), "message_start")
	assert.Contains(t, w.eventTypes(), "content_block_start")
	assert.Contains(t, w.eventTypes(), "content_block_delta")
}

func TestConvertChatChunkToAnthropicSSE_FinishReasonMapping(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		expected     string
	}{
		{"stop → end_turn", "stop", "end_turn"},
		{"length → max_tokens", "length", "max_tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockSSEWriter{}
			state := &AnthropicStreamState{}

			ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "assistant", "", ""))
			ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "text", ""))
			ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "", tt.finishReason))
			ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithUsage("id", 10, 5))

			// Find message_delta event
			var deltaEvent *sseEvent
			for i := range w.events {
				if w.events[i].eventType == "message_delta" {
					deltaEvent = &w.events[i]
					break
				}
			}
			require.NotNil(t, deltaEvent)

			dataMap, ok := deltaEvent.data.(map[string]any)
			require.True(t, ok)
			delta, ok := dataMap["delta"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.expected, delta["stop_reason"])
		})
	}
}

func TestConvertChatChunkToAnthropicSSE_ImmediateDone(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	done := ConvertChatChunkToAnthropicSSE(w, state, "[DONE]")
	assert.True(t, done)
	assert.Empty(t, w.events)
}

func TestConvertChatChunkToAnthropicSSE_InvalidJSON(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	done := ConvertChatChunkToAnthropicSSE(w, state, "garbage")
	assert.False(t, done)
	assert.Empty(t, w.events)
}

// --- SSE helpers ---

func TestParseSSEDataLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected string
	}{
		{"data line", "data: {\"key\":\"val\"}", "{\"key\":\"val\"}"},
		{"done line", "data: [DONE]", "[DONE]"},
		{"event line", "event: message_start", ""},
		{"empty line", "", ""},
		{"comment line", ": keepalive", ""},
		{"no space after colon", "data:{\"key\":\"val\"}", "{\"key\":\"val\"}"},
		{"data empty value", "data:", ""},
		{"data empty with space", "data: ", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ParseSSEDataLine(tt.line))
		})
	}
}

func TestFormatSSEEvent(t *testing.T) {
	data := []byte(`{"type":"test"}`)
	result := FormatSSEEvent("message_start", data)
	assert.Equal(t, "event: message_start\ndata: {\"type\":\"test\"}\n\n", result)
}

// --- Verify event data structure ---

func TestConvertChatChunkToAnthropicSSE_MessageStartStructure(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("msg-123", "assistant", "", ""))

	require.Len(t, w.events, 1)
	assert.Equal(t, "message_start", w.events[0].eventType)

	data, ok := w.events[0].data.(map[string]any)
	require.True(t, ok)
	msg, ok := data["message"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "msg-123", msg["id"])
	assert.Equal(t, "assistant", msg["role"])

	usage, ok := msg["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 0, usage["input_tokens"])
}

func TestConvertChatChunkToAnthropicSSE_ContentBlockDeltaStructure(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "assistant", "", ""))
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "delta text", ""))

	// Find content_block_delta event
	var deltaEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "content_block_delta" {
			deltaEvent = &w.events[i]
			break
		}
	}
	require.NotNil(t, deltaEvent)

	data, ok := deltaEvent.data.(map[string]any)
	require.True(t, ok)
	delta, ok := data["delta"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "text_delta", delta["type"])
	assert.Equal(t, "delta text", delta["text"])
}

func TestConvertChatChunkToAnthropicSSE_DeferredDeltaWithUsage(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "assistant", "", ""))
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "Hello", ""))
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "", "stop"))

	// finish_reason should emit content_block_stop but NOT message_delta yet
	assert.Contains(t, w.eventTypes(), "content_block_stop")
	assert.NotContains(t, w.eventTypes(), "message_delta")

	// Usage chunk triggers message_delta with real tokens
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithUsage("id", 1500, 42))
	assert.Contains(t, w.eventTypes(), "message_delta")
	assert.Contains(t, w.eventTypes(), "message_stop")

	// Verify usage in message_delta
	var deltaEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "message_delta" {
			deltaEvent = &w.events[i]
			break
		}
	}
	require.NotNil(t, deltaEvent)
	dataMap, ok := deltaEvent.data.(map[string]any)
	require.True(t, ok)
	delta, ok := dataMap["delta"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "end_turn", delta["stop_reason"])
	usage, ok := dataMap["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1500, usage["input_tokens"], "message_delta should include real input_tokens from upstream")
	assert.Equal(t, 42, usage["output_tokens"])

	// [DONE] should not emit duplicate events
	ConvertChatChunkToAnthropicSSE(w, state, "[DONE]")
	deltaCount := 0
	for _, e := range w.events {
		if e.eventType == "message_delta" {
			deltaCount++
		}
	}
	assert.Equal(t, 1, deltaCount, "message_delta should only be emitted once")
}

func TestConvertChatChunkToAnthropicSSE_DeferredDeltaFallbackOnDone(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "assistant", "", ""))
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "text", ""))
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "", "stop"))

	// No usage chunk — [DONE] triggers fallback
	assert.NotContains(t, w.eventTypes(), "message_delta")
	done := ConvertChatChunkToAnthropicSSE(w, state, "[DONE]")
	assert.True(t, done)
	assert.Contains(t, w.eventTypes(), "message_delta")
	assert.Contains(t, w.eventTypes(), "message_stop")
}

func TestConvertChatChunkToAnthropicSSE_MessageStartHasCacheFields(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	// Content without role triggers fallback message_start
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "direct", ""))

	var startEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "message_start" {
			startEvent = &w.events[i]
			break
		}
	}
	require.NotNil(t, startEvent)

	data, ok := startEvent.data.(map[string]any)
	require.True(t, ok)
	msg, ok := data["message"].(map[string]any)
	require.True(t, ok)
	usage, ok := msg["usage"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, usage, "cache_creation_input_tokens")
	assert.Contains(t, usage, "cache_read_input_tokens")
}

// --- Chat → Anthropic SSE: Tool calls ---

func TestConvertChatChunkToAnthropicSSE_ToolCallsStream(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	// message_start + text block
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "assistant", "", ""))
	// text delta
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "Let me check.", ""))
	// tool call start (closes text block, starts tool block)
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithToolCalls("id", []types.ToolCall{
		{Index: 0, ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "get_weather"}},
	}, ""))
	// tool arguments delta
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithToolCalls("id", []types.ToolCall{
		{Index: 0, Function: types.FunctionCall{Arguments: `{"location":"SF"}`}},
	}, ""))
	// finish
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "", "tool_calls"))
	// usage
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithUsage("id", 100, 50))

	events := w.eventTypes()
	assert.Contains(t, events, "message_start")
	assert.Contains(t, events, "content_block_start")
	assert.Contains(t, events, "content_block_delta")
	assert.Contains(t, events, "content_block_stop")
	assert.Contains(t, events, "message_delta")
	assert.Contains(t, events, "message_stop")

	// Find the tool_use content_block_start
	var toolStartEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "content_block_start" {
			data, _ := w.events[i].data.(map[string]any)
			block, _ := data["content_block"].(map[string]any)
			if block != nil && block["type"] == "tool_use" {
				toolStartEvent = &w.events[i]
				break
			}
		}
	}
	require.NotNil(t, toolStartEvent, "should have tool_use content_block_start")
	data, _ := toolStartEvent.data.(map[string]any)
	block, _ := data["content_block"].(map[string]any)
	assert.Equal(t, "call_1", block["id"])
	assert.Equal(t, "get_weather", block["name"])

	// Find input_json_delta
	var jsonDeltaEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "content_block_delta" {
			evData, _ := w.events[i].data.(map[string]any)
			delta, _ := evData["delta"].(map[string]any)
			if delta != nil && delta["type"] == "input_json_delta" {
				jsonDeltaEvent = &w.events[i]
				break
			}
		}
	}
	require.NotNil(t, jsonDeltaEvent, "should have input_json_delta")
	evData, _ := jsonDeltaEvent.data.(map[string]any)
	delta, _ := evData["delta"].(map[string]any)
	assert.Equal(t, `{"location":"SF"}`, delta["partial_json"])

	// Stop reason should be tool_use
	var deltaEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "message_delta" {
			deltaEvent = &w.events[i]
			break
		}
	}
	require.NotNil(t, deltaEvent)
	deltaData, _ := deltaEvent.data.(map[string]any)
	deltaMap, _ := deltaData["delta"].(map[string]any)
	assert.Equal(t, "tool_use", deltaMap["stop_reason"])
}

func TestConvertChatChunkToAnthropicSSE_ToolCallsOnlyNoText(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	// Tool call arrives without prior text
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithToolCalls("id", []types.ToolCall{
		{Index: 0, ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "calc"}},
	}, ""))

	assert.True(t, state.ContentSent)
	require.NotNil(t, state.ToolBlocks)
	assert.Len(t, state.ToolBlocks, 1)

	events := w.eventTypes()
	assert.Contains(t, events, "message_start")
	assert.Contains(t, events, "content_block_start")
	// No text block was started, so no text block close needed
	// content_block_stop only emitted for tool blocks at [DONE]
}

func TestConvertChatChunkToAnthropicSSE_MultipleToolCalls(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	// Start stream
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "assistant", "", ""))
	// First tool call
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithToolCalls("id", []types.ToolCall{
		{Index: 0, ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "get_weather"}},
	}, ""))
	// Second tool call
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithToolCalls("id", []types.ToolCall{
		{Index: 1, ID: "call_2", Type: "function", Function: types.FunctionCall{Name: "get_time"}},
	}, ""))
	// Arguments for both
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithToolCalls("id", []types.ToolCall{
		{Index: 0, Function: types.FunctionCall{Arguments: `{"loc":"SF"}`}},
		{Index: 1, Function: types.FunctionCall{Arguments: `{"tz":"PST"}`}},
	}, ""))
	// Finish + usage
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "", "tool_calls"))
	ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithUsage("id", 50, 20))

	// Count tool_use content_block_start events
	toolStartCount := 0
	for _, e := range w.events {
		if e.eventType == "content_block_start" {
			data, _ := e.data.(map[string]any)
			block, _ := data["content_block"].(map[string]any)
			if block != nil && block["type"] == "tool_use" {
				toolStartCount++
			}
		}
	}
	assert.Equal(t, 2, toolStartCount)

	// Both tool blocks should be closed
	for _, tbs := range state.ToolBlocks {
		assert.True(t, tbs.Stopped)
	}
}

func TestConvertChatChunkToAnthropicSSE_ToolCallsFinishReason(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		expected     string
	}{
		{"tool_calls → tool_use", "tool_calls", "tool_use"},
		{"stop → end_turn", "stop", "end_turn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &mockSSEWriter{}
			state := &AnthropicStreamState{}

			ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithToolCalls("id", []types.ToolCall{
				{Index: 0, ID: "call_1", Type: "function", Function: types.FunctionCall{Name: "test"}},
			}, ""))
			ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("id", "", "", tt.finishReason))
			ConvertChatChunkToAnthropicSSE(w, state, chatChunkWithUsage("id", 10, 5))

			var deltaEvent *sseEvent
			for i := range w.events {
				if w.events[i].eventType == "message_delta" {
					deltaEvent = &w.events[i]
					break
				}
			}
			require.NotNil(t, deltaEvent)
			dataMap, _ := deltaEvent.data.(map[string]any)
			delta, _ := dataMap["delta"].(map[string]any)
			assert.Equal(t, tt.expected, delta["stop_reason"])
		})
	}
}
