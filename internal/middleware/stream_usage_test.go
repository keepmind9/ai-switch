package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamUsageAccumulator_AnthropicFormat(t *testing.T) {
	acc := StreamUsageAccumulator{}

	// Simulate Anthropic SSE stream: message_start → content deltas → message_delta
	acc.Sniff(`{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":200}}}`, "anthropic")
	assert.Equal(t, "claude-sonnet-4-5", acc.Model)
	assert.Equal(t, int64(200), acc.InputTokens)
	assert.Equal(t, int64(0), acc.OutputTokens)

	// Content deltas should not change usage
	acc.Sniff(`{"type":"content_block_delta","delta":{"text":"hello"}}`, "anthropic")
	assert.Equal(t, int64(200), acc.InputTokens)
	assert.Equal(t, int64(0), acc.OutputTokens)

	// message_delta provides output_tokens
	acc.Sniff(`{"type":"message_delta","usage":{"output_tokens":80}}`, "anthropic")
	assert.Equal(t, int64(80), acc.OutputTokens)
}

func TestStreamUsageAccumulator_ChatFormat(t *testing.T) {
	acc := StreamUsageAccumulator{}

	// Regular chunks without usage
	acc.Sniff(`{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"delta":{"content":"hi"}}]}`, "chat")
	assert.Equal(t, int64(0), acc.InputTokens)

	// Final chunk with usage
	acc.Sniff(`{"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`, "chat")
	assert.Equal(t, "gpt-4o", acc.Model)
	assert.Equal(t, int64(100), acc.InputTokens)
	assert.Equal(t, int64(50), acc.OutputTokens)
}

func TestStreamUsageAccumulator_ResponsesFormat(t *testing.T) {
	acc := StreamUsageAccumulator{}

	// Non-completed events should not set usage
	acc.Sniff(`{"type":"response.created","response":{"id":"resp_1","model":"test"}}`, "responses")
	assert.Equal(t, int64(0), acc.InputTokens)

	// response.completed provides full usage
	acc.Sniff(`{"type":"response.completed","response":{"id":"resp_1","model":"test-model","usage":{"input_tokens":50,"output_tokens":25,"total_tokens":75}}}`, "responses")
	assert.Equal(t, "test-model", acc.Model)
	assert.Equal(t, int64(50), acc.InputTokens)
	assert.Equal(t, int64(25), acc.OutputTokens)
}

func TestStreamUsageAccumulator_SkipsInvalid(t *testing.T) {
	acc := StreamUsageAccumulator{}

	acc.Sniff("", "anthropic")
	acc.Sniff("[DONE]", "anthropic")
	acc.Sniff("not json", "chat")

	assert.Equal(t, int64(0), acc.InputTokens)
	assert.Equal(t, int64(0), acc.OutputTokens)
	assert.Equal(t, "", acc.Model)
}

func TestStreamUsageAccumulator_DefaultFormat(t *testing.T) {
	acc := StreamUsageAccumulator{}

	// Empty format defaults to chat parsing
	acc.Sniff(`{"model":"gpt-4o","usage":{"prompt_tokens":30,"completion_tokens":10,"total_tokens":40}}`, "")
	assert.Equal(t, "gpt-4o", acc.Model)
	assert.Equal(t, int64(30), acc.InputTokens)
	assert.Equal(t, int64(10), acc.OutputTokens)
}
