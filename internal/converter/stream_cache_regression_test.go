package converter

import (
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findMessageDelta(w *mockSSEWriter) *sseEvent {
	for i := range w.events {
		if w.events[i].eventType == "message_delta" {
			return &w.events[i]
		}
	}
	return nil
}

// Regression: Chat→Anthropic must surface cache_read_input_tokens in
// message_delta.usage so Claude Code can track context utilization and trigger
// auto-compact. Previously only input_tokens/output_tokens were emitted and the
// cached portion was double-counted under input_tokens.
func TestChatToAnthropic_MessageDeltaUsageIncludesCacheTokens(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	ConvertChatChunkToAnthropicSSE(w, state, `{"id":"1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`)
	ConvertChatChunkToAnthropicSSE(w, state, `{"id":"1","model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}`)
	ConvertChatChunkToAnthropicSSE(w, state, `{"id":"1","model":"m","choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":50,"prompt_tokens_details":{"cached_tokens":800}}}`)

	delta := findMessageDelta(w)
	require.NotNil(t, delta)
	data := delta.data.(map[string]any)
	usage := data["usage"].(map[string]any)
	// input = prompt(1000) - cache_read(800) = 200 (fresh, excludes cached)
	assert.Equal(t, 200, usage["input_tokens"], "input_tokens should exclude cached portion")
	assert.Equal(t, 50, usage["output_tokens"])
	assert.Equal(t, 800, usage["cache_read_input_tokens"])
}

// Regression: Chat→Anthropic without cache must still report plain usage.
func TestChatToAnthropic_MessageDeltaUsageWithoutCache(t *testing.T) {
	w := &mockSSEWriter{}
	state := &AnthropicStreamState{}

	ConvertChatChunkToAnthropicSSE(w, state, `{"id":"1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`)
	ConvertChatChunkToAnthropicSSE(w, state, `{"id":"1","model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}`)
	ConvertChatChunkToAnthropicSSE(w, state, `{"id":"1","model":"m","choices":[],"usage":{"prompt_tokens":300,"completion_tokens":20}}`)

	delta := findMessageDelta(w)
	require.NotNil(t, delta)
	data := delta.data.(map[string]any)
	usage := data["usage"].(map[string]any)
	assert.Equal(t, 300, usage["input_tokens"])
	assert.Equal(t, 20, usage["output_tokens"])
	_, hasCacheRead := usage["cache_read_input_tokens"]
	assert.False(t, hasCacheRead, "cache_read should be absent when upstream reports none")
}

// Regression: Responses→Anthropic must surface cache_read_input_tokens in
// message_delta.usage, resolved from response.usage.input_tokens_details.cached_tokens.
func TestResponsesToAnthropic_MessageDeltaUsageIncludesCacheTokens(t *testing.T) {
	w := &mockSSEWriter{}
	state := &ResponsesToAnthropicState{MessageID: "resp_1", Model: "m"}

	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.output_text.delta", map[string]any{
		"delta":   "hi",
		"item_id": "i1",
	}))
	ConvertResponsesEventToAnthropicSSE(w, state, buildResponsesJSON("response.completed", map[string]any{
		"response": map[string]any{
			"id":     "resp_1",
			"status": "completed",
			"usage": map[string]any{
				"input_tokens":         1000,
				"output_tokens":        50,
				"input_tokens_details": map[string]any{"cached_tokens": 800},
			},
		},
	}))

	delta := findMessageDelta(w)
	require.NotNil(t, delta)
	data := delta.data.(map[string]any)
	usage := data["usage"].(map[string]any)
	assert.Equal(t, 200, usage["input_tokens"], "input_tokens should exclude cached portion")
	assert.Equal(t, 50, usage["output_tokens"])
	assert.Equal(t, 800, usage["cache_read_input_tokens"])
}

// Regression: Gemini→Anthropic must include input_tokens in message_delta.usage
// (previously only output_tokens), so context utilization is non-zero.
func TestGeminiToAnthropic_MessageDeltaUsageIncludesInputTokens(t *testing.T) {
	w := &mockSSEWriter{}
	state := &GeminiToAnthropicState{Model: "gemini-1.5-pro"}

	ConvertGeminiLineToAnthropicSSE(w, state, `{"candidates":[{"content":{"parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1500,"candidatesTokenCount":40}}`)

	delta := findMessageDelta(w)
	require.NotNil(t, delta)
	data := delta.data.(map[string]any)
	usage := data["usage"].(map[string]any)
	assert.Equal(t, 1500, usage["input_tokens"])
	assert.Equal(t, 40, usage["output_tokens"])
}

// Regression: non-streaming ChatToAnthropic must populate
// cache_read_input_tokens (and exclude the cached portion from input_tokens)
// so Claude Code's context meter stays accurate for stream=false responses.
// Previously only input_tokens/output_tokens were set, with prompt_tokens
// (inclusive of cache) double-counted under input_tokens.
func TestChatToAnthropic_NonStreamUsageIncludesCacheTokens(t *testing.T) {
	c := &Converter{}
	chatResp := &types.ChatResponse{
		ID: "1",
		Usage: types.ChatUsage{
			PromptTokens:        1000,
			CompletionTokens:    50,
			PromptTokensDetails: &types.PromptTokensDetails{CachedTokens: 800},
		},
	}
	resp, err := c.ChatToAnthropic(chatResp, "m", "")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Usage.InputTokens, "input_tokens should exclude cached portion")
	assert.Equal(t, 50, resp.Usage.OutputTokens)
	assert.Equal(t, 800, resp.Usage.CacheReadInputTokens)
}

// Regression: non-streaming ChatToAnthropic without cache must still produce
// correct input_tokens (full prompt) and omit the cache field.
func TestChatToAnthropic_NonStreamUsageWithoutCache(t *testing.T) {
	c := &Converter{}
	chatResp := &types.ChatResponse{
		ID: "1",
		Usage: types.ChatUsage{
			PromptTokens:     300,
			CompletionTokens: 20,
		},
	}
	resp, err := c.ChatToAnthropic(chatResp, "m", "")
	require.NoError(t, err)
	assert.Equal(t, 300, resp.Usage.InputTokens)
	assert.Equal(t, 20, resp.Usage.OutputTokens)
	assert.Equal(t, 0, resp.Usage.CacheReadInputTokens)
}
