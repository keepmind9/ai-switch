package converter

import (
	"encoding/json"
	"testing"

	"github.com/keepmind9/llm-gateway/internal/types"
	"github.com/stretchr/testify/assert"
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
	assert.Equal(t, "Hello ", chunk.Choices[0].Delta.Content)

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
