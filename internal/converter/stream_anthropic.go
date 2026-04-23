package converter

import (
	"encoding/json"
	"log/slog"

	"github.com/keepmind9/ai-switch/internal/types"
)

// ConvertChatChunkToAnthropicSSE processes a single Chat SSE data line and emits
// corresponding Anthropic Messages SSE events. Returns true when stream is done.
//
// message_start uses input_tokens: 0 as placeholder.
// message_delta is deferred until upstream usage arrives (or [DONE] as fallback),
// and includes both input_tokens and output_tokens from real upstream data.
func ConvertChatChunkToAnthropicSSE(w SSEWriter, state *AnthropicStreamState, data string) bool {
	if data == "[DONE]" {
		closeOpenToolBlocks(w, state)
		emitAnthropicFinalEvents(w, state)
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
						"input_tokens":                0,
						"output_tokens":               0,
						"cache_creation_input_tokens": 0,
						"cache_read_input_tokens":     0,
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
			continue
		}

		// Tool calls
		if len(choice.Delta.ToolCalls) > 0 {
			processToolCalls(w, state, choice.Delta.ToolCalls, chunk)
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
							"input_tokens":                0,
							"output_tokens":               0,
							"cache_creation_input_tokens": 0,
							"cache_read_input_tokens":     0,
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

		// Finish reason: close blocks, defer message_delta
		if choice.FinishReason != "" && choice.FinishReason != "null" {
			state.FinishReason = chatStopToAnthropic(choice.FinishReason)

			if !state.TextBlockClosed && state.ContentSent {
				state.TextBlockClosed = true
				w.WriteEvent("content_block_stop", map[string]any{
					"type":  "content_block_stop",
					"index": 0,
				})
			}
		}
	}

	// Usage chunk arrives after finish_reason: emit deferred message_delta + message_stop
	if chunk.Usage != nil {
		state.InputTokens = chunk.Usage.PromptTokens
		state.OutputTokens = chunk.Usage.CompletionTokens
		slog.Debug("anthropic stream: upstream usage",
			"prompt_tokens", chunk.Usage.PromptTokens,
			"completion_tokens", chunk.Usage.CompletionTokens,
		)
		emitAnthropicDeltaAndStop(w, state)
	}

	return false
}

func processToolCalls(w SSEWriter, state *AnthropicStreamState, toolCalls []types.ToolCall, chunk types.ChatStreamResponse) {
	if state.ToolBlocks == nil {
		state.ToolBlocks = make(map[int]*toolBlockState)
	}

	// Ensure message started
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
					"input_tokens":                0,
					"output_tokens":               0,
					"cache_creation_input_tokens": 0,
					"cache_read_input_tokens":     0,
				},
			},
		})
	}

	// Close text block if still open
	if !state.TextBlockClosed {
		state.TextBlockClosed = true
		w.WriteEvent("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": 0,
		})
	}

	for _, tc := range toolCalls {
		tbs, exists := state.ToolBlocks[tc.Index]
		if !exists {
			blockIdx := state.nextBlockIndex()
			tbs = &toolBlockState{
				AnthropicIndex: blockIdx,
				ID:             tc.ID,
				Name:           tc.Function.Name,
			}
			state.ToolBlocks[tc.Index] = tbs

			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": blockIdx,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": map[string]any{},
				},
			})
			tbs.Started = true
		}

		if tc.Function.Arguments != "" {
			w.WriteEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": tbs.AnthropicIndex,
				"delta": map[string]any{
					"type":         "input_json_delta",
					"partial_json": tc.Function.Arguments,
				},
			})
		}
	}
}

func closeOpenToolBlocks(w SSEWriter, state *AnthropicStreamState) {
	if state.ToolBlocks == nil {
		return
	}
	for _, tbs := range state.ToolBlocks {
		if tbs.Started && !tbs.Stopped {
			w.WriteEvent("content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": tbs.AnthropicIndex,
			})
			tbs.Stopped = true
		}
	}
}

// emitAnthropicDeltaAndStop emits message_delta (with real token counts) + message_stop.
func emitAnthropicDeltaAndStop(w SSEWriter, state *AnthropicStreamState) {
	if state.DeltaSent {
		return
	}
	state.DeltaSent = true

	closeOpenToolBlocks(w, state)

	w.WriteEvent("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason": state.FinishReason,
		},
		"usage": map[string]any{
			"input_tokens":  state.InputTokens,
			"output_tokens": state.OutputTokens,
		},
	})

	w.WriteEvent("message_stop", map[string]any{
		"type": "message_stop",
	})
}

// emitAnthropicFinalEvents handles [DONE]: emit deferred events if not yet sent.
func emitAnthropicFinalEvents(w SSEWriter, state *AnthropicStreamState) {
	if state.ContentSent {
		emitAnthropicDeltaAndStop(w, state)
	}
}
