package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func responsesEventJSON(eventType string, extra map[string]any) string {
	raw := map[string]any{"type": eventType}
	for k, v := range extra {
		raw[k] = v
	}
	data, _ := json.Marshal(raw)
	return "data: " + string(data)
}

func TestConvertResponsesLineToChat_ResponseCreated(t *testing.T) {
	state := &ResponsesToChatState{}
	line := responsesEventJSON("response.created", map[string]any{
		"response": map[string]any{
			"id":     "resp-001",
			"model":  "gpt-4o",
			"status": "in_progress",
		},
	})

	result := ConvertResponsesLineToChat(state, line)
	chunk, ok := result.(*types.ChatStreamResponse)
	assert.True(t, ok)
	assert.Equal(t, "resp-001", chunk.ID)
	assert.Equal(t, "assistant", chunk.Choices[0].Delta.Role)
	assert.Equal(t, "resp-001", state.ID)
	assert.Equal(t, "gpt-4o", state.Model)
	assert.True(t, state.Started)
}

func TestConvertResponsesLineToChat_FullStream(t *testing.T) {
	state := &ResponsesToChatState{}

	// response.created
	result := ConvertResponsesLineToChat(state, responsesEventJSON("response.created", map[string]any{
		"response": map[string]any{
			"id":     "resp-abc",
			"model":  "gpt-4o",
			"status": "in_progress",
		},
	}))
	assert.NotNil(t, result)
	assert.True(t, state.Started)

	// output_text.delta 1
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.output_text.delta", map[string]any{
		"delta": "Hello ",
	}))
	chunk, ok := result.(*types.ChatStreamResponse)
	assert.True(t, ok)
	assert.Equal(t, "Hello ", derefStr(chunk.Choices[0].Delta.Content))

	// output_text.delta 2
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.output_text.delta", map[string]any{
		"delta": "world",
	}))
	assert.NotNil(t, result)
	assert.Equal(t, "Hello world", state.AccText)

	// response.completed
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.completed", map[string]any{
		"response": map[string]any{
			"id":     "resp-abc",
			"status": "completed",
		},
	}))
	chunk, ok = result.(*types.ChatStreamResponse)
	assert.True(t, ok)
	assert.Equal(t, "stop", chunk.Choices[0].FinishReason)
}

func TestConvertResponsesLineToChat_SkipsEmptyLine(t *testing.T) {
	state := &ResponsesToChatState{}
	assert.Nil(t, ConvertResponsesLineToChat(state, ""))
}

func TestConvertResponsesLineToChat_SkipsNonDataLine(t *testing.T) {
	state := &ResponsesToChatState{}
	assert.Nil(t, ConvertResponsesLineToChat(state, "event: response.created"))
}

func TestConvertResponsesLineToChat_SkipsInvalidJSON(t *testing.T) {
	state := &ResponsesToChatState{}
	assert.Nil(t, ConvertResponsesLineToChat(state, "data: not-json"))
}

func TestConvertResponsesLineToChat_SkipsEmptyDelta(t *testing.T) {
	state := &ResponsesToChatState{Started: true}
	assert.Nil(t, ConvertResponsesLineToChat(state, responsesEventJSON("response.output_text.delta", map[string]any{
		"delta": "",
	})))
}

func TestConvertResponsesLineToChat_SkipsInternalEvents(t *testing.T) {
	state := &ResponsesToChatState{Started: true}

	internalEvents := []string{
		"response.output_item.done",
		"response.content_part.done",
		"response.output_text.done",
		"response.output_item.added",
		"response.content_part.added",
	}

	for _, event := range internalEvents {
		result := ConvertResponsesLineToChat(state, responsesEventJSON(event, nil))
		assert.Nil(t, result, "expected nil for event type: %s", event)
	}
}

func TestConvertResponsesLineToChat_SkipsUnknownEvent(t *testing.T) {
	state := &ResponsesToChatState{}
	assert.Nil(t, ConvertResponsesLineToChat(state, responsesEventJSON("unknown.event", nil)))
}

func TestConvertResponsesLineToChat_ResponseCreatedNoResponse(t *testing.T) {
	state := &ResponsesToChatState{}
	result := ConvertResponsesLineToChat(state, responsesEventJSON("response.created", nil))
	chunk, ok := result.(*types.ChatStreamResponse)
	assert.True(t, ok)
	assert.Equal(t, "", state.ID)
	assert.Equal(t, "", state.Model)
	assert.True(t, state.Started)
	assert.Equal(t, "assistant", chunk.Choices[0].Delta.Role)
}

func TestConvertResponsesLineToChat_ToolCallStream(t *testing.T) {
	state := &ResponsesToChatState{}

	// response.created
	result := ConvertResponsesLineToChat(state, responsesEventJSON("response.created", map[string]any{
		"response": map[string]any{"id": "resp-001", "model": "gpt-4o"},
	}))
	require.NotNil(t, result)
	assert.True(t, state.Started)

	// response.output_item.added (function_call)
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"id":      "fc_123_0",
			"type":    "function_call",
			"call_id": "call_abc",
			"name":    "get_weather",
		},
	}))
	chunk, ok := result.(*types.ChatStreamResponse)
	require.True(t, ok)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, "call_abc", chunk.Choices[0].Delta.ToolCalls[0].ID)
	assert.Equal(t, "get_weather", chunk.Choices[0].Delta.ToolCalls[0].Function.Name)
	assert.Equal(t, 0, chunk.Choices[0].Delta.ToolCalls[0].Index)
	assert.True(t, state.HasToolCalls)

	// response.function_call_arguments.delta
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.function_call_arguments.delta", map[string]any{
		"output_index": 0,
		"item_id":      "fc_123_0",
		"delta":        `{"city":`,
	}))
	chunk, ok = result.(*types.ChatStreamResponse)
	require.True(t, ok)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, `{"city":`, chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)

	// response.function_call_arguments.delta (second chunk)
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.function_call_arguments.delta", map[string]any{
		"output_index": 0,
		"item_id":      "fc_123_0",
		"delta":        `"NYC"}`,
	}))
	chunk, ok = result.(*types.ChatStreamResponse)
	require.True(t, ok)
	assert.Equal(t, `"NYC"}`, chunk.Choices[0].Delta.ToolCalls[0].Function.Arguments)

	// Verify accumulated args in state
	require.NotNil(t, state.ToolCallItems["fc_123_0"])
	assert.Equal(t, `{"city":"NYC"}`, state.ToolCallItems["fc_123_0"].Args)

	// response.function_call_arguments.done (should return nil)
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.function_call_arguments.done", map[string]any{
		"output_index": 0,
		"item_id":      "fc_123_0",
		"arguments":    `{"city":"NYC"}`,
	}))
	assert.Nil(t, result)

	// response.completed
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.completed", map[string]any{
		"response": map[string]any{"id": "resp-001", "status": "completed"},
	}))
	chunk, ok = result.(*types.ChatStreamResponse)
	require.True(t, ok)
	assert.Equal(t, "tool_calls", chunk.Choices[0].FinishReason)
}

func TestConvertResponsesLineToChat_MultipleToolCalls(t *testing.T) {
	state := &ResponsesToChatState{}

	// response.created
	ConvertResponsesLineToChat(state, responsesEventJSON("response.created", map[string]any{
		"response": map[string]any{"id": "resp-002", "model": "gpt-4o"},
	}))

	// First tool call added
	result := ConvertResponsesLineToChat(state, responsesEventJSON("response.output_item.added", map[string]any{
		"output_index": 0,
		"item": map[string]any{
			"id": "fc_1", "type": "function_call", "call_id": "call_a", "name": "get_weather",
		},
	}))
	chunk, _ := result.(*types.ChatStreamResponse)
	assert.Equal(t, 0, chunk.Choices[0].Delta.ToolCalls[0].Index)

	// Second tool call added
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.output_item.added", map[string]any{
		"output_index": 1,
		"item": map[string]any{
			"id": "fc_2", "type": "function_call", "call_id": "call_b", "name": "get_stock",
		},
	}))
	chunk, _ = result.(*types.ChatStreamResponse)
	assert.Equal(t, 1, chunk.Choices[0].Delta.ToolCalls[0].Index)

	// Arguments delta for first tool call
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.function_call_arguments.delta", map[string]any{
		"item_id": "fc_1", "delta": `{"city":"NYC"}`,
	}))
	chunk, _ = result.(*types.ChatStreamResponse)
	assert.Equal(t, 0, chunk.Choices[0].Delta.ToolCalls[0].Index)

	// Arguments delta for second tool call
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.function_call_arguments.delta", map[string]any{
		"item_id": "fc_2", "delta": `{"sym":"AAPL"}`,
	}))
	chunk, _ = result.(*types.ChatStreamResponse)
	assert.Equal(t, 1, chunk.Choices[0].Delta.ToolCalls[0].Index)

	// Completed
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.completed", map[string]any{
		"response": map[string]any{"status": "completed"},
	}))
	chunk, _ = result.(*types.ChatStreamResponse)
	assert.Equal(t, "tool_calls", chunk.Choices[0].FinishReason)
}

func TestConvertResponsesLineToChat_TextThenToolCall(t *testing.T) {
	state := &ResponsesToChatState{}

	// response.created
	ConvertResponsesLineToChat(state, responsesEventJSON("response.created", map[string]any{
		"response": map[string]any{"id": "resp-003", "model": "gpt-4o"},
	}))

	// Text delta
	result := ConvertResponsesLineToChat(state, responsesEventJSON("response.output_text.delta", map[string]any{
		"delta": "Let me check",
	}))
	chunk, _ := result.(*types.ChatStreamResponse)
	assert.Equal(t, "Let me check", derefStr(chunk.Choices[0].Delta.Content))

	// Tool call added
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.output_item.added", map[string]any{
		"item": map[string]any{
			"id": "fc_1", "type": "function_call", "call_id": "call_a", "name": "search",
		},
	}))
	chunk, _ = result.(*types.ChatStreamResponse)
	require.Len(t, chunk.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, "search", chunk.Choices[0].Delta.ToolCalls[0].Function.Name)

	// Completed with tool_calls finish reason
	result = ConvertResponsesLineToChat(state, responsesEventJSON("response.completed", map[string]any{
		"response": map[string]any{"status": "completed"},
	}))
	chunk, _ = result.(*types.ChatStreamResponse)
	assert.Equal(t, "tool_calls", chunk.Choices[0].FinishReason)
}

func TestConvertResponsesLineToChat_EmptyDeltaForToolArgs(t *testing.T) {
	state := &ResponsesToChatState{}
	ConvertResponsesLineToChat(state, responsesEventJSON("response.created", map[string]any{
		"response": map[string]any{"id": "resp-004", "model": "gpt-4o"},
	}))

	// Empty delta should return nil
	result := ConvertResponsesLineToChat(state, responsesEventJSON("response.function_call_arguments.delta", map[string]any{
		"item_id": "fc_1", "delta": "",
	}))
	assert.Nil(t, result)
}

func TestConvertResponsesLineToChat_ToolArgsWithoutItemID(t *testing.T) {
	state := &ResponsesToChatState{}
	ConvertResponsesLineToChat(state, responsesEventJSON("response.created", map[string]any{
		"response": map[string]any{"id": "resp-005", "model": "gpt-4o"},
	}))

	// Delta without item_id should return nil
	result := ConvertResponsesLineToChat(state, responsesEventJSON("response.function_call_arguments.delta", map[string]any{
		"delta": "some data",
	}))
	assert.Nil(t, result)
}
