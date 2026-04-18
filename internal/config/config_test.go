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

upstream:
  base_url: "https://api.example.com/v1"
  api_key: "sk-test-key"
  model: "test-model"
  format: "chat"
  model_map:
    "gpt-4o": "test-model"
    "claude-3": "test-model-v2"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "https://api.example.com/v1", cfg.Upstream.BaseURL)
	assert.Equal(t, "sk-test-key", cfg.Upstream.APIKey)
	assert.Equal(t, "test-model", cfg.Upstream.Model)
	assert.Equal(t, "chat", cfg.Upstream.Format)
	assert.Equal(t, "test-model", cfg.Upstream.ModelMap["gpt-4o"])
	assert.Equal(t, "test-model-v2", cfg.Upstream.ModelMap["claude-3"])
}

func TestLoad_DefaultFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
upstream:
  base_url: "https://api.example.com/v1"
  api_key: "key"
  model: "model"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "chat", cfg.Upstream.Format)
}

func TestLoad_InvalidFormat(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
upstream:
  base_url: "https://api.example.com/v1"
  api_key: "key"
  model: "model"
  format: "invalid"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	_, err = Load(cfgPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid upstream format")
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
				formatLine = "  format: \"" + tt.format + "\"\n"
			}

			content := "server:\n  host: \"0.0.0.0\"\n  port: 12345\nupstream:\n  base_url: \"https://api.example.com/v1\"\n  api_key: \"key\"\n  model: \"model\"\n" + formatLine
			err := os.WriteFile(cfgPath, []byte(content), 0644)
			require.NoError(t, err)

			cfg, err := Load(cfgPath)
			if tt.valid {
				require.NoError(t, err)
				expected := tt.format
				if expected == "" {
					expected = "chat"
				}
				assert.Equal(t, expected, cfg.Upstream.Format)
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
upstream:
  base_url: "https://api.example.com/v1"
  api_key: "key"
  model: "model"
providers:
  deepseek:
    name: "DeepSeek"
    base_url: "https://api.deepseek.com/v1"
    api_key: "ds-key"
    model: "deepseek-chat"
    format: "chat"
    sponsor: true
  minimax:
    name: "MiniMax"
    base_url: "https://api.minimaxi.com/v1"
    api_key: "mm-key"
    model: "MiniMax-M2.5"
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

func TestLoad_ExpandEnv(t *testing.T) {
	os.Setenv("TEST_API_KEY_123", "expanded-key-value")
	defer os.Unsetenv("TEST_API_KEY_123")

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
upstream:
  base_url: "https://api.example.com/v1"
  api_key: "${TEST_API_KEY_123}"
  model: "model"
providers:
  test:
    name: "Test"
    base_url: "https://test.com/v1"
    api_key: "${TEST_API_KEY_123}"
    model: "test"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "expanded-key-value", cfg.Upstream.APIKey)
	assert.Equal(t, "expanded-key-value", cfg.Providers["test"].APIKey)
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	assert.Error(t, err)
}

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name     string
		modelMap map[string]string
		input    string
		expected string
	}{
		{"mapped model", map[string]string{"gpt-4o": "upstream-model"}, "gpt-4o", "upstream-model"},
		{"unmapped model passthrough", map[string]string{"gpt-4o": "upstream-model"}, "claude-3", "claude-3"},
		{"nil model map", nil, "gpt-4o", "gpt-4o"},
		{"empty model map", map[string]string{}, "gpt-4o", "gpt-4o"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &UpstreamConfig{ModelMap: tt.modelMap}
			result := u.ResolveModel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDataDir(t *testing.T) {
	dir, err := DataDir()
	require.NoError(t, err)
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".llm-gateway"), dir)
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
