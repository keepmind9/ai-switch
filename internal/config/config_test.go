package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "127.0.0.1"
  port: 8080

default_route: "gw-openai"

providers:
  openai:
    name: "OpenAI"
    base_url: "https://api.example.com/v1"
    api_key: "sk-test-key"
    format: "chat"
routes:
  "gw-openai":
    provider: "openai"
    default_model: "test-model"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "gw-openai", cfg.DefaultRoute)

	p, ok := cfg.Providers["openai"]
	require.True(t, ok)
	assert.Equal(t, "OpenAI", p.Name)
	assert.Equal(t, "https://api.example.com/v1", p.BaseURL)
	assert.Equal(t, "sk-test-key", p.APIKey)
	assert.Equal(t, "chat", p.Format)
}

func TestLoad_DefaultFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  test:
    name: "Test"
    base_url: "https://api.example.com/v1"
    api_key: "key"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "chat", cfg.Providers["test"].Format)
}

func TestLoad_InvalidFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  test:
    name: "Test"
    base_url: "https://api.example.com/v1"
    api_key: "key"
    format: "invalid"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	_, err = Load(cfgPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestLoad_AllFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
		valid  bool
	}{
		{"chat format", "chat", true},
		{"responses format", "responses", true},
		{"anthropic format", "anthropic", true},
		{"invalid format", "websocket", false},
		{"empty format handled as default", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")

			formatLine := ""
			if tt.format != "" {
				formatLine = "    format: \"" + tt.format + "\"\n"
			}

			content := "server:\n  host: \"0.0.0.0\"\n  port: 12345\nproviders:\n  test:\n    name: \"Test\"\n    base_url: \"https://api.example.com/v1\"\n    api_key: \"key\"\n" + formatLine
			err := os.WriteFile(cfgPath, []byte(content), 0644)
			require.NoError(t, err)

			cfg, err := Load(cfgPath)
			if tt.valid {
				require.NoError(t, err)
				expected := tt.format
				if expected == "" {
					expected = "chat"
				}
				assert.Equal(t, expected, cfg.Providers["test"].Format)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestLoad_Providers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  deepseek:
    name: "DeepSeek"
    base_url: "https://api.deepseek.com/v1"
    api_key: "ds-key"
    format: "chat"
    sponsor: true
  minimax:
    name: "MiniMax"
    base_url: "https://api.minimaxi.com/v1"
    api_key: "mm-key"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	require.Len(t, cfg.Providers, 2)

	ds, ok := cfg.Providers["deepseek"]
	require.True(t, ok)
	assert.Equal(t, "DeepSeek", ds.Name)
	assert.Equal(t, "chat", ds.Format)
	assert.True(t, ds.Sponsor)

	mm, ok := cfg.Providers["minimax"]
	require.True(t, ok)
	assert.Equal(t, "MiniMax", mm.Name)
	assert.Equal(t, "chat", mm.Format) // default
}

func TestLoad_DefaultRouteValidation(t *testing.T) {
	tests := []struct {
		name          string
		defaultRoute  string
		providers     string
		routes        string
		expectError   bool
		errorContains string
	}{
		{
			name:         "valid default_route",
			defaultRoute: "gw-test",
			providers: `  test:
    name: "Test"
    base_url: "https://test.com/v1"
    api_key: "key"`,
			routes: `  "gw-test":
    provider: "test"
    default_model: "model"`,
			expectError: false,
		},
		{
			name:         "default_route not in routes",
			defaultRoute: "missing",
			providers: `  test:
    name: "Test"
    base_url: "https://test.com/v1"
    api_key: "key"`,
			routes: `  "gw-test":
    provider: "test"
    default_model: "model"`,
			expectError:   true,
			errorContains: "default_route",
		},
		{
			name:         "empty default_route is valid",
			defaultRoute: "",
			providers: `  test:
    name: "Test"
    base_url: "https://test.com/v1"
    api_key: "key"`,
			routes:        "",
			expectError:   false,
			errorContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.yaml")

			drLine := ""
			if tt.defaultRoute != "" {
				drLine = "default_route: \"" + tt.defaultRoute + "\"\n"
			}

			routesSection := ""
			if tt.routes != "" {
				routesSection = "routes:\n" + tt.routes + "\n"
			}

			content := "server:\n  host: \"0.0.0.0\"\n  port: 12345\n" + drLine + "providers:\n" + tt.providers + "\n" + routesSection
			err := os.WriteFile(cfgPath, []byte(content), 0644)
			require.NoError(t, err)

			cfg, err := Load(cfgPath)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestLoad_ExpandEnv(t *testing.T) {
	os.Setenv("TEST_API_KEY_123", "expanded-key-value")
	defer os.Unsetenv("TEST_API_KEY_123")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  default:
    name: "Default"
    base_url: "https://api.example.com/v1"
    api_key: "${TEST_API_KEY_123}"
  test:
    name: "Test"
    base_url: "https://test.com/v1"
    api_key: "${TEST_API_KEY_123}"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "expanded-key-value", cfg.Providers["default"].APIKey)
	assert.Equal(t, "expanded-key-value", cfg.Providers["test"].APIKey)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	assert.Error(t, err)
}

func TestDataDir(t *testing.T) {
	dir, err := DataDir()
	require.NoError(t, err)
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".ai-switch"), dir)
}

func TestEnsureDataDir(t *testing.T) {
	dir, err := EnsureDataDir()
	require.NoError(t, err)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestDefaultConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		flagPath string
		setup    func() // prepare local config.yaml
		cleanup  func()
		expected string
	}{
		{
			name:     "custom flag path",
			flagPath: "/custom/path.yaml",
			expected: "/custom/path.yaml",
		},
		{
			name:     "flag path not config.yaml",
			flagPath: "my-config.yaml",
			expected: "my-config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultConfigPath(tt.flagPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoad_Routes(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  default:
    name: "Default"
    base_url: "https://api.example.com/v1"
    api_key: "key"
  zhipu:
    name: "Zhipu"
    base_url: "https://open.bigmodel.cn/api/anthropic"
    api_key: "zhipu-key"
    format: "anthropic"
routes:
  "gw-zhipu":
    provider: "zhipu"
    default_model: "glm-5.1"
    scene_map:
      default: "glm-5.1"
      background: "glm-4.5-air"
      think: "glm-5.1"
      websearch: "glm-4.7"
    model_map:
      "gpt-4o": "glm-5.1"
  "gw-deepseek":
    provider: "deepseek"
    default_model: "deepseek-chat"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	require.Len(t, cfg.Routes, 2)

	zhipuRule := cfg.Routes["gw-zhipu"]
	assert.Equal(t, "zhipu", zhipuRule.Provider)
	assert.Equal(t, "glm-5.1", zhipuRule.DefaultModel)
	assert.Equal(t, "glm-4.5-air", zhipuRule.SceneMap["background"])
	assert.Equal(t, "glm-4.7", zhipuRule.SceneMap["websearch"])
	assert.Equal(t, "glm-5.1", zhipuRule.ModelMap["gpt-4o"])

	dsRule := cfg.Routes["gw-deepseek"]
	assert.Equal(t, "deepseek", dsRule.Provider)
	assert.Equal(t, "deepseek-chat", dsRule.DefaultModel)
	assert.Equal(t, 0, dsRule.LongContextThreshold)
}

func TestLoad_RoutesWithLongContextThreshold(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  default:
    name: "Default"
    base_url: "https://api.example.com/v1"
    api_key: "key"
routes:
  "gw-test":
    provider: "default"
    default_model: "test-model"
    long_context_threshold: 60000
    scene_map:
      default: "test-model"
      longContext: "test-model-large"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	rule := cfg.Routes["gw-test"]
	assert.Equal(t, 60000, rule.LongContextThreshold)
	assert.Equal(t, "test-model-large", rule.SceneMap["longcontext"])
}

func TestLoad_RoutesEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  default:
    name: "Default"
    base_url: "https://api.example.com/v1"
    api_key: "key"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Nil(t, cfg.Routes)
}

func TestDefaultRouteConfig(t *testing.T) {
	tests := []struct {
		name         string
		defaultRoute string
		routes       map[string]RouteRule
		expectNil    bool
		expectedProv string
	}{
		{
			name:         "returns matching route",
			defaultRoute: "gw-test",
			routes: map[string]RouteRule{
				"gw-test": {Provider: "openai", DefaultModel: "gpt-4"},
			},
			expectNil:    false,
			expectedProv: "openai",
		},
		{
			name:         "empty default_route returns nil",
			defaultRoute: "",
			routes: map[string]RouteRule{
				"gw-test": {Provider: "test"},
			},
			expectNil: true,
		},
		{
			name:         "missing route returns nil",
			defaultRoute: "missing",
			routes: map[string]RouteRule{
				"gw-test": {Provider: "test"},
			},
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				DefaultRoute: tt.defaultRoute,
				Routes:       tt.routes,
			}
			result := cfg.DefaultRouteConfig()
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedProv, result.Provider)
			}
		})
	}
}

func TestExpandEnv(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		envKey   string
		envValue string
		expected string
	}{
		{"env var", "${MY_KEY}", "MY_KEY", "my-value", "my-value"},
		{"plain string", "plain-key", "", "", "plain-key"},
		{"empty string", "", "", "", ""},
		{"missing env", "${NONEXISTENT_VAR_XYZ}", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envKey != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			}
			result := expandEnv(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
