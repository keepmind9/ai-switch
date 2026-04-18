package converter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertRequest_ChatPassthrough(t *testing.T) {
	c := NewConverter()
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":false}`

	conv, err := c.ConvertRequest(FormatChat, FormatChat, []byte(body), "default-model", map[string]string{"gpt-4o": "mapped-model"})
	require.NoError(t, err)

	assert.Equal(t, "/v1/chat/completions", conv.UpstreamPath)
	assert.Equal(t, "mapped-model", conv.Model)
	assert.False(t, conv.IsStreaming)

	var raw map[string]any
	json.Unmarshal(conv.UpstreamBody, &raw)
	assert.Equal(t, "mapped-model", raw["model"])
}

func TestConvertRequest_ResponsesToChat(t *testing.T) {
	c := NewConverter()
	body := `{"model":"codex-model","input":"hello","stream":true}`

	conv, err := c.ConvertRequest(FormatResponses, FormatChat, []byte(body), "default-model", nil)
	require.NoError(t, err)

	assert.Equal(t, "/chat/completions", conv.UpstreamPath)
	assert.True(t, conv.IsStreaming)

	var chatReq map[string]any
	json.Unmarshal(conv.UpstreamBody, &chatReq)
	assert.NotNil(t, chatReq["messages"])
}

func TestConvertRequest_ResponsesToAnthropic(t *testing.T) {
	c := NewConverter()
	body := `{"model":"codex-model","input":"hello","max_tokens":1024}`

	conv, err := c.ConvertRequest(FormatResponses, FormatAnthropic, []byte(body), "default-model", nil)
	require.NoError(t, err)

	assert.Equal(t, "/v1/messages", conv.UpstreamPath)

	var anthReq map[string]any
	json.Unmarshal(conv.UpstreamBody, &anthReq)
	assert.NotNil(t, anthReq["messages"])
	assert.Equal(t, float64(1024), anthReq["max_tokens"])
}

func TestConvertRequest_AnthropicToChat(t *testing.T) {
	c := NewConverter()
	body := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"max_tokens":2048}`

	conv, err := c.ConvertRequest(FormatAnthropic, FormatChat, []byte(body), "default-model", map[string]string{"claude-sonnet-4-5": "upstream-model"})
	require.NoError(t, err)

	assert.Equal(t, "/chat/completions", conv.UpstreamPath)
	assert.Equal(t, "claude-sonnet-4-5", conv.Model)

	var chatReq map[string]any
	json.Unmarshal(conv.UpstreamBody, &chatReq)
	assert.NotNil(t, chatReq["messages"])
}

func TestConvertRequest_AnthropicToResponses(t *testing.T) {
	c := NewConverter()
	body := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}]}`

	conv, err := c.ConvertRequest(FormatAnthropic, FormatResponses, []byte(body), "default-model", nil)
	require.NoError(t, err)

	assert.Equal(t, "/v1/responses", conv.UpstreamPath)

	var respReq map[string]any
	json.Unmarshal(conv.UpstreamBody, &respReq)
	assert.NotNil(t, respReq["input"])
}

func TestConvertRequest_ChatToAnthropic(t *testing.T) {
	c := NewConverter()
	body := `{"model":"gpt-4o","messages":[{"role":"system","content":"Be helpful"},{"role":"user","content":"hi"}],"max_tokens":1024}`

	conv, err := c.ConvertRequest(FormatChat, FormatAnthropic, []byte(body), "default-model", nil)
	require.NoError(t, err)

	assert.Equal(t, "/v1/messages", conv.UpstreamPath)

	var anthReq map[string]any
	json.Unmarshal(conv.UpstreamBody, &anthReq)
	assert.Equal(t, "Be helpful", anthReq["system"])

	messages, ok := anthReq["messages"].([]any)
	require.True(t, ok)
	assert.Len(t, messages, 1) // only user message, system extracted
}

func TestConvertRequest_AnthropicPassthrough(t *testing.T) {
	c := NewConverter()
	body := `{"model":"claude-sonnet-4-5","messages":[{"role":"user","content":"hi"}],"max_tokens":1024,"stream":true}`

	conv, err := c.ConvertRequest(FormatAnthropic, FormatAnthropic, []byte(body), "default-model", nil)
	require.NoError(t, err)

	assert.Equal(t, "/v1/messages", conv.UpstreamPath)
	assert.True(t, conv.IsStreaming)
	assert.Equal(t, "claude-sonnet-4-5", conv.Model)
}

func TestConvertRequest_ResponsesPassthrough(t *testing.T) {
	c := NewConverter()
	body := `{"model":"codex","input":"hello","stream":false}`

	conv, err := c.ConvertRequest(FormatResponses, FormatResponses, []byte(body), "default-model", nil)
	require.NoError(t, err)

	assert.Equal(t, "/v1/responses", conv.UpstreamPath)
	assert.False(t, conv.IsStreaming)
}

func TestConvertRequest_DefaultModel(t *testing.T) {
	c := NewConverter()
	body := `{"messages":[{"role":"user","content":"hi"}]}`

	conv, err := c.ConvertRequest(FormatChat, FormatChat, []byte(body), "fallback-model", nil)
	require.NoError(t, err)
	assert.Equal(t, "fallback-model", conv.Model)
}

func TestConvertRequest_InvalidJSON(t *testing.T) {
	c := NewConverter()
	_, err := c.ConvertRequest(FormatChat, FormatChat, []byte("not json"), "model", nil)
	assert.Error(t, err)
}

func TestConvertRequest_UnsupportedClientFormat(t *testing.T) {
	c := NewConverter()
	_, err := c.ConvertRequest("websocket", FormatChat, []byte(`{}`), "model", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported client format")
}

func TestUpstreamPath(t *testing.T) {
	tests := []struct {
		format   string
		expected string
	}{
		{FormatChat, "/v1/chat/completions"},
		{"", "/v1/chat/completions"},
		{FormatAnthropic, "/v1/messages"},
		{FormatResponses, "/v1/responses"},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			assert.Equal(t, tt.expected, upstreamPath(tt.format))
		})
	}
}
