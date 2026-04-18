package router

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectScene(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "thinking field present",
			body:     `{"model":"claude-sonnet","thinking":{"type":"enabled","budget_tokens":5000},"messages":[]}`,
			expected: "think",
		},
		{
			name:     "thinking field is object",
			body:     `{"model":"claude-sonnet","thinking":{"type":"enabled"},"messages":[]}`,
			expected: "think",
		},
		{
			name:     "web search tool",
			body:     `{"model":"claude-sonnet","tools":[{"type":"web_search_20250305","name":"web_search"}],"messages":[]}`,
			expected: "websearch",
		},
		{
			name:     "web search tool variant",
			body:     `{"model":"claude-sonnet","tools":[{"type":"web_search_beta"}],"messages":[]}`,
			expected: "websearch",
		},
		{
			name:     "haiku model",
			body:     `{"model":"claude-3-5-haiku-20241022","messages":[]}`,
			expected: "background",
		},
		{
			name:     "haiku in model name case insensitive",
			body:     `{"model":"Claude-Haiku","messages":[]}`,
			expected: "background",
		},
		{
			name:     "default fallback",
			body:     `{"model":"claude-sonnet","messages":[]}`,
			expected: "default",
		},
		{
			name:     "empty body",
			body:     `{}`,
			expected: "default",
		},
		{
			name:     "invalid json",
			body:     `not json`,
			expected: "default",
		},
		{
			name:     "thinking takes priority over web search",
			body:     `{"model":"claude-sonnet","thinking":{"type":"enabled"},"tools":[{"type":"web_search_20250305"}],"messages":[]}`,
			expected: "think",
		},
		{
			name:     "web search takes priority over haiku",
			body:     `{"model":"claude-3-5-haiku","tools":[{"type":"web_search_beta"}],"messages":[]}`,
			expected: "websearch",
		},
		{
			name:     "non web search tools",
			body:     `{"model":"claude-sonnet","tools":[{"type":"computer_20250124"}],"messages":[]}`,
			expected: "default",
		},
		{
			name:     "empty tools array",
			body:     `{"model":"claude-sonnet","tools":[],"messages":[]}`,
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectScene([]byte(tt.body))
			assert.Equal(t, tt.expected, result)
		})
	}
}
