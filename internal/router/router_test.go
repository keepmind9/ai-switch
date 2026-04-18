package router

import (
	"testing"

	"github.com/keepmind9/llm-gateway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider() *config.Provider {
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: "https://api.default.com/v1",
			APIKey:  "default-key",
			Format:  "chat",
			Model:   "default-model",
		},
		Providers: map[string]config.ProviderConfig{
			"zhipu": {
				Name:    "Zhipu",
				BaseURL: "https://open.bigmodel.cn/api/anthropic",
				APIKey:  "zhipu-key",
				Format:  "anthropic",
			},
			"deepseek": {
				Name:    "DeepSeek",
				BaseURL: "https://api.deepseek.com/v1",
				APIKey:  "ds-key",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-zhipu": {
				Provider:     "zhipu",
				DefaultModel: "glm-5.1",
				SceneMap: map[string]string{
					"default":    "glm-5.1",
					"think":      "glm-5.1",
					"websearch":  "glm-4.7",
					"background": "glm-4.5-air",
				},
				ModelMap: map[string]string{
					"gpt-4o": "glm-5.1",
				},
			},
			"gw-deepseek": {
				Provider:     "deepseek",
				DefaultModel: "deepseek-chat",
			},
		},
	}
	return config.NewProvider(cfg, "")
}

func TestConfigRouter_RouteByAPIKey(t *testing.T) {
	r := NewConfigRouter(newTestProvider())

	result, err := r.Route("chat", "gw-deepseek", []byte(`{"model":"gpt-4o"}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.deepseek.com/v1", result.BaseURL)
	assert.Equal(t, "ds-key", result.APIKey)
	assert.Equal(t, "chat", result.Format)
	assert.Equal(t, "deepseek-chat", result.Model)
}

func TestConfigRouter_RouteSceneDetection(t *testing.T) {
	r := NewConfigRouter(newTestProvider())

	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "thinking scene",
			body:     `{"model":"claude-sonnet","thinking":{"type":"enabled"},"messages":[]}`,
			expected: "glm-5.1",
		},
		{
			name:     "websearch scene",
			body:     `{"model":"claude-sonnet","tools":[{"type":"web_search_20250305"}],"messages":[]}`,
			expected: "glm-4.7",
		},
		{
			name:     "background scene",
			body:     `{"model":"claude-3-5-haiku","messages":[]}`,
			expected: "glm-4.5-air",
		},
		{
			name:     "default scene",
			body:     `{"model":"claude-sonnet","messages":[]}`,
			expected: "glm-5.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.Route("anthropic", "gw-zhipu", []byte(tt.body))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Model)
			assert.Equal(t, "https://open.bigmodel.cn/api/anthropic", result.BaseURL)
			assert.Equal(t, "anthropic", result.Format)
		})
	}
}

func TestConfigRouter_RouteModelMap(t *testing.T) {
	r := NewConfigRouter(newTestProvider())

	result, err := r.Route("chat", "gw-zhipu", []byte(`{"model":"gpt-4o"}`))
	require.NoError(t, err)
	assert.Equal(t, "glm-5.1", result.Model)
}

func TestConfigRouter_RouteModelMapNoMatch(t *testing.T) {
	r := NewConfigRouter(newTestProvider())

	result, err := r.Route("chat", "gw-zhipu", []byte(`{"model":"claude-3"}`))
	require.NoError(t, err)
	assert.Equal(t, "glm-5.1", result.Model) // falls back to default_model
}

func TestConfigRouter_RouteUnknownAPIKey(t *testing.T) {
	r := NewConfigRouter(newTestProvider())

	result, err := r.Route("chat", "unknown-key", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.default.com/v1", result.BaseURL)
	assert.Equal(t, "default-key", result.APIKey)
	assert.Equal(t, "default-model", result.Model)
}

func TestConfigRouter_RouteNoRoutes(t *testing.T) {
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: "https://api.default.com/v1",
			APIKey:  "default-key",
			Format:  "chat",
			Model:   "default-model",
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	result, err := r.Route("chat", "any-key", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.default.com/v1", result.BaseURL)
	assert.Equal(t, "default-model", result.Model)
}

func TestConfigRouter_RouteUnknownProvider(t *testing.T) {
	cfg := &config.Config{
		Upstream: config.UpstreamConfig{
			BaseURL: "https://api.default.com/v1",
			APIKey:  "default-key",
			Format:  "chat",
			Model:   "default-model",
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {
				Provider:     "nonexistent",
				DefaultModel: "test-model",
			},
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	_, err := r.Route("chat", "gw-test", []byte(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
}
