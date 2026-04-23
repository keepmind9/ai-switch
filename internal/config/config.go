package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

const (
	DefaultHost             = "127.0.0.1"
	DefaultPort             = 12345
	DataDirName             = ".ai-switch"
	UsageDBName             = "usage.db"
	ConfigFile              = "config.yaml"
	PidFileName             = "ai-switch.pid"
	DefaultLogRetentionDays = 30
)

type Config struct {
	Server                ServerConfig              `mapstructure:"server" yaml:"server"`
	LogRetentionDays      int                       `mapstructure:"log_retention_days" yaml:"log_retention_days,omitempty"`
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
	SceneMap             map[string]string `mapstructure:"scene_map" yaml:"scene_map"`
	ModelMap             map[string]string `mapstructure:"model_map" yaml:"model_map"`
	LongContextThreshold int               `mapstructure:"long_context_threshold" yaml:"long_context_threshold,omitempty"`
}

type ServerConfig struct {
	Host string `mapstructure:"host" yaml:"host"`
	Port int    `mapstructure:"port" yaml:"port"`
}

type ProviderConfig struct {
	Name     string   `mapstructure:"name" yaml:"name"`
	BaseURL  string   `mapstructure:"base_url" yaml:"base_url"`
	Path     string   `mapstructure:"path" yaml:"path"`
	APIKey   string   `mapstructure:"api_key" yaml:"api_key"`
	Format   string   `mapstructure:"format" yaml:"format"`
	LogoURL  string   `mapstructure:"logo_url" yaml:"logo_url"`
	Sponsor  bool     `mapstructure:"sponsor" yaml:"sponsor"`
	ThinkTag string   `mapstructure:"think_tag" yaml:"think_tag,omitempty"`
	Models   []string `mapstructure:"models" yaml:"models,omitempty"`
}

var validFormats = map[string]bool{
	"chat":      true,
	"responses": true,
	"anthropic": true,
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

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
			if writeErr := WriteConfig(path, &Config{
				Server: ServerConfig{Host: DefaultHost, Port: DefaultPort},
			}); writeErr != nil {
				return nil, fmt.Errorf("failed to create default config: %w", writeErr)
			}
			if readErr := v.ReadInConfig(); readErr != nil {
				return nil, readErr
			}
		} else {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if cfg.LogRetentionDays <= 0 {
		cfg.LogRetentionDays = DefaultLogRetentionDays
	}

	// Validate default_route references an existing route
	if cfg.DefaultRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultRoute]; !ok {
			return nil, fmt.Errorf("default_route %q not found in routes", cfg.DefaultRoute)
		}
	}
	if cfg.DefaultAnthropicRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultAnthropicRoute]; !ok {
			return nil, fmt.Errorf("default_anthropic_route %q not found in routes", cfg.DefaultAnthropicRoute)
		}
	}
	if cfg.DefaultResponsesRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultResponsesRoute]; !ok {
			return nil, fmt.Errorf("default_responses_route %q not found in routes", cfg.DefaultResponsesRoute)
		}
	}
	if cfg.DefaultChatRoute != "" {
		if _, ok := cfg.Routes[cfg.DefaultChatRoute]; !ok {
			return nil, fmt.Errorf("default_chat_route %q not found in routes", cfg.DefaultChatRoute)
		}
	}

	// Set defaults and expand env vars for all providers
	for k, p := range cfg.Providers {
		p.APIKey = expandEnv(p.APIKey)
		if p.Format == "" {
			p.Format = "chat"
		}
		if !validFormats[p.Format] {
			return nil, fmt.Errorf("invalid format %q for provider %q: must be one of chat, responses, anthropic", p.Format, k)
		}
		// Trim trailing slash to keep URL clean.
		p.BaseURL = strings.TrimRight(p.BaseURL, "/")
		cfg.Providers[k] = p
	}

	return &cfg, nil
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
