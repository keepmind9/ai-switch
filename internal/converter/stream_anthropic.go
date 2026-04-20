package converter

import (
	"encoding/json"

	"github.com/keepmind9/llm-gateway/internal/types"
)

// ConvertChatChunkToAnthropicSSE processes a single Chat SSE data line and emits
// corresponding Anthropic Messages SSE events. Returns true when stream is done.
func ConvertChatChunkToAnthropicSSE(w SSEWriter, state *AnthropicStreamState, data string) bool {
	if data == "[DONE]" {
		if state.ContentSent {
			emitAnthropicMessageStop(w, state)
		}
		return true
	}

	var chunk types.ChatStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false
	}

	for _, choice := range chunk.Choices {
		// First chunk with role: emit message_start
		if !state.ContentSent && choice.Delta.Role == "assistant" {
			state.MessageID = chunk.ID
			state.Model = chunk.Model
			state.ContentSent = true

			w.WriteEvent("message_start", map[string]any{
				"type": "message_start",
				"message": map[string]any{
					"id":          chunk.ID,
					"type":        "message",
					"role":        "assistant",
					"content":     []any{},
					"model":       chunk.Model,
					"stop_reason": nil,
					"usage": map[string]any{
						"input_tokens":                state.InputTokens,
						"output_tokens":               0,
						"cache_creation_input_tokens": 0,
						"cache_read_input_tokens":     0,
					},
				},
			})

			// Emit content_block_start for first text block
			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": state.nextBlockIndex(),
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			})
			continue
		}

		// Content delta
		if choice.Delta.Content != "" {
			content := state.TagState.FilterChunk(choice.Delta.Content, state.ThinkTag)
			if content == "" {
				continue
			}
			if !state.ContentSent {
				state.ContentSent = true
				state.MessageID = chunk.ID
				state.Model = chunk.Model

				w.WriteEvent("message_start", map[string]any{
					"type": "message_start",
					"message": map[string]any{
						"id":          chunk.ID,
						"type":        "message",
						"role":        "assistant",
						"content":     []any{},
						"model":       chunk.Model,
						"stop_reason": nil,
						"usage": map[string]any{
							"input_tokens":  state.InputTokens,
							"output_tokens": 0,
						},
					},
				})

				w.WriteEvent("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": state.nextBlockIndex(),
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				})
			}

			state.AccText += content
			state.OutputTokens++

			w.WriteEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": content,
				},
			})
		}

		// Finish reason
		if choice.FinishReason != "" && choice.FinishReason != "null" {
			emitAnthropicStop(w, state, choice.FinishReason)
		}
	}

	return false
}

func emitAnthropicStop(w SSEWriter, state *AnthropicStreamState, finishReason string) {
	stopReason := chatStopToAnthropic(finishReason)

	// content_block_stop
	w.WriteEvent("content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})

	// message_delta with stop_reason
	w.WriteEvent("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason": stopReason,
		},
		"usage": map[string]any{
			"output_tokens": state.OutputTokens,
		},
	})
}

func emitAnthropicMessageStop(w SSEWriter, state *AnthropicStreamState) {
	w.WriteEvent("message_stop", map[string]any{
		"type": "message_stop",
	})
}
