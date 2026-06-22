package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	assert.Equal(t, filepath.Join(home, DataDirName), dir)
}

func TestEnsureDataDir(t *testing.T) {
	dir, err := EnsureDataDir()
	require.NoError(t, err)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestDefaultConfigPath(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	tests := []struct {
		name     string
		flagPath string
		expected string
	}{
		{
			name:     "custom flag path",
			flagPath: "/custom/path.yaml",
			expected: "/custom/path.yaml",
		},
		{
			name:     "empty flag defaults to data dir",
			flagPath: "",
			expected: filepath.Join(home, DataDirName, ConfigFile),
		},
		{
			name:     "flag path config.yaml",
			flagPath: "config.yaml",
			expected: "config.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := DefaultConfigPath(tt.flagPath)
			require.NoError(t, err)
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
			result := cfg.DefaultRouteConfig("")
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedProv, result.Provider)
			}
		})
	}

	t.Run("protocol-specific default", func(t *testing.T) {
		routes := map[string]RouteRule{
			"gw-anth":   {Provider: "anth-provider"},
			"gw-resp":   {Provider: "resp-provider"},
			"gw-chat":   {Provider: "chat-provider"},
			"gw-global": {Provider: "global-provider"},
		}

		cfg := &Config{
			DefaultRoute:          "gw-global",
			DefaultAnthropicRoute: "gw-anth",
			DefaultResponsesRoute: "gw-resp",
			DefaultChatRoute:      "gw-chat",
			Routes:                routes,
		}

		r := cfg.DefaultRouteConfig("anthropic")
		require.NotNil(t, r)
		assert.Equal(t, "anth-provider", r.Provider)

		r = cfg.DefaultRouteConfig("responses")
		require.NotNil(t, r)
		assert.Equal(t, "resp-provider", r.Provider)

		r = cfg.DefaultRouteConfig("chat")
		require.NotNil(t, r)
		assert.Equal(t, "chat-provider", r.Provider)

		r = cfg.DefaultRouteConfig("")
		require.NotNil(t, r)
		assert.Equal(t, "global-provider", r.Provider)

		cfg2 := &Config{
			DefaultRoute: "gw-global",
			Routes:       routes,
		}
		r = cfg2.DefaultRouteConfig("anthropic")
		require.NotNil(t, r)
		assert.Equal(t, "global-provider", r.Provider)
	})
}

func TestLoad_LogRetentionDays(t *testing.T) {
	t.Run("default value when not set", func(t *testing.T) {
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
		require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))
		cfg, err := Load(cfgPath)
		require.NoError(t, err)
		assert.Equal(t, DefaultLogRetentionDays, cfg.LogRetentionDays)
	})

	t.Run("custom value", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		content := `
server:
  host: "0.0.0.0"
  port: 12345
log_retention_days: 7
providers:
  test:
    name: "Test"
    base_url: "https://api.example.com/v1"
    api_key: "key"
`
		require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))
		cfg, err := Load(cfgPath)
		require.NoError(t, err)
		assert.Equal(t, 7, cfg.LogRetentionDays)
	})

	t.Run("invalid value falls back to default", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		content := `
server:
  host: "0.0.0.0"
  port: 12345
log_retention_days: -1
providers:
  test:
    name: "Test"
    base_url: "https://api.example.com/v1"
    api_key: "key"
`
		require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))
		cfg, err := Load(cfgPath)
		require.NoError(t, err)
		assert.Equal(t, DefaultLogRetentionDays, cfg.LogRetentionDays)
	})
}

func TestLoad_LLMLogEnabled(t *testing.T) {
	t.Run("defaults to true when not set", func(t *testing.T) {
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
		require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))
		cfg, err := Load(cfgPath)
		require.NoError(t, err)
		assert.True(t, cfg.LLMLogEnabled, "LLM logging should default to enabled")
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		content := `
server:
  host: "0.0.0.0"
  port: 12345
llm_log_enabled: false
providers:
  test:
    name: "Test"
    base_url: "https://api.example.com/v1"
    api_key: "key"
`
		require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))
		cfg, err := Load(cfgPath)
		require.NoError(t, err)
		assert.False(t, cfg.LLMLogEnabled)
	})

	t.Run("false survives a write/read round-trip", func(t *testing.T) {
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		require.NoError(t, WriteConfig(cfgPath, &Config{
			Server:        ServerConfig{Host: "127.0.0.1", Port: 12345},
			LLMLogEnabled: false,
		}))
		cfg, err := Load(cfgPath)
		require.NoError(t, err)
		assert.False(t, cfg.LLMLogEnabled, "disabled flag must persist across write/read")
	})
}

func TestLoad_RecoveryAppliesLLMLogEnabledDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Simulate a backup from before llm_log_enabled existed: valid YAML,
	// but the field is absent. Written directly because WriteConfig now
	// always persists the field.
	legacyBackup := `server:
  host: "127.0.0.1"
  port: 12345
providers:
  test:
    name: "Test"
    base_url: "https://api.example.com/v1"
    api_key: "key"
`
	require.NoError(t, os.WriteFile(backupName(path, time.Now()), []byte(legacyBackup), 0644))

	// Corrupt the main file so Load falls back to backup recovery.
	require.NoError(t, os.WriteFile(path, []byte("garbage: [[["), 0644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.True(t, cfg.LLMLogEnabled,
		"recovery from a pre-field backup must apply the default (true), not the bool zero value (false)")
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
		{"embedded env", "key-${MY_KEY}-suffix", "MY_KEY", "my-value", "key-my-value-suffix"},
		{"multiple env vars", "${A}-${B}", "A,B", "x,y", "x-y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := strings.Split(tt.envKey, ",")
			vals := strings.Split(tt.envValue, ",")
			for i, k := range keys {
				if k != "" && i < len(vals) {
					os.Setenv(k, vals[i])
					defer os.Unsetenv(k)
				}
			}
			result := expandEnv(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoad_RecoversFromCorruptMain_WithValidBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// 1. Seed a valid config (this is the "first" write; no backup yet).
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))

	// 2. Write a second time so the first one gets backed up.
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))
	infos, err := ListBackups(path)
	require.NoError(t, err)
	require.Len(t, infos, 1, "second write should create exactly one backup")

	// 3. Corrupt the main file. The backup is still intact.
	require.NoError(t, os.WriteFile(path, []byte("not: valid: yaml: [[["), 0644))

	// 4. Load must silently recover from the backup and return a valid cfg.
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 12345, cfg.Server.Port)
}

func TestLoad_FailsWhenMainAndAllBackupsCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Seed two writes so a backup exists.
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))
	infos, err := ListBackups(path)
	require.NoError(t, err)
	require.Len(t, infos, 1)

	// Corrupt both main and the only backup.
	require.NoError(t, os.WriteFile(path, []byte("garbage: [[["), 0644))
	require.NoError(t, os.WriteFile(infos[0].Path, []byte("also: garbage: [[["), 0644))

	_, err = Load(path)
	require.Error(t, err, "Load must fail when no usable config exists")
}

func TestLoad_FailsWhenMainCorrupt_NoBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Manually create a corrupt file with no backups.
	require.NoError(t, os.WriteFile(path, []byte("garbage: [[["), 0644))

	_, err := Load(path)
	require.Error(t, err)
}

func TestLoad_DoesNotModifyGoodMain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	writeFile := func(p, c string) { require.NoError(t, os.WriteFile(p, []byte(c), 0644)) }
	writeFile(path, "server:\n  host: 127.0.0.1\n  port: 12345\n")

	before, err := os.ReadFile(path)
	require.NoError(t, err)

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 12345, cfg.Server.Port)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "Load must not rewrite a good main file")
}

func TestLoad_RecoversFromInvalidYAMLTypeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Two writes so a backup exists.
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))

	// Type mismatch: port expects int, give it a string.
	require.NoError(t, os.WriteFile(path, []byte("server:\n  port: \"not-a-number\"\n"), 0644))

	cfg, err := Load(path)
	require.NoError(t, err, "must recover from type mismatch")
	assert.Equal(t, 12345, cfg.Server.Port)
}

func TestLoad_RecoveryNormalizesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Seed a config with env-var reference and a provider missing Format.
	t.Setenv("TEST_API_KEY", "secret-from-env")
	seeded := &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 12345},
		Providers: map[string]ProviderConfig{
			"p1": {Name: "P1", BaseURL: "https://x.test/", APIKey: "${TEST_API_KEY}"},
		},
		Routes: map[string]RouteRule{
			"r1": {Provider: "p1", DefaultModel: "m"},
		},
	}
	require.NoError(t, WriteConfig(path, seeded))
	// Second write to ensure a backup exists.
	require.NoError(t, WriteConfig(path, seeded))

	// Corrupt main.
	require.NoError(t, os.WriteFile(path, []byte("garbage: [[["), 0644))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "secret-from-env", cfg.Providers["p1"].APIKey,
		"recovered config must have env vars expanded")
	assert.Equal(t, "chat", cfg.Providers["p1"].Format,
		"recovered config must have Format defaulted to 'chat'")
	assert.Equal(t, "https://x.test", cfg.Providers["p1"].BaseURL,
		"recovered config must have BaseURL trimmed")
}

func TestLoad_RecoveryDoesNotPolluteBackupChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Seed two writes so we have one good backup.
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))
	require.NoError(t, WriteConfig(path, &Config{Server: ServerConfig{Host: "127.0.0.1", Port: 12345}}))
	infosBefore, err := ListBackups(path)
	require.NoError(t, err)
	require.Len(t, infosBefore, 1)

	// Corrupt main.
	require.NoError(t, os.WriteFile(path, []byte("garbage: [[["), 0644))

	// Recover.
	_, err = Load(path)
	require.NoError(t, err)

	// Main is now valid YAML.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "127.0.0.1")

	// The corrupt bytes must NOT have been captured as a new backup.
	infosAfter, err := ListBackups(path)
	require.NoError(t, err)
	assert.Len(t, infosAfter, 1, "recovery must not add a new backup")
	for _, bi := range infosAfter {
		b, err := os.ReadFile(bi.Path)
		require.NoError(t, err)
		assert.NotContains(t, string(b), "garbage", "no backup may contain the corrupt content")
	}
}
