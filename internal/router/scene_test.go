package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectScene(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		cfg      SceneConfig
		expected string
	}{
		{
			name:     "thinking field present",
			body:     `{"model":"claude-sonnet","thinking":{"type":"enabled","budget_tokens":5000},"messages":[]}`,
			cfg:      SceneConfig{},
			expected: SceneThink,
		},
		{
			name:     "web search tool",
			body:     `{"model":"claude-sonnet","tools":[{"type":"web_search_20250305","name":"web_search"}],"messages":[]}`,
			cfg:      SceneConfig{},
			expected: SceneWebSearch,
		},
		{
			name:     "haiku model",
			body:     `{"model":"claude-3-5-haiku-20241022","messages":[]}`,
			cfg:      SceneConfig{},
			expected: SceneBackground,
		},
		{
			name:     "default fallback",
			body:     `{"model":"claude-sonnet","messages":[]}`,
			cfg:      SceneConfig{},
			expected: SceneDefault,
		},
		{
			name:     "empty body",
			body:     `{}`,
			cfg:      SceneConfig{},
			expected: SceneDefault,
		},
		{
			name:     "invalid json",
			body:     `not json`,
			cfg:      SceneConfig{},
			expected: SceneDefault,
		},
		// Priority: websearch > think
		{
			name:     "websearch takes priority over think",
			body:     `{"model":"claude-sonnet","thinking":{"type":"enabled"},"tools":[{"type":"web_search_20250305"}],"messages":[]}`,
			cfg:      SceneConfig{},
			expected: SceneWebSearch,
		},
		// Priority: background > websearch
		{
			name:     "background takes priority over websearch",
			body:     `{"model":"claude-3-5-haiku","tools":[{"type":"web_search_beta"}],"messages":[]}`,
			cfg:      SceneConfig{},
			expected: SceneBackground,
		},
		{
			name:     "non web search tools",
			body:     `{"model":"claude-sonnet","tools":[{"type":"computer_20250124"}],"messages":[]}`,
			cfg:      SceneConfig{},
			expected: SceneDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectScene([]byte(tt.body), tt.cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectScene_Image(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "image in user message",
			body:     `{"model":"claude-sonnet","messages":[{"role":"user","content":[{"type":"text","text":"What is this?"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBOR..."}}]}]}`,
			expected: SceneImage,
		},
		{
			name:     "assistant message with image ignored",
			body:     `{"model":"claude-sonnet","messages":[{"role":"assistant","content":[{"type":"image","source":{}}]}]}`,
			expected: SceneDefault,
		},
		{
			name:     "string content no image",
			body:     `{"model":"claude-sonnet","messages":[{"role":"user","content":"Hello"}]}`,
			expected: SceneDefault,
		},
		{
			name:     "think takes priority over image",
			body:     `{"model":"claude-sonnet","thinking":{"type":"enabled"},"messages":[{"role":"user","content":[{"type":"image","source":{}}]}]}`,
			expected: SceneThink,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectScene([]byte(tt.body), SceneConfig{})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectScene_LongContext(t *testing.T) {
	longText := ""
	for i := 0; i < 500; i++ {
		longText += "This is a test sentence for token counting. "
	}
	body := `{"model":"claude-sonnet","messages":[{"role":"user","content":"` + longText + `"}]}`

	t.Run("exceeds threshold", func(t *testing.T) {
		result := DetectScene([]byte(body), SceneConfig{LongContextThreshold: 100})
		assert.Equal(t, SceneLongContext, result)
	})

	t.Run("below threshold", func(t *testing.T) {
		result := DetectScene([]byte(body), SceneConfig{LongContextThreshold: 100000})
		assert.Equal(t, "default", result)
	})

	t.Run("threshold zero disables longContext", func(t *testing.T) {
		result := DetectScene([]byte(body), SceneConfig{LongContextThreshold: 0})
		assert.Equal(t, "default", result)
	})

	t.Run("longContext takes priority over background", func(t *testing.T) {
		haikuBody := `{"model":"claude-3-5-haiku","messages":[{"role":"user","content":"` + longText + `"}]}`
		result := DetectScene([]byte(haikuBody), SceneConfig{LongContextThreshold: 100})
		assert.Equal(t, SceneLongContext, result)
	})
}
