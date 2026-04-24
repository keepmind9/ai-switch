package converter

import (
	"encoding/json"
)

// ResponsesToAnthropicState tracks state when converting Responses SSE to Anthropic SSE.
type ResponsesToAnthropicState struct {
	MessageID       string
	Model           string
	InputTokens     int
	OutputTokens    int
	AccText         string
	ContentSent     bool
	MessageStarted  bool
	CurrentBlockIdx int
	HasToolUse      bool
}

// ConvertResponsesEventToAnthropicSSE processes a raw Responses SSE data line and emits
// corresponding Anthropic Messages SSE events via the writer. Returns true when done.
func ConvertResponsesEventToAnthropicSSE(w SSEWriter, state *ResponsesToAnthropicState, data string) bool {
	if data == "[DONE]" {
		if state.MessageStarted {
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

	case "response.output_item.added":
		item, _ := raw["item"].(map[string]any)
		if item == nil {
			return false
		}
		itemType, _ := item["type"].(string)

		if itemType == "function_call" {
			if !state.MessageStarted {
				state.MessageStarted = true
				state.ContentSent = true
				state.ensureMessageStart(w)
			}

			callID, _ := item["call_id"].(string)
			name, _ := item["name"].(string)

			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": state.CurrentBlockIdx,
				"content_block": map[string]any{
					"type": "tool_use",
					"id":   callID,
					"name": name,
				},
			})
			state.CurrentBlockIdx++
			state.HasToolUse = true
		}

	case "response.function_call_arguments.delta":
		delta, _ := raw["delta"].(string)
		outputIndex := int(toFloat64(raw["output_index"]))
		w.WriteEvent("content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": outputIndex,
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": delta,
			},
		})

	case "response.function_call_arguments.done":
		// Arguments already streamed via delta events, nothing to do

	case "response.output_item.done":
		item, _ := raw["item"].(map[string]any)
		if item == nil {
			return false
		}
		itemType, _ := item["type"].(string)
		if itemType == "function_call" {
			outputIndex := int(toFloat64(raw["output_index"]))
			w.WriteEvent("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": outputIndex,
			})
		}

	case "response.output_text.delta":
		delta, _ := raw["delta"].(string)
		if delta == "" {
			return false
		}

		if !state.ContentSent {
			state.ContentSent = true
			state.ensureMessageStart(w)

			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": state.CurrentBlockIdx,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			})
			state.CurrentBlockIdx++
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
			if state.MessageStarted {
				// Close text block if it was opened
				if state.AccText != "" {
					w.WriteEvent("content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": 0,
					})
				}
			}

			stopReason := "end_turn"
			if state.HasToolUse {
				stopReason = "tool_use"
			}

			w.WriteEvent("message_delta", map[string]any{
				"type": "message_delta",
				"delta": map[string]any{
					"stop_reason": stopReason,
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

	case "response.output_text.done", "response.content_part.done",
		"response.content_part.added":
		return false
	}

	return false
}

func (s *ResponsesToAnthropicState) ensureMessageStart(w SSEWriter) {
	if s.MessageStarted {
		return
	}
	s.MessageStarted = true
	w.WriteEvent("message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":      s.MessageID,
			"type":    "message",
			"role":    "assistant",
			"content": []any{},
			"model":   s.Model,
			"usage": map[string]any{
				"input_tokens":  s.InputTokens,
				"output_tokens": 0,
			},
		},
	})
}
