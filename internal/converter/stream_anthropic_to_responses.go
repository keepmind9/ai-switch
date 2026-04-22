package converter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AnthropicToResponsesState tracks state when converting Anthropic SSE to Responses SSE.
type AnthropicToResponsesState struct {
	ResponseID   string
	ItemID       string
	Model        string
	Created      int64
	AccText      string
	CreatedSent  bool
	ItemSent     bool
	SeqNum       int
	InputTokens  int
	OutputTokens int
	ThinkTag     string
	TagState     ThinkTagState
}

func (s *AnthropicToResponsesState) nextSeq() int {
	s.SeqNum++
	return s.SeqNum
}

// ConvertAnthropicLineToResponses processes a raw Anthropic SSE line and writes
// corresponding Responses API SSE events via the writer. Returns true when done.
func ConvertAnthropicLineToResponses(w SSEWriter, state *AnthropicToResponsesState, line string) bool {
	if strings.HasPrefix(line, "event: ") || line == "" {
		return false
	}

	data := ParseSSEDataLine(line)
	if data == "" {
		return false
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return false
	}

	eventType, _ := raw["type"].(string)

	switch eventType {
	case "message_start":
		msg, _ := raw["message"].(map[string]any)
		if msg != nil {
			state.ResponseID, _ = msg["id"].(string)
			state.Model, _ = msg["model"].(string)
			if usage, ok := msg["usage"].(map[string]any); ok {
				state.InputTokens = int(toFloat64(usage["input_tokens"]))
			}
		}
		state.Created = time.Now().Unix()
		state.CreatedSent = true

		w.WriteEvent("response.created", map[string]any{
			"type":            "response.created",
			"sequence_number": state.nextSeq(),
			"response": map[string]any{
				"id":         state.ResponseID,
				"object":     "response",
				"created_at": state.Created,
				"model":      state.Model,
				"status":     "in_progress",
				"output":     []any{},
				"usage":      nil,
			},
		})

	case "content_block_start":
		// Emit output_item.added and content_part.added on first content block
		if !state.ItemSent {
			state.ItemSent = true
			state.ItemID = fmt.Sprintf("item_%d", time.Now().UnixNano())

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

	case "content_block_delta":
		delta, _ := raw["delta"].(map[string]any)
		if delta == nil {
			return false
		}
		text, _ := delta["text"].(string)
		if text == "" {
			return false
		}
		content := state.TagState.FilterChunk(text, state.ThinkTag)
		if content == "" {
			return false
		}
		state.AccText += content
		state.OutputTokens++

		w.WriteEvent("response.output_text.delta", map[string]any{
			"type":            "response.output_text.delta",
			"sequence_number": state.nextSeq(),
			"output_index":    0,
			"content_index":   0,
			"item_id":         state.ItemID,
			"delta":           content,
		})

	case "message_delta":
		delta, _ := raw["delta"].(map[string]any)
		if delta != nil {
			_, _ = delta["stop_reason"].(string)
		}
		if usage, ok := raw["usage"].(map[string]any); ok {
			state.OutputTokens = int(toFloat64(usage["output_tokens"]))
			if in := int(toFloat64(usage["input_tokens"])); in > 0 {
				state.InputTokens = in
			}
		}

		// Emit text done events if we have accumulated text
		if state.AccText != "" {
			w.WriteEvent("response.output_text.done", map[string]any{
				"type":            "response.output_text.done",
				"sequence_number": state.nextSeq(),
				"output_index":    0,
				"content_index":   0,
				"item_id":         state.ItemID,
				"text":            state.AccText,
			})

			w.WriteEvent("response.content_part.done", map[string]any{
				"type":            "response.content_part.done",
				"sequence_number": state.nextSeq(),
				"output_index":    0,
				"content_index":   0,
				"item_id":         state.ItemID,
				"part": map[string]any{
					"type": "output_text",
					"text": state.AccText,
				},
			})

			w.WriteEvent("response.output_item.done", map[string]any{
				"type":            "response.output_item.done",
				"sequence_number": state.nextSeq(),
				"output_index":    0,
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
					"input_tokens":  state.InputTokens,
					"output_tokens": state.OutputTokens,
					"total_tokens":  state.InputTokens + state.OutputTokens,
				},
			},
		})
		return true

	case "message_stop":
		// Already handled in message_delta, but emit completed if not yet done
		if state.ItemSent {
			return true
		}
	}

	return false
}
