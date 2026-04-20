package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/keepmind9/llm-gateway/internal/types"
)

// ConvertChatChunkToResponsesSSE processes a single Chat SSE data line and emits
// corresponding Responses API SSE events. Returns true when stream is done.
func ConvertChatChunkToResponsesSSE(w SSEWriter, state *ResponsesStreamState, data string) bool {
	if data == "[DONE]" {
		if state.CreatedSent {
			emitResponseCompleted(w, state)
		}
		return true
	}

	var chunk types.ChatStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false
	}

	if !state.CreatedSent {
		state.CreatedSent = true
		state.ResponseID = chunk.ID
		w.WriteEvent("response.created", map[string]any{
			"type":            "response.created",
			"sequence_number": state.nextSeq(),
			"response": map[string]any{
				"id":         chunk.ID,
				"object":     "response",
				"created_at": state.Created,
				"model":      state.Model,
				"status":     "in_progress",
				"output":     []any{},
				"usage":      nil,
			},
		})
	}

	for _, choice := range chunk.Choices {
		if state.ItemID == "" {
			state.ItemID = fmt.Sprintf("item_%d", time.Now().UnixNano())
			state.OutputIndex = 0
			state.ContentIndex = 0

			w.WriteEvent("response.output_item.added", map[string]any{
				"type":            "response.output_item.added",
				"sequence_number": state.nextSeq(),
				"output_index":    0,
				"item": map[string]any{
					"id":      state.ItemID,
					"type":    "message",
					"status":  "in_progress",
					"role":    "assistant",
					"content": []any{},
				},
			})

			w.WriteEvent("response.content_part.added", map[string]any{
				"type":            "response.content_part.added",
				"sequence_number": state.nextSeq(),
				"output_index":    0,
				"content_index":   0,
				"item_id":         state.ItemID,
				"part": map[string]any{
					"type": "output_text",
					"text": "",
				},
			})
		}

		if choice.Delta.Content != "" {
			content := state.TagState.FilterChunk(choice.Delta.Content, state.ThinkTag)
			if content != "" {
				state.AccText += content
				w.WriteEvent("response.output_text.delta", map[string]any{
					"type":            "response.output_text.delta",
					"sequence_number": state.nextSeq(),
					"output_index":    state.OutputIndex,
					"content_index":   state.ContentIndex,
					"item_id":         state.ItemID,
					"delta":           content,
				})
			}
		}

		if choice.FinishReason != "" && choice.FinishReason != "null" {
			emitTextDone(w, state)
		}
	}

	return false
}

func emitTextDone(w SSEWriter, state *ResponsesStreamState) {
	w.WriteEvent("response.output_text.done", map[string]any{
		"type":            "response.output_text.done",
		"sequence_number": state.nextSeq(),
		"output_index":    state.OutputIndex,
		"content_index":   state.ContentIndex,
		"item_id":         state.ItemID,
		"text":            state.AccText,
	})

	w.WriteEvent("response.content_part.done", map[string]any{
		"type":            "response.content_part.done",
		"sequence_number": state.nextSeq(),
		"output_index":    state.OutputIndex,
		"content_index":   state.ContentIndex,
		"item_id":         state.ItemID,
		"part": map[string]any{
			"type": "output_text",
			"text": state.AccText,
		},
	})

	w.WriteEvent("response.output_item.done", map[string]any{
		"type":            "response.output_item.done",
		"sequence_number": state.nextSeq(),
		"output_index":    state.OutputIndex,
		"item": map[string]any{
			"id":     state.ItemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []map[string]any{
				{"type": "output_text", "text": state.AccText},
			},
		},
	})
}

func emitResponseCompleted(w SSEWriter, state *ResponsesStreamState) {
	w.WriteEvent("response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": state.nextSeq(),
		"response": map[string]any{
			"id":         state.ResponseID,
			"object":     "response",
			"created_at": state.Created,
			"model":      state.Model,
			"status":     "completed",
			"output": []map[string]any{
				{
					"id":     state.ItemID,
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": state.AccText},
					},
				},
			},
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
				"total_tokens":  0,
			},
		},
	})
}

// ParseSSEDataLine extracts the data portion from an SSE line.
// Returns empty string if not a data line.
func ParseSSEDataLine(line string) string {
	if strings.HasPrefix(line, "data: ") {
		return line[6:]
	}
	return ""
}

// FormatSSEEvent formats an SSE event string.
func FormatSSEEvent(eventType string, data []byte) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(data))
}
