package router

import (
	"fmt"
	"testing"

	"github.com/keepmind9/ai-switch/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider() *config.Provider {
	cfg := &config.Config{
		DefaultRoute: "gw-default",
		Providers: map[string]config.ProviderConfig{
			"minimax": {
				Name:    "MiniMax",
				BaseURL: "https://api.default.com",
				APIKey:  "default-key",
				Format:  "chat",
			},
			"zhipu": {
				Name:    "Zhipu",
				BaseURL: "https://open.bigmodel.cn/api/anthropic",
				APIKey:  "zhipu-key",
				Format:  "anthropic",
			},
			"deepseek": {
				Name:    "DeepSeek",
				BaseURL: "https://api.deepseek.com",
				APIKey:  "ds-key",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-default": {
				Provider:     "minimax",
				DefaultModel: "default-model",
			},
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
	assert.Equal(t, "https://api.deepseek.com", result.BaseURL)
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
	assert.Equal(t, "https://api.default.com", result.BaseURL)
	assert.Equal(t, "default-key", result.APIKey)
	assert.Equal(t, "default-model", result.Model)
}

func TestConfigRouter_RouteNoRoutes(t *testing.T) {
	cfg := &config.Config{
		DefaultRoute: "gw-default",
		Providers: map[string]config.ProviderConfig{
			"minimax": {
				BaseURL: "https://api.default.com",
				APIKey:  "default-key",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-default": {
				Provider:     "minimax",
				DefaultModel: "default-model",
			},
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	result, err := r.Route("chat", "any-key", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.default.com", result.BaseURL)
	assert.Equal(t, "default-model", result.Model)
}

func TestConfigRouter_RouteNoDefault(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	_, err := r.Route("chat", "unknown-key", []byte(`{}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "default_route not configured")
}

func TestConfigRouter_CrossProviderRouting(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"minimax": {
				Name:    "MiniMax",
				BaseURL: "https://api.minimaxi.com",
				APIKey:  "mm-key",
				Format:  "chat",
			},
			"deepseek": {
				Name:    "DeepSeek",
				BaseURL: "https://api.deepseek.com",
				APIKey:  "ds-key",
				Format:  "chat",
			},
			"zhipu": {
				Name:    "Zhipu",
				BaseURL: "https://open.bigmodel.cn/api/anthropic",
				APIKey:  "zhipu-key",
				Format:  "anthropic",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {
				Provider:     "zhipu",
				DefaultModel: "MiniMax-M2.5",
				SceneMap: map[string]string{
					"default":   "glm-5.1",
					"think":     "deepseek:deepseek-chat",
					"websearch": "glm-4.7",
				},
			},
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	// think scene → "deepseek:deepseek-chat" → resolves to deepseek provider
	result, err := r.Route("anthropic", "gw-test", []byte(`{"model":"claude-sonnet","thinking":{"type":"enabled"},"messages":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.deepseek.com", result.BaseURL)
	assert.Equal(t, "ds-key", result.APIKey)
	assert.Equal(t, "chat", result.Format)
	assert.Equal(t, "deepseek-chat", result.Model)

	// default scene → "glm-5.1" → plain model, uses route provider (zhipu)
	result, err = r.Route("anthropic", "gw-test", []byte(`{"model":"claude-sonnet","messages":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "https://open.bigmodel.cn/api/anthropic", result.BaseURL)
	assert.Equal(t, "zhipu-key", result.APIKey)
	assert.Equal(t, "glm-5.1", result.Model)
}

func TestConfigRouter_CrossProviderModelMap(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"minimax": {
				Name:    "MiniMax",
				BaseURL: "https://api.minimaxi.com",
				APIKey:  "mm-key",
				Format:  "chat",
			},
			"deepseek": {
				Name:    "DeepSeek",
				BaseURL: "https://api.deepseek.com",
				APIKey:  "ds-key",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {
				Provider:     "minimax",
				DefaultModel: "MiniMax-M2.5",
				ModelMap: map[string]string{
					"claude-sonnet-4-5": "deepseek:deepseek-chat",
				},
			},
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	// ModelMap with provider:model format → resolves to deepseek
	result, err := r.Route("chat", "gw-test", []byte(`{"model":"claude-sonnet-4-5"}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.deepseek.com", result.BaseURL)
	assert.Equal(t, "ds-key", result.APIKey)
	assert.Equal(t, "deepseek-chat", result.Model)
}

func TestParseProviderModel(t *testing.T) {
	tests := []struct {
		input           string
		defaultProvider string
		expectedProv    string
		expectedModel   string
	}{
		{"deepseek:deepseek-chat", "minimax", "deepseek", "deepseek-chat"},
		{"MiniMax-M2.5", "minimax", "minimax", "MiniMax-M2.5"},
		{"zhipu:glm-4.7", "minimax", "zhipu", "glm-4.7"},
		{"plain-model", "deepseek", "deepseek", "plain-model"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			prov, model := config.SplitProviderModel(tt.input, tt.defaultProvider)
			assert.Equal(t, tt.expectedProv, prov)
			assert.Equal(t, tt.expectedModel, model)
		})
	}
}

func TestConfigRouter_DisabledRoute(t *testing.T) {
	p := newTestProvider()
	cfg := p.Get()
	// Disable gw-deepseek route
	r := cfg.Routes["gw-deepseek"]
	r.Disabled = true
	cfg.Routes["gw-deepseek"] = r

	router := NewConfigRouter(p)

	// Request with disabled route key should fall through to default route
	result, err := router.Route("chat", "gw-deepseek", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.default.com", result.BaseURL)
	assert.Equal(t, "default-key", result.APIKey)
	assert.Equal(t, "default-model", result.Model)
}

func TestConfigRouter_DisabledRouteWithProtocolDefault(t *testing.T) {
	cfg := &config.Config{
		DefaultRoute:          "gw-default",
		DefaultAnthropicRoute: "gw-anth",
		Providers: map[string]config.ProviderConfig{
			"minimax": {
				BaseURL: "https://api.default.com",
				APIKey:  "default-key",
				Format:  "chat",
			},
			"zhipu": {
				BaseURL: "https://open.bigmodel.cn/api/anthropic",
				APIKey:  "zhipu-key",
				Format:  "anthropic",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-default": {
				Provider:     "minimax",
				DefaultModel: "default-model",
			},
			"gw-anth": {
				Provider:     "zhipu",
				DefaultModel: "glm-5.1",
			},
			"gw-disabled": {
				Provider:     "minimax",
				DefaultModel: "should-not-use",
				Disabled:     true,
			},
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	// Disabled route with anthropic protocol falls through to protocol-specific default
	result, err := r.Route("anthropic", "gw-disabled", []byte(`{"model":"claude-sonnet","messages":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "https://open.bigmodel.cn/api/anthropic", result.BaseURL)
	assert.Equal(t, "glm-5.1", result.Model)
}

func TestConfigRouter_DisabledNotDefault(t *testing.T) {
	p := newTestProvider()
	cfg := p.Get()
	// gw-default is the default route, but it's not disabled
	r := cfg.Routes["gw-default"]

	router := NewConfigRouter(p)

	result, err := router.Route("chat", "gw-default", []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "https://api.default.com", result.BaseURL)
	assert.False(t, r.Disabled)
}

func TestConfigRouter_RouteUnknownProvider(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"minimax": {BaseURL: "https://api.minimax.com", APIKey: "key", Format: "chat"},
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
	assert.Contains(t, err.Error(), "provider \"nonexistent\" not found")
}

func TestConfigRouter_RouteModelMapPriorityOverSceneMap(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"zhipu": {
				Name:    "Zhipu",
				BaseURL: "https://open.bigmodel.cn/api/anthropic",
				APIKey:  "zhipu-key",
				Format:  "anthropic",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {
				Provider:     "zhipu",
				DefaultModel: "glm-5.1",
				SceneMap: map[string]string{
					"default": "glm-5.1",
					"think":   "glm-5.1-think",
				},
				ModelMap: map[string]string{
					"claude-sonnet-4-5": "glm-4.7",
				},
			},
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	// ModelMap should win over SceneMap
	result, err := r.Route("anthropic", "gw-test", []byte(`{"model":"claude-sonnet-4-5","messages":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "glm-4.7", result.Model)
}

func TestConfigRouter_RouteModelMapCaseInsensitive(t *testing.T) {
	cfg := &config.Config{
		Providers: map[string]config.ProviderConfig{
			"test": {
				Name:    "Test",
				BaseURL: "https://api.test.com",
				APIKey:  "key",
				Format:  "chat",
			},
		},
		Routes: map[string]config.RouteRule{
			"gw-test": {
				Provider:     "test",
				DefaultModel: "fallback-model",
				ModelMap: map[string]string{
					"GPT-4o": "mapped-model",
				},
			},
		},
	}
	r := NewConfigRouter(config.NewProvider(cfg, ""))

	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{"exact case", "GPT-4o", "mapped-model"},
		{"lowercase", "gpt-4o", "mapped-model"},
		{"mixed case", "Gpt-4O", "mapped-model"},
		{"no match", "claude-3", "fallback-model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.Route("chat", "gw-test", []byte(fmt.Sprintf(`{"model":"%s"}`, tt.model)))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Model)
		})
	}
}

func TestBuildUpstreamURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		apiPath  string
		expected string
	}{
		{"base without /v1", "https://api.example.com", "/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"base with /v1", "https://api.example.com/v1", "/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"base with trailing slash", "https://api.example.com/", "/v1/messages", "https://api.example.com/v1/messages"},
		{"custom path", "https://api.example.com/v1", "/custom/path", "https://api.example.com/v1/custom/path"},
		{"anthropic with /v1", "https://api.anthropic.com/v1", "/v1/messages", "https://api.anthropic.com/v1/messages"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, BuildUpstreamURL(tt.baseURL, tt.apiPath))
		})
	}
}

func TestFormatToPath(t *testing.T) {
	tests := []struct {
		format   string
		expected string
	}{
		{"chat", "/v1/chat/completions"},
		{"", "/v1/chat/completions"},
		{"anthropic", "/v1/messages"},
		{"responses", "/v1/responses"},
		{"unknown", "/v1/chat/completions"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatToPath(tt.format))
		})
	}
}

func TestConfigRouter_PathResolution(t *testing.T) {
	t.Run("path derived from format when config path empty", func(t *testing.T) {
		cfg := &config.Config{
			DefaultRoute: "gw-default",
			Providers: map[string]config.ProviderConfig{
				"chat-provider": {
					BaseURL: "https://api.example.com",
					APIKey:  "key",
					Format:  "chat",
				},
				"anthropic-provider": {
					BaseURL: "https://api.anthropic.com",
					APIKey:  "key",
					Format:  "anthropic",
				},
				"responses-provider": {
					BaseURL: "https://api.openai.com",
					APIKey:  "key",
					Format:  "responses",
				},
			},
			Routes: map[string]config.RouteRule{
				"gw-default": {Provider: "chat-provider", DefaultModel: "model"},
				"gw-anth":    {Provider: "anthropic-provider", DefaultModel: "model"},
				"gw-resp":    {Provider: "responses-provider", DefaultModel: "model"},
			},
		}
		r := NewConfigRouter(config.NewProvider(cfg, ""))

		result, err := r.Route("chat", "gw-default", []byte(`{}`))
		require.NoError(t, err)
		assert.Equal(t, "/v1/chat/completions", result.Path)

		result, err = r.Route("anthropic", "gw-anth", []byte(`{}`))
		require.NoError(t, err)
		assert.Equal(t, "/v1/messages", result.Path)

		result, err = r.Route("responses", "gw-resp", []byte(`{}`))
		require.NoError(t, err)
		assert.Equal(t, "/v1/responses", result.Path)
	})

	t.Run("config path overrides format-derived path", func(t *testing.T) {
		cfg := &config.Config{
			DefaultRoute: "gw-default",
			Providers: map[string]config.ProviderConfig{
				"custom": {
					BaseURL: "https://api.custom.com",
					APIKey:  "key",
					Format:  "chat",
					Path:    "/custom/v1/chat",
				},
			},
			Routes: map[string]config.RouteRule{
				"gw-default": {Provider: "custom", DefaultModel: "model"},
			},
		}
		r := NewConfigRouter(config.NewProvider(cfg, ""))

		result, err := r.Route("chat", "gw-default", []byte(`{}`))
		require.NoError(t, err)
		assert.Equal(t, "/custom/v1/chat", result.Path)
	})
}
