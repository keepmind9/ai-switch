package converter

import (
	"encoding/json"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

// ResponsesToChatState tracks state when converting Responses SSE to Chat SSE.
type ResponsesToChatState struct {
	ID           string
	Model        string
	Created      int64
	AccText      string
	Started      bool
	InputTokens  int
	OutputTokens int
}

// ConvertResponsesLineToChat processes a raw Responses SSE line and returns
// a ChatStreamResponse chunk, or nil if the line should be skipped.
// Returns the string "[DONE]" when the stream is complete.
func ConvertResponsesLineToChat(state *ResponsesToChatState, line string) any {
	if line == "" {
		return nil
	}

	data := ParseSSEDataLine(line)
	if data == "" {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return nil
	}

	eventType, _ := raw["type"].(string)

	switch eventType {
	case "response.created":
		resp, _ := raw["response"].(map[string]any)
		if resp != nil {
			state.ID, _ = resp["id"].(string)
			state.Model, _ = resp["model"].(string)
		}
		state.Created = time.Now().Unix()
		state.Started = true

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{Role: "assistant"}, FinishReason: ""},
			},
		}

	case "response.output_text.delta":
		delta, _ := raw["delta"].(string)
		if delta == "" {
			return nil
		}
		state.AccText += delta

		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{Content: delta}, FinishReason: ""},
			},
		}

	case "response.completed":
		if resp, ok := raw["response"].(map[string]any); ok {
			if usage, ok := resp["usage"].(map[string]any); ok {
				state.InputTokens = int(toFloat64(usage["input_tokens"]))
				state.OutputTokens = int(toFloat64(usage["output_tokens"]))
			}
		}
		return &types.ChatStreamResponse{
			ID:      state.ID,
			Object:  "chat.completion.chunk",
			Created: state.Created,
			Model:   state.Model,
			Choices: []types.StreamChoice{
				{Index: 0, Delta: types.ChatMessage{}, FinishReason: "stop"},
			},
		}

	case "response.output_item.done", "response.content_part.done",
		"response.output_text.done", "response.output_item.added",
		"response.content_part.added":
		// Internal state events, no Chat SSE equivalent
		return nil
	}

	return nil
}
