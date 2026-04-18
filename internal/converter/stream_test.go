package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/llm-gateway/internal/types"
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
	state := &AnthropicStreamState{
		InputTokens: 25,
	}

	// Chunk 1: role announcement
	done := ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "assistant", "", ""))
	assert.False(t, done)
	assert.Equal(t, []string{"message_start", "content_block_start"}, w.eventTypes())

	// Chunk 2: content delta
	done = ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "", "Hello ", ""))
	assert.False(t, done)
	assert.Contains(t, w.eventTypes(), "content_block_delta")

	// Chunk 3: more content
	done = ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "", "world", ""))
	assert.False(t, done)

	// Chunk 4: finish
	done = ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("chatcmpl-a", "", "", "stop"))
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
	state := &AnthropicStreamState{InputTokens: 10}

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
	state := &AnthropicStreamState{InputTokens: 50}

	ConvertChatChunkToAnthropicSSE(w, state, chatChunkJSON("msg-123", "assistant", "", ""))

	require.Len(t, w.events, 2)
	assert.Equal(t, "message_start", w.events[0].eventType)

	data, ok := w.events[0].data.(map[string]any)
	require.True(t, ok)
	msg, ok := data["message"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "msg-123", msg["id"])
	assert.Equal(t, "assistant", msg["role"])

	usage, ok := msg["usage"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 50, usage["input_tokens"])
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
