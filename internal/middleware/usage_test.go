package middleware

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractUsage_ChatFormat(t *testing.T) {
	body := `{"id":"chatcmpl-1","model":"gpt-4o","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`

	record := extractUsage([]byte(body), "openai")
	require.NotNil(t, record)
	assert.Equal(t, "openai", record.Provider)
	assert.Equal(t, "gpt-4o", record.Model)
	assert.Equal(t, int64(100), record.InputTokens)
	assert.Equal(t, int64(50), record.OutputTokens)
	assert.Equal(t, int64(150), record.TotalTokens)
	assert.Equal(t, int64(1), record.Requests)
}

func TestExtractUsage_AnthropicFormat(t *testing.T) {
	body := `{"id":"msg_1","type":"message","model":"claude-sonnet-4-5","usage":{"input_tokens":200,"output_tokens":80,"cache_creation_input_tokens":30,"cache_read_input_tokens":70}}`

	record := extractUsage([]byte(body), "anthropic")
	require.NotNil(t, record)
	assert.Equal(t, "anthropic", record.Provider)
	assert.Equal(t, "claude-sonnet-4-5", record.Model)
	assert.Equal(t, int64(200), record.InputTokens)
	assert.Equal(t, int64(80), record.OutputTokens)
	assert.Equal(t, int64(280), record.TotalTokens)
	assert.Equal(t, int64(30), record.CacheCreationTokens)
	assert.Equal(t, int64(70), record.CacheReadTokens)
}

func TestExtractUsage_ResponsesFormat(t *testing.T) {
	body := `{"id":"resp_1","model":"test","usage":{"input_tokens":50,"output_tokens":25,"total_tokens":75}}`

	record := extractUsage([]byte(body), "provider")
	require.NotNil(t, record)
	assert.Equal(t, int64(75), record.TotalTokens)
}

func TestExtractUsage_NoUsage(t *testing.T) {
	body := `{"id":"chatcmpl-1","model":"gpt-4o","choices":[]}`
	record := extractUsage([]byte(body), "provider")
	assert.Nil(t, record)
}

func TestExtractUsage_EmptyBody(t *testing.T) {
	record := extractUsage([]byte{}, "provider")
	assert.Nil(t, record)
}

func TestExtractUsage_InvalidJSON(t *testing.T) {
	record := extractUsage([]byte("not json"), "provider")
	assert.Nil(t, record)
}

func TestExtractUsage_ZeroTokens(t *testing.T) {
	body := `{"model":"test","usage":{"input_tokens":0,"output_tokens":0}}`
	record := extractUsage([]byte(body), "provider")
	assert.Nil(t, record)
}

func TestExtractUsage_OpenAICacheTokens(t *testing.T) {
	body := `{"model":"gpt-4o","usage":{"prompt_tokens":200,"completion_tokens":100,"total_tokens":300,"prompt_tokens_details":{"cached_tokens":150}}}`

	record := extractUsage([]byte(body), "openai")
	require.NotNil(t, record)
	assert.Equal(t, int64(150), record.CacheReadTokens)
}

func TestToFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected float64
	}{
		{"float64", float64(42.5), 42.5},
		{"int", 42, 42.0},
		{"int64", int64(42), 42.0},
		{"json.Number", json.Number("42.5"), 42.5},
		{"nil", nil, 0.0},
		{"string", "42", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, toFloat(tt.input))
		})
	}
}
