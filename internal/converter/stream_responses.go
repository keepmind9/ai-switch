package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

// ConvertChatChunkToResponsesSSE processes a single Chat SSE data line and emits
// corresponding Responses API SSE events. Returns true when stream is done.
func ConvertChatChunkToResponsesSSE(w SSEWriter, state *ResponsesStreamState, data string) bool {
	if data == "[DONE]" {
		if state.CreatedSent {
			emitToolCallsDone(w, state)
			emitResponseCompleted(w, state)
		}
		return true
	}

	var chunk types.ChatStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false
	}

	if state.ToolCalls == nil {
		state.ToolCalls = make(map[int]*chatToolCallState)
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
		// Handle tool calls
		if len(choice.Delta.ToolCalls) > 0 {
			processChatToolCalls(w, state, choice.Delta.ToolCalls)
			continue
		}

		// Handle text content
		if choice.Delta.Content != "" {
			if state.ItemID == "" {
				state.ItemID = fmt.Sprintf("item_%d", time.Now().UnixNano())
				state.TextItemID = state.ItemID
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
			if choice.FinishReason == "tool_calls" {
				// Close text item if any text was sent
				if state.TextItemID != "" && !state.TextDoneSent {
					emitTextDone(w, state)
				}
				// Tool call done events will be emitted at [DONE] or here
				emitToolCallsDone(w, state)
			} else {
				emitTextDone(w, state)
			}
		}
	}

	// Capture usage from final chunk
	if chunk.Usage != nil {
		state.InputTokens = chunk.Usage.PromptTokens
		state.OutputTokens = chunk.Usage.CompletionTokens
	}

	return false
}

// processChatToolCalls handles tool_calls from a Chat SSE chunk.
func processChatToolCalls(w SSEWriter, state *ResponsesStreamState, toolCalls []types.ToolCall) {
	for _, tc := range toolCalls {
		idx := tc.Index
		if idx == 0 && tc.ID != "" {
			// Check if index 0 was already used
			if _, exists := state.ToolCalls[0]; exists && tc.ID == "" {
				idx = len(state.ToolCalls)
			}
		}

		entry, exists := state.ToolCalls[idx]
		if !exists {
			// New tool call
			entry = &chatToolCallState{
				ID:     tc.ID,
				Name:   tc.Function.Name,
				ItemID: fmt.Sprintf("fc_%d_%d", time.Now().UnixNano(), idx),
			}
			state.ToolCalls[idx] = entry

			state.FuncOutputIdx++
			outIdx := state.FuncOutputIdx

			w.WriteEvent("response.output_item.added", map[string]any{
				"type":            "response.output_item.added",
				"sequence_number": state.nextSeq(),
				"output_index":    outIdx,
				"item": map[string]any{
					"id":        entry.ItemID,
					"type":      "function_call",
					"status":    "in_progress",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": "",
				},
			})
		}

		// Append arguments delta
		if tc.Function.Arguments != "" {
			entry.Args += tc.Function.Arguments
			w.WriteEvent("response.function_call_arguments.delta", map[string]any{
				"type":            "response.function_call_arguments.delta",
				"sequence_number": state.nextSeq(),
				"output_index":    state.FuncOutputIdx,
				"item_id":         entry.ItemID,
				"delta":           tc.Function.Arguments,
			})
		}
	}
}

// emitToolCallsDone emits done events for all tracked tool calls.
func emitToolCallsDone(w SSEWriter, state *ResponsesStreamState) {
	for idx, tc := range state.ToolCalls {
		outIdx := idx + 1
		if outIdx <= state.FuncOutputIdx {
			outIdx = state.FuncOutputIdx - len(state.ToolCalls) + idx + 1
		}

		w.WriteEvent("response.function_call_arguments.done", map[string]any{
			"type":            "response.function_call_arguments.done",
			"sequence_number": state.nextSeq(),
			"output_index":    outIdx,
			"item_id":         tc.ItemID,
			"arguments":       tc.Args,
		})
		w.WriteEvent("response.output_item.done", map[string]any{
			"type":            "response.output_item.done",
			"sequence_number": state.nextSeq(),
			"output_index":    outIdx,
			"item": map[string]any{
				"id":        tc.ItemID,
				"type":      "function_call",
				"status":    "completed",
				"call_id":   tc.ID,
				"name":      tc.Name,
				"arguments": tc.Args,
			},
		})
	}
	// Clear to avoid re-emitting
	state.ToolCalls = make(map[int]*chatToolCallState)
}

func emitTextDone(w SSEWriter, state *ResponsesStreamState) {
	if state.TextDoneSent {
		return
	}
	state.TextDoneSent = true

	itemID := state.ItemID
	if state.TextItemID != "" {
		itemID = state.TextItemID
	}
	if itemID == "" {
		return
	}

	w.WriteEvent("response.output_text.done", map[string]any{
		"type":            "response.output_text.done",
		"sequence_number": state.nextSeq(),
		"output_index":    state.OutputIndex,
		"content_index":   state.ContentIndex,
		"item_id":         itemID,
		"text":            state.AccText,
	})

	w.WriteEvent("response.content_part.done", map[string]any{
		"type":            "response.content_part.done",
		"sequence_number": state.nextSeq(),
		"output_index":    state.OutputIndex,
		"content_index":   state.ContentIndex,
		"item_id":         itemID,
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
			"id":     itemID,
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
	var output []map[string]any

	// Message item with text
	if state.AccText != "" || (state.ItemID != "" && len(state.ToolCalls) == 0) {
		itemID := state.ItemID
		if state.TextItemID != "" {
			itemID = state.TextItemID
		}
		if itemID == "" {
			itemID = fmt.Sprintf("item_%d", time.Now().UnixNano())
		}
		output = append(output, map[string]any{
			"id":     itemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []map[string]any{
				{"type": "output_text", "text": state.AccText},
			},
		})
	}

	w.WriteEvent("response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": state.nextSeq(),
		"response": map[string]any{
			"id":         state.ResponseID,
			"object":     "response",
			"created_at": state.Created,
			"model":      state.Model,
			"status":     "completed",
			"output":     output,
			"usage": map[string]any{
				"input_tokens":  state.InputTokens,
				"output_tokens": state.OutputTokens,
				"total_tokens":  state.InputTokens + state.OutputTokens,
			},
		},
	})
}

// ParseSSEDataLine extracts the data portion from an SSE line.
// Returns empty string if not a data line.
func ParseSSEDataLine(line string) string {
	after, ok := strings.CutPrefix(line, "data:")
	if !ok {
		return ""
	}
	// Trim single leading space per SSE spec, but also handle "data:value" without space
	if len(after) > 0 && after[0] == ' ' {
		return after[1:]
	}
	return after
}

// FormatSSEEvent formats an SSE event string.
func FormatSSEEvent(eventType string, data []byte) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, string(data))
}
