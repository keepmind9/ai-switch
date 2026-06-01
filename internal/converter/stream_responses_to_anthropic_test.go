package converter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildResponsesJSON creates a raw JSON string for Responses SSE events
// (without the "data: " prefix that buildResponsesJSON adds).
func buildResponsesJSON(eventType string, payload map[string]any) string {
	payload["type"] = eventType
	data, _ := json.Marshal(payload)
	return string(data)
}

// --- Responses SSE → Anthropic SSE ---

func TestConvertResponsesEventToAnthropicSSE_TextStream(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{MessageID: "resp_123", Model: "test-model"}

	// response.output_text.delta triggers message_start + content_block_start + content_block_delta
	done := ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_text.delta", map[string]any{
		"delta":   "Hello",
		"item_id": "item_1",
	}))
	assert.False(t, done)
	assert.True(t, state.MessageStarted)
	assert.True(t, state.TextBlockOpened)
	events := w.eventTypes()
	assert.Equal(t, "message_start", events[0])
	assert.Equal(t, "content_block_start", events[1])
	assert.Equal(t, "content_block_delta", events[2])

	// More text
	done = ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_text.delta", map[string]any{
		"delta":   " world",
		"item_id": "item_1",
	}))
	assert.False(t, done)
	assert.Equal(t, "Hello world", state.AccText)

	// response.completed — closes text block, emits message_delta + message_stop
	done = ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.completed", map[string]any{
		"response": map[string]any{
			"id":     "resp_123",
			"status": "completed",
			"usage":  map[string]any{"input_tokens": 10, "output_tokens": 5},
		},
	}))
	assert.True(t, done)
	events = w.eventTypes()
	assert.Contains(t, events, "content_block_stop")
	assert.Contains(t, events, "message_delta")
	assert.Contains(t, events, "message_stop")

	// Verify stop_reason
	var deltaEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "message_delta" {
			deltaEvent = &w.events[i]
			break
		}
	}
	require.NotNil(t, deltaEvent)
	data, _ := deltaEvent.data.(map[string]any)
	delta, _ := data["delta"].(map[string]any)
	assert.Equal(t, "end_turn", delta["stop_reason"])
}

func TestConvertResponsesEventToAnthropicSSE_EmptyResponseThenDone(t *testing.T) {
	// Edge case: no content arrives before [DONE]. Should not hang or panic.
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{}

	done := ConvertResponsesEventToAnthropicSSE(w, state, "[DONE]")
	assert.True(t, done)
	// No message was started, so no events
	assert.Empty(t, w.events)
}

func TestConvertResponsesEventToAnthropicSSE_EmptyTextResponse(t *testing.T) {
	// Text block opens but no actual text content before response.completed.
	// content_block_stop must still be emitted to avoid Anthropic client hang.
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{MessageID: "resp_1", Model: "model"}

	// Empty delta — should be ignored
	done := ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_text.delta", map[string]any{
		"delta":   "",
		"item_id": "item_1",
	}))
	assert.False(t, done)
	assert.False(t, state.TextBlockOpened, "empty delta should not open text block")

	// Non-empty delta opens the block
	done = ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_text.delta", map[string]any{
		"delta":   "Hi",
		"item_id": "item_1",
	}))
	assert.False(t, done)
	assert.True(t, state.TextBlockOpened)

	// response.completed closes it
	done = ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.completed", map[string]any{
		"response": map[string]any{
			"id":     "resp_1",
			"status": "completed",
			"usage":  map[string]any{"input_tokens": 5, "output_tokens": 1},
		},
	}))
	assert.True(t, done)
	assert.Contains(t, w.eventTypes(), "content_block_stop")
	assert.Contains(t, w.eventTypes(), "message_stop")
}

func TestConvertResponsesEventToAnthropicSSE_DoneClosesTextBlock(t *testing.T) {
	// [DONE] must close text block if it was opened and response.completed wasn't received.
	// This happens when upstream stream is interrupted.
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{MessageID: "resp_1", Model: "model"}

	// Open text block
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_text.delta", map[string]any{
		"delta":   "Partial",
		"item_id": "item_1",
	}))
	assert.True(t, state.TextBlockOpened)

	// [DONE] without response.completed — must close text block
	done := ConvertResponsesEventToAnthropicSSE(w, state, "[DONE]")
	assert.True(t, done)
	events := w.eventTypes()
	assert.Contains(t, events, "content_block_stop", "[DONE] must close text block")
	assert.Contains(t, events, "message_delta")
	assert.Contains(t, events, "message_stop")
}

func TestConvertResponsesEventToAnthropicSSE_ToolUseBlockIndex(t *testing.T) {
	// Tool block indices must match between content_block_start and content_block_stop.
	// Previously used raw["output_index"] which was wrong — now uses ToolBlockIndices lookup.
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{MessageID: "resp_1", Model: "model"}

	// Tool call added
	done := ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "call_1",
			"name":    "get_weather",
		},
	}))
	assert.False(t, done)

	// Record the block index assigned
	require.NotNil(t, state.ToolBlockIndices)
	startIdx := state.ToolBlockIndices["call_1"]

	// Arguments delta
	done = ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.function_call_arguments.delta", map[string]any{
		"delta":        `{"city":"NYC"}`,
		"output_index": 0,
		"item_id":      "item_1",
	}))
	assert.False(t, done)

	// Tool call done — block index must match
	done = ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.done", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "call_1",
		},
	}))
	assert.False(t, done)

	// Find content_block_start and content_block_stop for tool_use
	var startEvent, stopEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "content_block_start" {
			startEvent = &w.events[i]
		}
		if w.events[i].eventType == "content_block_stop" {
			stopEvent = &w.events[i]
		}
	}
	require.NotNil(t, startEvent)
	require.NotNil(t, stopEvent)

	startData, _ := startEvent.data.(map[string]any)
	stopData, _ := stopEvent.data.(map[string]any)
	assert.Equal(t, startIdx, startData["index"], "content_block_start index should match ToolBlockIndices")
	assert.Equal(t, startIdx, stopData["index"], "content_block_stop index must match content_block_start")
}

func TestConvertResponsesEventToAnthropicSSE_MultipleToolCalls(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{MessageID: "resp_1", Model: "model"}

	// First tool call
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "call_1",
			"name":    "get_weather",
		},
	}))
	// Second tool call
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.added", map[string]any{
		"output_index": 1,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "call_2",
			"name":    "get_time",
		},
	}))

	// Both done
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.done", map[string]any{
		"output_index": 0,
		"item":         map[string]any{"type": "function_call", "call_id": "call_1"},
	}))
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.done", map[string]any{
		"output_index": 1,
		"item":         map[string]any{"type": "function_call", "call_id": "call_2"},
	}))

	// response.completed — stop_reason should be tool_use
	done := ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.completed", map[string]any{
		"response": map[string]any{
			"id":     "resp_1",
			"status": "completed",
			"usage":  map[string]any{"input_tokens": 10, "output_tokens": 20},
		},
	}))
	assert.True(t, done)

	// Count content_block_start for tool_use
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

	// stop_reason must be tool_use
	var deltaEvent *sseEvent
	for i := range w.events {
		if w.events[i].eventType == "message_delta" {
			deltaEvent = &w.events[i]
			break
		}
	}
	require.NotNil(t, deltaEvent)
	data, _ := deltaEvent.data.(map[string]any)
	delta, _ := data["delta"].(map[string]any)
	assert.Equal(t, "tool_use", delta["stop_reason"])
}

func TestConvertResponsesEventToAnthropicSSE_TextThenToolUse(t *testing.T) {
	// Text block followed by tool call block — text block must close properly.
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{MessageID: "resp_1", Model: "model"}

	// Text
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_text.delta", map[string]any{
		"delta":   "Let me search",
		"item_id": "item_1",
	}))
	assert.True(t, state.TextBlockOpened)
	textBlockIdx := state.TextBlockIdx

	// Tool call
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.added", map[string]any{
		"output_index": 1,
		"item": map[string]any{
			"type":    "function_call",
			"call_id": "call_1",
			"name":    "search",
		},
	}))
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_item.done", map[string]any{
		"output_index": 1,
		"item":         map[string]any{"type": "function_call", "call_id": "call_1"},
	}))

	// response.completed
	done := ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.completed", map[string]any{
		"response": map[string]any{
			"id":     "resp_1",
			"status": "completed",
			"usage":  map[string]any{"input_tokens": 10, "output_tokens": 10},
		},
	}))
	assert.True(t, done)

	// Verify text content_block_stop uses correct index
	stopCount := 0
	for _, e := range w.events {
		if e.eventType == "content_block_stop" {
			data, _ := e.data.(map[string]any)
			idx, _ := data["index"].(int)
			if idx == textBlockIdx {
				stopCount++
			}
		}
	}
	// At least one content_block_stop for text block
	assert.GreaterOrEqual(t, stopCount, 1, "text block should be closed")
}

func TestConvertResponsesEventToAnthropicSSE_IgnoresPassthroughEvents(t *testing.T) {
	// Events that don't need conversion should be silently ignored
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{}

	for _, eventType := range []string{
		"response.output_text.done",
		"response.content_part.done",
		"response.content_part.added",
	} {
		done := ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON(eventType, map[string]any{}))
		assert.False(t, done, "%s should not terminate stream", eventType)
	}
	assert.Empty(t, w.events)
}

func TestConvertResponsesEventToAnthropicSSE_InvalidJSON(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{}

	done := ConvertResponsesEventToAnthropicSSE(w, state, "not json")
	assert.False(t, done)
	assert.Empty(t, w.events)
}
