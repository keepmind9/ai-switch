package converter

import (
	"encoding/json"
	"fmt"
	"time"
)

// GeminiToResponsesState tracks state when converting Gemini SSE to Responses SSE.
type GeminiToResponsesState struct {
	ResponseID    string
	ItemID        string
	Model         string
	Created       int64
	SeqNum        int
	CreatedSent   bool
	ItemSent      bool
	InputTokens   int
	OutputTokens  int
	AccText       string
	ThinkTag      string
	TagState      ThinkTagState
	TextOutputIdx int
	FuncOutputIdx int
}

func (s *GeminiToResponsesState) nextSeq() int {
	s.SeqNum++
	return s.SeqNum
}

// ConvertGeminiLineToResponsesSSE processes a Gemini SSE line and emits Responses SSE.
// Returns true when stream is done.
func ConvertGeminiLineToResponsesSSE(w SSEWriter, state *GeminiToResponsesState, data string) bool {
	if data == "[DONE]" {
		if state.CreatedSent {
			emitResponseCompletedFromGemini(w, state)
		}
		return true
	}

	var gemResp GeminiResponse
	if err := json.Unmarshal([]byte(data), &gemResp); err != nil {
		return false
	}

	// Extract usage
	if gemResp.UsageMetadata != nil {
		state.InputTokens = gemResp.UsageMetadata.PromptTokenCount
		state.OutputTokens = gemResp.UsageMetadata.CandidatesTokenCount
	}

	// No candidates
	if len(gemResp.Candidates) == 0 || gemResp.Candidates[0].Content == nil {
		if gemResp.UsageMetadata != nil && state.CreatedSent {
			emitResponseCompletedFromGemini(w, state)
			return true
		}
		return false
	}

	candidate := gemResp.Candidates[0]

	// Emit response.created on first data
	if !state.CreatedSent {
		state.CreatedSent = true
		state.Created = time.Now().Unix()
		state.ResponseID = fmt.Sprintf("resp_%d", time.Now().UnixNano())
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
			},
		})
	}

	for _, part := range candidate.Content.Parts {
		// Text
		if part.Text != "" {
			text := state.TagState.FilterChunk(part.Text, state.ThinkTag)
			if text == "" {
				continue
			}
			state.AccText += text

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
					"part":            map[string]any{"type": "output_text", "text": ""},
				})
			}

			w.WriteEvent("response.output_text.delta", map[string]any{
				"type":            "response.output_text.delta",
				"sequence_number": state.nextSeq(),
				"output_index":    0,
				"content_index":   0,
				"item_id":         state.ItemID,
				"delta":           text,
			})
		}

		// Function call
		if part.FunctionCall != nil {
			idx := state.FuncOutputIdx
			state.FuncOutputIdx++
			callID := fmt.Sprintf("fc_%d", time.Now().UnixNano())
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)

			w.WriteEvent("response.output_item.added", map[string]any{
				"type":            "response.output_item.added",
				"sequence_number": state.nextSeq(),
				"output_index":    idx,
				"item": map[string]any{
					"id":        callID,
					"type":      "function_call",
					"status":    "in_progress",
					"call_id":   callID,
					"name":      part.FunctionCall.Name,
					"arguments": "",
				},
			})

			if string(argsJSON) != "" {
				w.WriteEvent("response.function_call_arguments.delta", map[string]any{
					"type":            "response.function_call_arguments.delta",
					"sequence_number": state.nextSeq(),
					"output_index":    idx,
					"item_id":         callID,
					"delta":           string(argsJSON),
				})
			}

			w.WriteEvent("response.output_item.done", map[string]any{
				"type":            "response.output_item.done",
				"sequence_number": state.nextSeq(),
				"output_index":    idx,
				"item": map[string]any{
					"id":        callID,
					"type":      "function_call",
					"status":    "completed",
					"call_id":   callID,
					"name":      part.FunctionCall.Name,
					"arguments": string(argsJSON),
				},
			})
		}
	}

	// Finish
	if candidate.FinishReason != "" {
		// Close text item if open
		if state.ItemSent {
			w.WriteEvent("response.output_text.done", map[string]any{
				"type":            "response.output_text.done",
				"sequence_number": state.nextSeq(),
				"output_index":    state.TextOutputIdx,
				"content_index":   0,
				"item_id":         state.ItemID,
				"text":            state.AccText,
			})
			w.WriteEvent("response.content_part.done", map[string]any{
				"type":            "response.content_part.done",
				"sequence_number": state.nextSeq(),
				"output_index":    state.TextOutputIdx,
				"content_index":   0,
				"item_id":         state.ItemID,
				"part":            map[string]any{"type": "output_text", "text": state.AccText},
			})
			w.WriteEvent("response.output_item.done", map[string]any{
				"type":            "response.output_item.done",
				"sequence_number": state.nextSeq(),
				"output_index":    state.TextOutputIdx,
				"item": map[string]any{
					"id":      state.ItemID,
					"type":    "message",
					"status":  "completed",
					"role":    "assistant",
					"content": []map[string]any{{"type": "output_text", "text": state.AccText}},
				},
			})
		}

		emitResponseCompletedFromGemini(w, state)
		return true
	}

	return false
}

func emitResponseCompletedFromGemini(w SSEWriter, state *GeminiToResponsesState) {
	w.WriteEvent("response.completed", map[string]any{
		"type":            "response.completed",
		"sequence_number": state.nextSeq(),
		"response": map[string]any{
			"id":         state.ResponseID,
			"object":     "response",
			"created_at": state.Created,
			"model":      state.Model,
			"status":     "completed",
			"usage": map[string]any{
				"input_tokens":  state.InputTokens,
				"output_tokens": state.OutputTokens,
				"total_tokens":  state.InputTokens + state.OutputTokens,
			},
		},
	})
}
