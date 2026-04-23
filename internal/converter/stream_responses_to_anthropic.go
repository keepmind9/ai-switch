package converter

import (
	"encoding/json"
)

// ResponsesToAnthropicState tracks state when converting Responses SSE to Anthropic SSE.
type ResponsesToAnthropicState struct {
	MessageID    string
	Model        string
	InputTokens  int
	OutputTokens int
	AccText      string
	ContentSent  bool
}

// ConvertResponsesEventToAnthropicSSE processes a raw Responses SSE data line and emits
// corresponding Anthropic Messages SSE events via the writer. Returns true when done.
func ConvertResponsesEventToAnthropicSSE(w SSEWriter, state *ResponsesToAnthropicState, data string) bool {
	if data == "[DONE]" {
		if state.ContentSent {
			w.WriteEvent("message_stop", map[string]any{
				"type": "message_stop",
			})
		}
		return true
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return false
	}

	eventType, _ := raw["type"].(string)

	switch eventType {
	case "response.created":
		resp, _ := raw["response"].(map[string]any)
		if resp != nil {
			state.MessageID, _ = resp["id"].(string)
			state.Model, _ = resp["model"].(string)
		}

	case "response.output_text.delta":
		delta, _ := raw["delta"].(string)
		if delta == "" {
			return false
		}

		if !state.ContentSent {
			state.ContentSent = true

			w.WriteEvent("message_start", map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id":      state.MessageID,
					"type":    "message",
					"role":    "assistant",
					"content": []any{},
					"model":   state.Model,
					"usage": map[string]any{
						"input_tokens":  state.InputTokens,
						"output_tokens": 0,
					},
				},
			})

			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			})
		}

		state.AccText += delta
		state.OutputTokens++

		w.WriteEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type": "text_delta",
				"text": delta,
			},
		})

	case "response.completed":
		if resp, ok := raw["response"].(map[string]any); ok {
			if usage, ok := resp["usage"].(map[string]any); ok {
				state.InputTokens = int(toFloat64(usage["input_tokens"]))
				state.OutputTokens = int(toFloat64(usage["output_tokens"]))
			}
		}

		if state.ContentSent {
			w.WriteEvent("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": 0,
			})

			w.WriteEvent("message_delta", map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason": "end_turn",
				},
				"usage": map[string]any{
					"output_tokens": state.OutputTokens,
				},
			})

			w.WriteEvent("message_stop", map[string]any{
				"type": "message_stop",
			})
		}
		return true

	case "response.output_item.done", "response.content_part.done",
		"response.output_text.done", "response.output_item.added",
		"response.content_part.added":
		return false
	}

	return false
}
