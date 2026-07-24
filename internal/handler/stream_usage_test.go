package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// sniff operates on []byte (the zero-copy scanner path). The pre-filter contract
// is unchanged from the previous string version: lines without "usage" must
// short-circuit without allocation, while lines containing usage must still be
// parsed and extracted exactly.

// TestStreamUsageSniff_SkipsLinesWithoutUsageKeyword: a content-block delta has
// no usage. The pre-filter must reject it before json.Unmarshal, so the call
// performs zero heap allocations (the performance gate for this optimization).
func TestStreamUsageSniff_SkipsLinesWithoutUsageKeyword(t *testing.T) {
	var acc streamUsageAccumulator
	delta := []byte(`{"type":"content_block_delta","delta":{"text":"hello world"}}`)

	allocs := testing.AllocsPerRun(200, func() {
		acc.sniff(delta, "anthropic")
	})

	assert.Equal(t, 0.0, allocs, "lines without 'usage' must short-circuit without allocation")
	assert.Equal(t, int64(0), acc.InputTokens)
	assert.Equal(t, int64(0), acc.OutputTokens)
}

// TestStreamUsageSniff_DoneLineIsFree: the sentinel "[DONE]" line must remain a
// zero-allocation fast path (checked before the pre-filter).
func TestStreamUsageSniff_DoneLineIsFree(t *testing.T) {
	var acc streamUsageAccumulator
	done := []byte("[DONE]")
	allocs := testing.AllocsPerRun(200, func() {
		acc.sniff(done, "anthropic")
	})
	assert.Equal(t, 0.0, allocs)
	assert.Equal(t, int64(0), acc.InputTokens)
}

// Regression guards: lines containing "usage" must still be parsed and extracted
// exactly as before.

func TestStreamUsageSniff_ExtractsAnthropicUsageWhenPresent(t *testing.T) {
	var acc streamUsageAccumulator
	acc.sniff([]byte(`{"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":3}}}`), "anthropic")
	assert.Equal(t, int64(10), acc.InputTokens)
	assert.Equal(t, int64(3), acc.CacheReadTokens)
}

func TestStreamUsageSniff_ExtractsAnthropicMessageDeltaUsage(t *testing.T) {
	var acc streamUsageAccumulator
	acc.sniff([]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`), "anthropic")
	assert.Equal(t, int64(5), acc.OutputTokens)
}

func TestStreamUsageSniff_ExtractsChatUsageWhenPresent(t *testing.T) {
	var acc streamUsageAccumulator
	acc.sniff([]byte(`{"usage":{"prompt_tokens":7,"completion_tokens":9}}`), "openai")
	assert.Equal(t, int64(7), acc.InputTokens)
	assert.Equal(t, int64(9), acc.OutputTokens)
}

func TestStreamUsageSniff_ExtractsResponsesUsageWhenPresent(t *testing.T) {
	var acc streamUsageAccumulator
	acc.sniff([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":4,"output_tokens":6,"cache_read_input_tokens":2}}}`), "responses")
	assert.Equal(t, int64(4), acc.InputTokens)
	assert.Equal(t, int64(6), acc.OutputTokens)
	assert.Equal(t, int64(2), acc.CacheReadTokens)
}
