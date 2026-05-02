package converter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/keepmind9/ai-switch/internal/types"
)

var (
	toolIDSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	toolIDCounter   uint64
)

// sanitizeToolID ensures the tool call ID conforms to Claude's tool_use.id regex ^[a-zA-Z0-9_-]+$.
func sanitizeToolID(id string) string {
	s := toolIDSanitizer.ReplaceAllString(id, "_")
	if s == "" {
		s = fmt.Sprintf("toolu_%d_%d", time.Now().UnixNano(), atomic.AddUint64(&toolIDCounter, 1))
	}
	return s
}

// ConvertChatChunkToAnthropicSSE processes a single Chat SSE data line and emits
// corresponding Anthropic Messages SSE events. Returns true when stream is done.
//
// message_start uses input_tokens: 0 as placeholder.
// message_delta is deferred until upstream usage arrives (or [DONE] as fallback),
// and includes both input_tokens and output_tokens from real upstream data.
//
// Handles DeepSeek reasoning_content by emitting Anthropic thinking blocks.
func ConvertChatChunkToAnthropicSSE(w SSEWriter, state *AnthropicStreamState, data string) bool {
	if data == "[DONE]" {
		closeReasoningBlock(w, state)
		closeOpenToolBlocks(w, state)
		emitAnthropicFinalEvents(w, state)
		return true
	}

	var chunk types.ChatStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		slog.Warn("failed to parse chat SSE chunk", "error", err, "data", data)
		return false
	}

	for _, choice := range chunk.Choices {
		// First chunk with role: emit message_start only (blocks start on content)
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
			continue
		}

		// Process content: tool_calls > reasoning > text (mutually exclusive per delta).
		// finish_reason is handled separately below so it is never skipped by continue.
		if len(choice.Delta.ToolCalls) > 0 {
			state.SawToolCall = true
			closeReasoningBlock(w, state)
			processToolCalls(w, state, choice.Delta.ToolCalls, chunk)
		} else if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
			ensureMessageStarted(w, state, chunk)
			if !state.ReasoningStarted {
				state.ReasoningStarted = true
				w.WriteEvent("content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": state.nextBlockIndex(),
					"content_block": map[string]any{
						"type":     "thinking",
						"thinking": "",
					},
				})
			}
			w.WriteEvent("content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type":     "thinking_delta",
					"thinking": *choice.Delta.ReasoningContent,
				},
			})
			state.OutputTokens++
		} else if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			content := state.TagState.FilterChunk(*choice.Delta.Content, state.ThinkTag)
			if content != "" {
				ensureMessageStarted(w, state, chunk)
				closeReasoningBlock(w, state)

				if !state.TextBlockClosed && !state.TextBlockStarted {
					state.TextBlockStarted = true
					state.TextBlockIdx = state.nextBlockIndex()
					w.WriteEvent("content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": state.TextBlockIdx,
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
					"index": state.TextBlockIdx,
					"delta": map[string]any{
						"type": "text_delta",
						"text": content,
					},
				})
			}
		}

		// Always check finish_reason (independent of content type)
		if choice.FinishReason != "" && choice.FinishReason != "null" {
			state.FinishReason = chatStopToAnthropic(choice.FinishReason)
			closeReasoningBlock(w, state)

			if !state.TextBlockClosed && state.TextBlockStarted {
				state.TextBlockClosed = true
				w.WriteEvent("content_block_stop", map[string]any{
					"type":  "content_block_stop",
					"index": state.TextBlockIdx,
				})
			}
		}
	}

	// Usage chunk arrives after finish_reason: emit deferred message_delta + message_stop
	if chunk.Usage != nil {
		state.InputTokens = chunk.Usage.PromptTokens
		state.OutputTokens = chunk.Usage.CompletionTokens
		if chunk.Usage.PromptTokensDetails != nil {
			state.CacheReadTokens = chunk.Usage.PromptTokensDetails.CachedTokens
		}
		if chunk.Usage.PromptCacheHitTokens > 0 {
			state.CacheReadTokens = chunk.Usage.PromptCacheHitTokens
		}
		slog.Debug("anthropic stream: upstream usage",
			"prompt_tokens", chunk.Usage.PromptTokens,
			"completion_tokens", chunk.Usage.CompletionTokens,
		)
		emitAnthropicDeltaAndStop(w, state)
	}

	return false
}

func ensureMessageStarted(w SSEWriter, state *AnthropicStreamState, chunk types.ChatStreamResponse) {
	if state.ContentSent {
		return
	}
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

func closeReasoningBlock(w SSEWriter, state *AnthropicStreamState) {
	if state.ReasoningStarted && !state.ReasoningClosed {
		state.ReasoningClosed = true
		w.WriteEvent("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": 0,
		})
	}
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
	if !state.TextBlockClosed && state.TextBlockStarted {
		state.TextBlockClosed = true
		w.WriteEvent("content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": state.TextBlockIdx,
		})
	}

	for _, tc := range toolCalls {
		tbs, exists := state.ToolBlocks[tc.Index]
		if !exists {
			blockIdx := state.nextBlockIndex()
			sanitizedID := sanitizeToolID(tc.ID)
			tbs = &toolBlockState{
				AnthropicIndex: blockIdx,
				ID:             sanitizedID,
				Name:           tc.Function.Name,
			}
			state.ToolBlocks[tc.Index] = tbs

			w.WriteEvent("content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": blockIdx,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    sanitizedID,
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
// Overrides stop_reason to "tool_use" when tool calls were seen, regardless of what
// the upstream model sent (some models send "stop" instead of "tool_calls").
func emitAnthropicDeltaAndStop(w SSEWriter, state *AnthropicStreamState) {
	if state.DeltaSent {
		return
	}
	state.DeltaSent = true

	closeOpenToolBlocks(w, state)

	stopReason := state.FinishReason
	if state.SawToolCall {
		stopReason = "tool_use"
	} else if stopReason == "" {
		stopReason = "end_turn"
	}

	w.WriteEvent("message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason": stopReason,
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
