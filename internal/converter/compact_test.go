package converter

import (
	"testing"

	"github.com/keepmind9/ai-switch/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeCompactionPayload(t *testing.T) {
	payload := &types.CompactionPayload{
		Summary: "User asked to create a landing page. Assistant provided HTML/CSS code. User requested color change to blue.",
		Model:   "claude-sonnet-4-6",
		TS:      1746403200,
	}

	encoded, err := EncodeCompactionPayload(payload)
	assert.NoError(t, err)
	assert.True(t, IsFakeCompaction(encoded))

	decoded, err := DecodeCompactionPayload(encoded)
	assert.NoError(t, err)
	assert.Equal(t, payload.Summary, decoded.Summary)
	assert.Equal(t, payload.Model, decoded.Model)
	assert.Equal(t, payload.TS, decoded.TS)
}

func TestDecodeCompactionPayload_NotFake(t *testing.T) {
	_, err := DecodeCompactionPayload("gAAAAABpM0Yj-some-real-openai-blob")
	assert.Error(t, err)
}

func TestIsFakeCompaction(t *testing.T) {
	assert.True(t, IsFakeCompaction("aisw_dGVzdA=="))
	assert.False(t, IsFakeCompaction("gAAAAABpM0Yj-"))
	assert.False(t, IsFakeCompaction(""))
}

func TestExtractConversationText_NilInput(t *testing.T) {
	assert.Equal(t, "", ExtractConversationText(nil))
}

func TestExtractConversationText_StringInput(t *testing.T) {
	assert.Equal(t, "hello world", ExtractConversationText("hello world"))
}

func TestExtractConversationText_ArrayInput(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "Create a landing page"},
			},
		},
		map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "output_text", "text": "Here is the HTML..."},
			},
		},
	}
	result := ExtractConversationText(input)
	assert.Contains(t, result, "[user]: Create a landing page")
	assert.Contains(t, result, "[assistant]: Here is the HTML...")
}

func TestExtractConversationText_SkipsCompaction(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "Hello"},
			},
		},
		map[string]any{
			"type":              "compaction",
			"encrypted_content": "aisw_eyJzdW1tYXJ5IjoiZm9vIn0=",
		},
	}
	result := ExtractConversationText(input)
	assert.Contains(t, result, "[user]: Hello")
	assert.NotContains(t, result, "compaction")
}

func TestExtractConversationText_FunctionCallItems(t *testing.T) {
	input := []any{
		map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "What's the weather?"},
			},
		},
		map[string]any{
			"type":      "function_call",
			"call_id":   "call_1",
			"name":      "get_weather",
			"arguments": `{"city":"NYC"}`,
		},
		map[string]any{
			"type":    "function_call_output",
			"call_id": "call_1",
			"output":  `{"temp":72,"condition":"sunny"}`,
		},
		map[string]any{
			"type": "message",
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "output_text", "text": "It's 72°F and sunny in NYC"},
			},
		},
	}
	result := ExtractConversationText(input)
	assert.Contains(t, result, "[user]: What's the weather?")
	assert.Contains(t, result, "[tool_call: get_weather({\"city\":\"NYC\"})]")
	assert.Contains(t, result, "[tool_result: {\"temp\":72,\"condition\":\"sunny\"}]")
	assert.Contains(t, result, "[assistant]: It's 72°F and sunny in NYC")
}

func TestExtractConversationText_FunctionCallTruncation(t *testing.T) {
	// Long arguments should be truncated to 200 chars so compaction summary stays compact
	longArgs := string(make([]byte, 500))
	for i := range longArgs {
		longArgs = longArgs[:i] + "x" + longArgs[i+1:]
	}
	longArgs = `{"data":"` + longArgs + `"}`

	input := []any{
		map[string]any{
			"type":      "function_call",
			"name":      "read_file",
			"arguments": longArgs,
		},
		map[string]any{
			"type":   "function_call_output",
			"output": longArgs,
		},
	}
	result := ExtractConversationText(input)
	assert.Contains(t, result, "[tool_call: read_file(")
	assert.Contains(t, result, "...")
	// Verify truncation keeps it reasonable (< 300 chars per line)
	for _, line := range splitLines(result) {
		assert.Less(t, len(line), 300, "each line should be truncated: %s", line[:min(50, len(line))])
	}
}

func TestExtractConversationText_FunctionCallSkipsCompaction(t *testing.T) {
	input := []any{
		map[string]any{
			"type":              "compaction",
			"encrypted_content": "aisw_abc",
		},
		map[string]any{
			"type":      "function_call",
			"name":      "test",
			"arguments": "{}",
		},
	}
	result := ExtractConversationText(input)
	assert.NotContains(t, result, "compaction")
	assert.Contains(t, result, "[tool_call: test({})]")
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestBuildSummarizationRequest(t *testing.T) {
	req := BuildSummarizationRequest("user said hello\nassistant replied hi", "test-model")
	assert.Equal(t, "test-model", req.Model)
	assert.False(t, req.Stream)
	assert.Equal(t, 1024, req.MaxTokens)
	assert.Len(t, req.Messages, 2)
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Equal(t, "user", req.Messages[1].Role)
}
