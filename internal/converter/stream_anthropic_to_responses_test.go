package converter

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSSEWriter struct {
	buf bytes.Buffer
}

func (w *testSSEWriter) WriteEvent(eventType string, data any) {
	w.buf.WriteString("event: " + eventType + "\ndata: ")
	jsonData, _ := json.Marshal(data)
	w.buf.Write(jsonData)
	w.buf.WriteString("\n\n")
}

func TestConvertAnthropicLineToResponses(t *testing.T) {
	w := &testSSEWriter{}
	state := &AnthropicToResponsesState{Model: "test-model"}

	// message_start
	done := ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_start","message":{"id":"msg_123","model":"test-model","usage":{"input_tokens":10}}}`)
	assert.False(t, done)
	assert.True(t, state.CreatedSent)
	assert.Equal(t, "msg_123", state.ResponseID)
	output := w.buf.String()
	assert.Contains(t, output, "response.created")
	assert.Contains(t, output, "in_progress")

	w.buf.Reset()

	// content_block_start
	done = ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	assert.False(t, done)
	assert.True(t, state.ItemSent)
	output = w.buf.String()
	assert.Contains(t, output, "response.output_item.added")
	assert.Contains(t, output, "response.content_part.added")

	w.buf.Reset()

	// content_block_delta
	done = ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`)
	assert.False(t, done)
	assert.Equal(t, "Hello", state.AccText)
	output = w.buf.String()
	assert.Contains(t, output, "response.output_text.delta")
	assert.Contains(t, output, "Hello")

	w.buf.Reset()

	// message_delta (stop)
	done = ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
	assert.True(t, done)
	output = w.buf.String()
	assert.Contains(t, output, "response.output_text.done")
	assert.Contains(t, output, "response.output_item.done")
	assert.Contains(t, output, "response.completed")
	assert.Contains(t, output, "completed")
}

func TestConvertAnthropicLineToResponses_SkipEvents(t *testing.T) {
	w := &testSSEWriter{}
	state := &AnthropicToResponsesState{Model: "test"}

	// event lines should be skipped
	done := ConvertAnthropicLineToResponses(w, state, "event: message_start")
	assert.False(t, done)
	assert.Empty(t, w.buf.String())

	// empty lines should be skipped
	done = ConvertAnthropicLineToResponses(w, state, "")
	assert.False(t, done)

	// invalid JSON should be skipped
	done = ConvertAnthropicLineToResponses(w, state, "data: not json")
	assert.False(t, done)
}

func TestConvertAnthropicLineToResponses_ThinkTag(t *testing.T) {
	w := &testSSEWriter{}
	state := &AnthropicToResponsesState{Model: "test", ThinkTag: "think"}

	// Initialize with message_start and content_block_start
	ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_start","message":{"id":"msg_1","model":"test","usage":{"input_tokens":0}}}`)
	w.buf.Reset()
	ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	w.buf.Reset()

	// Content with think tag should be filtered
	done := ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"<think">}}`)
	assert.False(t, done)
	// The tag state should start filtering
	w.buf.Reset()

	done = ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"reasoning here</think">}}`)
	assert.False(t, done)

	w.buf.Reset()

	// After think tag closes, normal text should pass through
	done = ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello world"}}`)
	assert.False(t, done)
	output := w.buf.String()
	require.Contains(t, output, "response.output_text.delta")
	assert.Contains(t, output, "Hello world")
}

func TestConvertAnthropicLineToResponses_MultipleDeltas(t *testing.T) {
	w := &testSSEWriter{}
	state := &AnthropicToResponsesState{Model: "test"}

	ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_start","message":{"id":"msg_1","model":"test","usage":{"input_tokens":5}}}`)
	w.buf.Reset()
	ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	w.buf.Reset()

	// Multiple content deltas
	ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi "}}`)
	w.buf.Reset()
	ConvertAnthropicLineToResponses(w, state, `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"there"}}`)
	w.buf.Reset()

	assert.Equal(t, "Hi there", state.AccText)

	// Stop with message_delta
	done := ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`)
	assert.True(t, done)
	output := w.buf.String()
	assert.Contains(t, output, `"text":"Hi there"`)
	assert.Contains(t, output, `"input_tokens":5`)
	assert.Contains(t, output, `"output_tokens":2`)
}

func TestConvertAnthropicLineToResponses_MessageStopFallback(t *testing.T) {
	w := &testSSEWriter{}
	state := &AnthropicToResponsesState{Model: "test"}

	// message_stop without prior content should still emit response.completed
	done := ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_stop"}`)
	assert.True(t, done)
	assert.True(t, state.CompletedSent)
	output := w.buf.String()
	assert.Contains(t, output, "response.output_item.added")
	assert.Contains(t, output, "response.content_part.added")
	assert.Contains(t, output, "response.completed")
}

func TestEmitCompleted_Idempotent(t *testing.T) {
	w := &testSSEWriter{}
	state := &AnthropicToResponsesState{Model: "test", CreatedSent: true}

	EmitCompleted(w, state)
	assert.True(t, state.CompletedSent)
	first := w.buf.String()
	assert.Contains(t, first, "response.completed")

	// Second call should not emit anything
	w.buf.Reset()
	EmitCompleted(w, state)
	assert.Empty(t, w.buf.String())
}

func TestConvertAnthropicLineToResponses_MessageStopAfterMessageDelta(t *testing.T) {
	w := &testSSEWriter{}
	state := &AnthropicToResponsesState{Model: "test"}

	// message_delta emits response.completed
	ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_start","message":{"id":"msg_1","model":"test","usage":{"input_tokens":5}}}`)
	w.buf.Reset()

	done := ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`)
	assert.True(t, done)
	assert.True(t, state.CompletedSent)

	w.buf.Reset()

	// message_stop after completion should not emit again
	done = ConvertAnthropicLineToResponses(w, state, `data: {"type":"message_stop"}`)
	assert.True(t, done)
	assert.Empty(t, w.buf.String())
}
