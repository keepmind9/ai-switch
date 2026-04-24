package converter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// AnthropicToResponsesState tracks state when converting Anthropic SSE to Responses SSE.
type AnthropicToResponsesState struct {
	ResponseID    string
	ItemID        string
	Model         string
	Created       int64
	AccText       string
	CreatedSent   bool
	ItemSent      bool
	CompletedSent bool
	SeqNum        int
	InputTokens   int
	OutputTokens  int
	ThinkTag      string
	TagState      ThinkTagState

	// Tool use tracking
	CurrentBlockType string
	CurrentBlockIdx  int
	FuncArgsBuf      map[int]*strings.Builder
	FuncNames        map[int]string
	FuncCallIDs      map[int]string
	TextItemID       string
	TextDoneSent     bool
	OutputIndex      int
}

func (s *AnthropicToResponsesState) NextSeq() int {
	s.SeqNum++
	return s.SeqNum
}

func (s *AnthropicToResponsesState) EnsureInit() {
	if s.FuncArgsBuf == nil {
		s.FuncArgsBuf = make(map[int]*strings.Builder)
	}
	if s.FuncNames == nil {
		s.FuncNames = make(map[int]string)
	}
	if s.FuncCallIDs == nil {
		s.FuncCallIDs = make(map[int]string)
	}
	if s.Created == 0 {
		s.Created = time.Now().Unix()
	}
}

// EmitCompleted emits all terminal Responses API events and marks the stream as done.
// Idempotent: safe to call multiple times — only emits once.
func EmitCompleted(w SSEWriter, state *AnthropicToResponsesState) {
	if state.CompletedSent {
		return
	}
	state.CompletedSent = true
	state.EnsureInit()

	if state.ItemID == "" {
		state.ItemID = fmt.Sprintf("item_%d", time.Now().UnixNano())
	}

	// Close any open tool_use blocks
	for idx := range state.FuncCallIDs {
		if buf, ok := state.FuncArgsBuf[idx]; ok && buf.Len() > 0 {
			// Already emitted in content_block_stop, skip
		}
	}

	// Emit text item done events if we never saw content_block_stop
	if state.ItemSent && state.TextItemID != "" && !state.TextDoneSent {
		w.WriteEvent("response.output_text.done", map[string]any{
			"type":            "response.output_text.done",
			"sequence_number": state.NextSeq(),
			"output_index":    0,
			"content_index":   0,
			"item_id":         state.TextItemID,
			"text":            state.AccText,
		})
		w.WriteEvent("response.content_part.done", map[string]any{
			"type":            "response.content_part.done",
			"sequence_number": state.NextSeq(),
			"output_index":    0,
			"content_index":   0,
			"item_id":         state.TextItemID,
			"part": map[string]any{
				"type": "output_text",
				"text": state.AccText,
			},
		})
		w.WriteEvent("response.output_item.done", map[string]any{
			"type":            "response.output_item.done",
			"sequence_number": state.NextSeq(),
			"output_index":    0,
			"item": map[string]any{
				"id":     state.TextItemID,
				"type":   "message",
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]any{
					{"type": "output_text", "text": state.AccText},
				},
			},
		})
	}

	// Build output array for response.completed
	var output []map[string]any

	// Message item with text
	if state.AccText != "" || !state.ItemSent {
		itemID := state.TextItemID
		if itemID == "" {
			itemID = state.ItemID
		}

		if !state.ItemSent {
			state.ItemSent = true
			w.WriteEvent("response.output_item.added", map[string]any{
				"type":            "response.output_item.added",
				"sequence_number": state.NextSeq(),
				"output_index":    state.OutputIndex,
				"item": map[string]any{
					"id":      itemID,
					"type":    "message",
					"status":  "in_progress",
					"role":    "assistant",
					"content": []any{},
				},
			})
			w.WriteEvent("response.content_part.added", map[string]any{
				"type":            "response.content_part.added",
				"sequence_number": state.NextSeq(),
				"output_index":    state.OutputIndex,
				"content_index":   0,
				"item_id":         itemID,
				"part": map[string]any{
					"type": "output_text",
					"text": "",
				},
			})
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

	// Function call items
	for idx := 0; idx < len(state.FuncCallIDs); idx++ {
		callID, ok := state.FuncCallIDs[idx]
		if !ok {
			continue
		}
		args := ""
		if buf, ok := state.FuncArgsBuf[idx]; ok {
			args = buf.String()
		}
		name := state.FuncNames[idx]
		output = append(output, map[string]any{
			"id":        fmt.Sprintf("fc_%s", callID),
			"type":      "function_call",
			"status":    "completed",
			"arguments": args,
			"call_id":   callID,
			"name":      name,
		})
	}

	w.WriteEvent("response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": state.NextSeq(),
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
	state.EnsureInit()

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
			"sequence_number": state.NextSeq(),
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
		index := int(toFloat64(raw["index"]))
		state.CurrentBlockIdx = index

		contentBlock, _ := raw["content_block"].(map[string]any)
		if contentBlock != nil {
			state.CurrentBlockType, _ = contentBlock["type"].(string)
		}

		switch state.CurrentBlockType {
		case "text":
			if !state.ItemSent {
				state.ItemSent = true
				state.TextItemID = fmt.Sprintf("item_%d", time.Now().UnixNano())

				w.WriteEvent("response.output_item.added", map[string]any{
					"type":            "response.output_item.added",
					"sequence_number": state.NextSeq(),
					"output_index":    state.OutputIndex,
					"item": map[string]any{
						"id":      state.TextItemID,
						"type":    "message",
						"status":  "in_progress",
						"role":    "assistant",
						"content": []any{},
					},
				})
				w.WriteEvent("response.content_part.added", map[string]any{
					"type":            "response.content_part.added",
					"sequence_number": state.NextSeq(),
					"output_index":    state.OutputIndex,
					"content_index":   0,
					"item_id":         state.TextItemID,
					"part": map[string]any{
						"type": "output_text",
						"text": "",
					},
				})
			}

		case "tool_use":
			id, _ := contentBlock["id"].(string)
			name, _ := contentBlock["name"].(string)

			state.FuncCallIDs[index] = id
			state.FuncNames[index] = ""
			if name != "" {
				state.FuncNames[index] = name
			}
			state.FuncArgsBuf[index] = &strings.Builder{}

			state.OutputIndex++
			w.WriteEvent("response.output_item.added", map[string]any{
				"type":            "response.output_item.added",
				"sequence_number": state.NextSeq(),
				"output_index":    state.OutputIndex,
				"item": map[string]any{
					"id":        fmt.Sprintf("fc_%s", id),
					"type":      "function_call",
					"status":    "in_progress",
					"call_id":   id,
					"name":      name,
					"arguments": "",
				},
			})
		}

	case "content_block_delta":
		delta, _ := raw["delta"].(map[string]any)
		if delta == nil {
			return false
		}
		deltaType, _ := delta["type"].(string)
		index := int(toFloat64(raw["index"]))

		switch deltaType {
		case "text_delta":
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
				"sequence_number": state.NextSeq(),
				"output_index":    0,
				"content_index":   0,
				"item_id":         state.TextItemID,
				"delta":           content,
			})

		case "input_json_delta":
			partialJSON, _ := delta["partial_json"].(string)
			callID := state.FuncCallIDs[index]

			if buf, ok := state.FuncArgsBuf[index]; ok {
				buf.WriteString(partialJSON)
			}

			w.WriteEvent("response.function_call_arguments.delta", map[string]any{
				"type":            "response.function_call_arguments.delta",
				"sequence_number": state.NextSeq(),
				"output_index":    state.OutputIndex,
				"item_id":         fmt.Sprintf("fc_%s", callID),
				"delta":           partialJSON,
			})
		}

	case "content_block_stop":
		index := int(toFloat64(raw["index"]))

		if state.CurrentBlockType == "text" && state.TextItemID != "" {
			state.TextDoneSent = true
			w.WriteEvent("response.output_text.done", map[string]any{
				"type":            "response.output_text.done",
				"sequence_number": state.NextSeq(),
				"output_index":    0,
				"content_index":   0,
				"item_id":         state.TextItemID,
				"text":            state.AccText,
			})
			w.WriteEvent("response.content_part.done", map[string]any{
				"type":            "response.content_part.done",
				"sequence_number": state.NextSeq(),
				"output_index":    0,
				"content_index":   0,
				"item_id":         state.TextItemID,
				"part": map[string]any{
					"type": "output_text",
					"text": state.AccText,
				},
			})
			w.WriteEvent("response.output_item.done", map[string]any{
				"type":            "response.output_item.done",
				"sequence_number": state.NextSeq(),
				"output_index":    0,
				"item": map[string]any{
					"id":     state.TextItemID,
					"type":   "message",
					"status": "completed",
					"role":   "assistant",
					"content": []map[string]any{
						{"type": "output_text", "text": state.AccText},
					},
				},
			})
		}

		if state.CurrentBlockType == "tool_use" {
			callID := state.FuncCallIDs[index]
			args := ""
			if buf, ok := state.FuncArgsBuf[index]; ok {
				args = buf.String()
			}
			name := state.FuncNames[index]

			w.WriteEvent("response.function_call_arguments.done", map[string]any{
				"type":            "response.function_call_arguments.done",
				"sequence_number": state.NextSeq(),
				"output_index":    state.OutputIndex,
				"item_id":         fmt.Sprintf("fc_%s", callID),
				"arguments":       args,
			})
			w.WriteEvent("response.output_item.done", map[string]any{
				"type":            "response.output_item.done",
				"sequence_number": state.NextSeq(),
				"output_index":    state.OutputIndex,
				"item": map[string]any{
					"id":        fmt.Sprintf("fc_%s", callID),
					"type":      "function_call",
					"status":    "completed",
					"arguments": args,
					"call_id":   callID,
					"name":      name,
				},
			})
		}

		state.CurrentBlockType = ""

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
		EmitCompleted(w, state)
		return true

	case "message_stop":
		EmitCompleted(w, state)
		return true

	default:
		slog.Debug("anthropic→responses: unhandled event type", "type", eventType)
	}

	return false
}
