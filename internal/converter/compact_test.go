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
