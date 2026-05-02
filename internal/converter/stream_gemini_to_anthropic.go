package converter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// GeminiToAnthropicState tracks state when converting Gemini SSE to Anthropic SSE.
type GeminiToAnthropicState struct {
	MessageID         string
	Model             string
	BlockIndex        int
	ContentSent       bool
	InputTokens       int
	OutputTokens      int
	CacheCreateTokens int
	CacheReadTokens   int
	ThinkTag          string
	TagState          ThinkTagState
	HasToolUse        bool
	AccText           string
}

// ConvertGeminiLineToAnthropicSSE processes a Gemini SSE line and emits Anthropic SSE.
// Returns true when stream is done.
func ConvertGeminiLineToAnthropicSSE(w SSEWriter, state *GeminiToAnthropicState, data string) bool {
	if data == "[DONE]" {
		// Gemini stream doesn't send [DONE], but we handle it for safety
		closeGeminiToAnthropicStream(w, state)
		return true
	}

	var gemResp GeminiResponse
	if err := json.Unmarshal([]byte(data), &gemResp); err != nil {
		return false
	}

	// Extract usage metadata
	if gemResp.UsageMetadata != nil {
		state.InputTokens = gemResp.UsageMetadata.PromptTokenCount
		state.OutputTokens = gemResp.UsageMetadata.CandidatesTokenCount
	}

	// No candidates — might be usage-only
	if len(gemResp.Candidates) == 0 || gemResp.Candidates[0].Content == nil {
		if gemResp.UsageMetadata != nil && state.ContentSent {
			closeGeminiToAnthropicStream(w, state)
			return true
		}
		return false
	}

	candidate := gemResp.Candidates[0]

	// Ensure message started
	if !state.ContentSent {
		state.ContentSent = true
		state.MessageID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
		w.WriteEvent("message_start", map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":      state.MessageID,
				"type":    "message",
				"role":    "assistant",
				"content": []any{},
				"model":   state.Model,
				"usage": map[string]any{
					"input_tokens":                0,
					"output_tokens":               0,
					"cache_creation_input_tokens": 0,
					"cache_read_input_tokens":     0,
				},
			},
		})
	}

	for _, part := range candidate.Content.Parts {
		// Text content
		if part.Text != "" {
			text := state.TagState.FilterChunk(part.Text, state.ThinkTag)
			if text == "" {
				continue
			}
			state.AccText += text

			idx := state.BlockIndex
			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]any{
					"type": "text",
					"text": "",
				},
			})

			w.WriteEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{
					"type": "text_delta",
					"text": text,
				},
			})

			w.WriteEvent("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": idx,
			})
			state.BlockIndex++
		}

		// Function call
		if part.FunctionCall != nil {
			state.HasToolUse = true
			idx := state.BlockIndex
			callID := fmt.Sprintf("toolu_%d", time.Now().UnixNano())
			argsJSON, _ := json.Marshal(part.FunctionCall.Args)

			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]any{
					"type": "tool_use",
					"id":   callID,
					"name": part.FunctionCall.Name,
				},
			})

			w.WriteEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": string(argsJSON),
				},
			})

			w.WriteEvent("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": idx,
			})
			state.BlockIndex++

			slog.Debug("gemini function call converted", "name", part.FunctionCall.Name, "call_id", callID)
		}
	}

	// Finish
	if candidate.FinishReason != "" {
		closeGeminiToAnthropicStream(w, state)
		return true
	}

	return false
}

func closeGeminiToAnthropicStream(w SSEWriter, state *GeminiToAnthropicState) {
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
