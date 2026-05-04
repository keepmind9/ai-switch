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

func TestBuildSummarizationRequest(t *testing.T) {
	req := BuildSummarizationRequest("user said hello\nassistant replied hi", "test-model")
	assert.Equal(t, "test-model", req.Model)
	assert.False(t, req.Stream)
	assert.Equal(t, 1024, req.MaxTokens)
	assert.Len(t, req.Messages, 2)
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Equal(t, "user", req.Messages[1].Role)
}
