package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	DefaultHost             = "127.0.0.1"
	DefaultPort             = 12345
	DataDirName             = ".ai-switch"
	UsageDBName             = "usage.db"
	ConfigFile              = "config.yaml"
	PidFileName             = "ai-switch.pid"
	DefaultLogRetentionDays = 30
	DefaultLLMLogEnabled    = true
)

type Config struct {
	Server                ServerConfig              `mapstructure:"server" yaml:"server"`
	LogRetentionDays      int                       `mapstructure:"log_retention_days" yaml:"log_retention_days,omitempty"`
	LLMLogEnabled         bool                      `mapstructure:"llm_log_enabled" yaml:"llm_log_enabled"`
	DefaultRoute          string                    `mapstructure:"default_route" yaml:"default_route,omitempty"`
	DefaultAnthropicRoute string                    `mapstructure:"default_anthropic_route" yaml:"default_anthropic_route,omitempty"`
	DefaultResponsesRoute string                    `mapstructure:"default_responses_route" yaml:"default_responses_route,omitempty"`
	DefaultChatRoute      string                    `mapstructure:"default_chat_route" yaml:"default_chat_route,omitempty"`
	Providers             map[string]ProviderConfig `mapstructure:"providers" yaml:"providers"`
	Routes                map[string]RouteRule      `mapstructure:"routes" yaml:"routes"`
}

type RouteRule struct {
	Provider             string            `mapstructure:"provider" yaml:"provider"`
	DefaultModel         string            `mapstructure:"default_model" yaml:"default_model"`
	Disabled             bool              `mapstructure:"disabled" yaml:"disabled,omitempty"`
	SceneMap             map[string]string `mapstructure:"scene_map" yaml:"scene_map"`
	ModelMap             map[string]string `mapstructure:"model_map" yaml:"model_map"`
	LongContextThreshold int               `mapstructure:"long_context_threshold" yaml:"long_context_threshold,omitempty"`
}

type ServerConfig struct {
	Host       string   `mapstructure:"host" yaml:"host"`
	Port       int      `mapstructure:"port" yaml:"port"`
	AllowedIPs []string `mapstructure:"allowed_ips" yaml:"allowed_ips,omitempty"`
	ProxyURL   string   `mapstructure:"proxy_url" yaml:"proxy_url,omitempty"`
}

type ProviderConfig struct {
	Name          string            `mapstructure:"name" yaml:"name"`
	BaseURL       string            `mapstructure:"base_url" yaml:"base_url"`
	Path          string            `mapstructure:"path" yaml:"path"`
	APIKey        string            `mapstructure:"api_key" yaml:"api_key"`
	FallbackKeys  []string          `mapstructure:"fallback_keys" yaml:"fallback_keys,omitempty"`
	Format        string            `mapstructure:"format" yaml:"format"`
	LogoURL       string            `mapstructure:"logo_url" yaml:"logo_url"`
	ThinkTag      string            `mapstructure:"think_tag" yaml:"think_tag,omitempty"`
	Models        []string          `mapstructure:"models" yaml:"models,omitempty"`
	EnableProxy   bool              `mapstructure:"enable_proxy" yaml:"enable_proxy,omitempty"`
	CustomHeaders map[string]string `mapstructure:"custom_headers" yaml:"custom_headers,omitempty"`
}

var validFormats = map[string]bool{
	"chat":      true,
	"responses": true,
	"anthropic": true,
	"gemini":    true,
}

func ValidFormat(f string) bool {
	return validFormats[f]
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("server.host", DefaultHost)
	v.SetDefault("server.port", DefaultPort)
	v.SetDefault("log_retention_days", DefaultLogRetentionDays)
	v.SetDefault("llm_log_enabled", DefaultLLMLogEnabled)

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
			if writeErr := WriteConfig(path, &Config{
				Server:        ServerConfig{Host: DefaultHost, Port: DefaultPort},
				LLMLogEnabled: DefaultLLMLogEnabled,
			}); writeErr != nil {
				return nil, fmt.Errorf("failed to create default config: %w", writeErr)
			}
			if readErr := v.ReadInConfig(); readErr != nil {
				return nil, readErr
			}
		} else {
			// Main file exists but is unreadable or unparseable. Try to
			// recover from a timestamped backup before giving up.
			cfg, recErr := recoverFromBackup(path)
			if recErr != nil {
				return nil, fmt.Errorf("failed to read config (recovery from backup also failed): %w", err)
			}
			return cfg, nil
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		// YAML syntax error or type mismatch in main file. Try recovery.
		if recCfg, recErr := recoverFromBackup(path); recErr == nil {
			return recCfg, nil
		}
		return nil, err
	}

	// viper lowercases map keys during Unmarshal, but HTTP header names are
	// case-sensitive in config/UI. Re-read custom_headers from the raw YAML to
	// restore the original casing (e.g. "User-Agent", not "user-agent").
	preserveCustomHeaderCase(path, &cfg)

	if cfg.LogRetentionDays <= 0 {
		cfg.LogRetentionDays = DefaultLogRetentionDays
	}

	if err := normalizeConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// normalizeConfig applies env-var expansion, default format, route reference
// validation, and URL trimming. Called from the main Load path AND from
// recoverFromBackup, so a recovered config behaves identically to one loaded
// from a fresh file.
func normalizeConfig(cfg *Config) error {
	// Validate default_route references an existing route
	if cfg.DefaultRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultRoute]; !ok {
			return fmt.Errorf("default_route %q not found in routes", cfg.DefaultRoute)
		}
	}
	if cfg.DefaultAnthropicRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultAnthropicRoute]; !ok {
			return fmt.Errorf("default_anthropic_route %q not found in routes", cfg.DefaultAnthropicRoute)
		}
	}
	if cfg.DefaultResponsesRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultResponsesRoute]; !ok {
			return fmt.Errorf("default_responses_route %q not found in routes", cfg.DefaultResponsesRoute)
		}
	}
	if cfg.DefaultChatRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultChatRoute]; !ok {
			return fmt.Errorf("default_chat_route %q not found in routes", cfg.DefaultChatRoute)
		}
	}

	// Set defaults and expand env vars for all providers
	for k, p := range cfg.Providers {
		p.APIKey = expandEnv(p.APIKey)
		for i, fk := range p.FallbackKeys {
			p.FallbackKeys[i] = expandEnv(fk)
		}
		if p.Format == "" {
			p.Format = "chat"
		}
		if !validFormats[p.Format] {
			return fmt.Errorf("invalid format %q for provider %q: must be one of chat, responses, anthropic, gemini", p.Format, k)
		}
		// Trim trailing slash to keep URL clean.
		p.BaseURL = strings.TrimRight(p.BaseURL, "/")
		cfg.Providers[k] = p
	}

	return nil
}

// DefaultRouteConfig returns the default route rule for the given protocol,
// falling back to the global default_route. Returns nil if none configured.
func (c *Config) DefaultRouteConfig(protocol string) *RouteRule {
	var key string
	switch protocol {
	case "anthropic":
		key = c.DefaultAnthropicRoute
	case "responses":
		key = c.DefaultResponsesRoute
	case "chat":
		key = c.DefaultChatRoute
	}
	if key == "" {
		key = c.DefaultRoute
	}
	if key == "" {
		return nil
	}
	r, ok := c.Routes[key]
	if !ok {
		return nil
	}
	return &r
}

// DataDir returns the path to the data directory (~/.ai-switch/).
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, DataDirName), nil
}

// EnsureDataDir creates the data directory if it does not exist.
func EnsureDataDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create data directory %s: %w", dir, err)
	}
	return dir, nil
}

// DefaultConfigPath returns the config file path following the priority:
// 1. provided path (from -c flag)
// 2. ~/.ai-switch/config.yaml
func DefaultConfigPath(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	dir, err := DataDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine config path: %w", err)
	}
	return filepath.Join(dir, ConfigFile), nil
}

func expandEnv(s string) string {
	return os.Expand(s, func(key string) string {
		return os.Getenv(key)
	})
}

// IsLocalhost returns true if the host is 127.0.0.1 or localhost.
func (s ServerConfig) IsLocalhost() bool {
	return s.Host == "127.0.0.1" || s.Host == "localhost"
}

// recoverFromBackup attempts to load a valid Config from a timestamped
// backup of path. The newest valid backup wins; on success the recovered
// config is normalized (env expansion, defaults, route validation) and
// written back to path (so subsequent starts don't need recovery). A WARN
// is logged identifying the recovery. Returns an error if no usable backup
// exists.
func recoverFromBackup(path string) (*Config, error) {
	infos, err := ListBackups(path)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("no backups available")
	}
	for _, bi := range infos {
		cfg, loadErr := loadFromFile(bi.Path)
		if loadErr != nil {
			slog.Warn("config: backup also unreadable, trying next",
				"backup", bi.Name, "error", loadErr)
			continue
		}
		// Apply defaults for fields that yaml.Unmarshal cannot distinguish
		// between "absent" and "zero value". A backup predating
		// llm_log_enabled must not silently disable trace logging. Done
		// before normalizeConfig so recovery mirrors the main Load path.
		applyLLMLogDefaultIfMissing(bi.Path, cfg)
		// Normalize the recovered config: env expansion, format defaults,
		// route validation. Mirrors the main Load path.
		if normErr := normalizeConfig(cfg); normErr != nil {
			slog.Warn("config: recovered backup failed normalization, trying next",
				"backup", bi.Name, "error", normErr)
			continue
		}
		slog.Warn("config: main file corrupt, restored from backup",
			"main", path, "backup", bi.Name)
		// Persist the recovered config back to the main path. Use a direct
		// temp+rename (NOT WriteConfig) so the corrupt main file is NOT
		// captured as a backup first. If the main file is gone or
		// unreadable, we treat its current state as garbage to overwrite.
		if writeErr := writeConfigDirect(path, cfg); writeErr != nil {
			slog.Warn("config: failed to persist recovered config to main path",
				"path", path, "error", writeErr)
		}
		return cfg, nil
	}
	return nil, fmt.Errorf("all %d backups are unreadable", len(infos))
}

// loadFromFile reads and parses a YAML config file via yaml.Unmarshal
// directly (no viper, no mapstructure). Used only for backup recovery
// where we want the raw parsed result without viper's strict type coercion
// — a corrupted main file may use string-for-int tricks that viper rejects
// but yaml.Unmarshal accepts.
func loadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// applyLLMLogDefaultIfMissing sets LLMLogEnabled to its default when the file
// does not contain the key. loadFromFile uses yaml.Unmarshal directly (no
// viper defaults), and a bool's zero value ("false") is indistinguishable
// from an absent key — so without this, recovering a backup that predates
// llm_log_enabled would silently disable trace logging, contradicting the
// documented default of true. The raw re-read is best-effort: on any
// read/parse error it leaves the loaded value untouched.
func applyLLMLogDefaultIfMissing(path string, cfg *Config) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return
	}
	if _, ok := raw["llm_log_enabled"]; !ok {
		cfg.LLMLogEnabled = DefaultLLMLogEnabled
	}
}

// preserveCustomHeaderCase re-reads providers' custom_headers from the raw
// YAML file with case-sensitive keys. viper lowercases map keys during
// Unmarshal (turning "User-Agent" into "user-agent"), but HTTP header names
// matter in config/UI, so the original casing is restored here. Best-effort:
// on any read error it leaves the (lowercased) viper result in place, which
// still works functionally — Go canonicalizes header names when sending.
//
// Scoped to custom_headers only: other map fields (RouteRule.SceneMap /
// ModelMap) are case-insensitive route selectors, so their lowercasing is
// harmless and intentionally not reversed here.
func preserveCustomHeaderCase(path string, cfg *Config) {
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("config: failed to re-read custom_headers for case preservation", "error", err)
		return
	}
	var raw struct {
		Providers map[string]struct {
			CustomHeaders map[string]string `yaml:"custom_headers"`
		} `yaml:"providers"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		slog.Warn("config: failed to parse custom_headers for case preservation", "error", err)
		return
	}
	for k, p := range raw.Providers {
		if p.CustomHeaders == nil {
			continue
		}
		if cp, ok := cfg.Providers[k]; ok {
			cp.CustomHeaders = p.CustomHeaders
			cfg.Providers[k] = cp
		}
	}
}

// writeConfigDirect writes cfg to path atomically (temp file + rename) but
// does NOT create a backup and does NOT prune. Used by recoverFromBackup so
// that a corrupt main file is never copied into the backup chain.
func writeConfigDirect(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-recover-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if _, statErr := os.Stat(tmpName); statErr == nil {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
