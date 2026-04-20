package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server          ServerConfig              `mapstructure:"server" yaml:"server"`
	DefaultProvider string                    `mapstructure:"default_provider" yaml:"default_provider"`
	Providers       map[string]ProviderConfig `mapstructure:"providers" yaml:"providers"`
	Routes          map[string]RouteRule      `mapstructure:"routes" yaml:"routes"`
}

type RouteRule struct {
	Provider     string            `mapstructure:"provider" yaml:"provider"`
	DefaultModel string            `mapstructure:"default_model" yaml:"default_model"`
	SceneMap     map[string]string `mapstructure:"scene_map" yaml:"scene_map"`
	ModelMap     map[string]string `mapstructure:"model_map" yaml:"model_map"`
}

type ServerConfig struct {
	Host string `mapstructure:"host" yaml:"host"`
	Port int    `mapstructure:"port" yaml:"port"`
}

type ProviderConfig struct {
	Name     string `mapstructure:"name" yaml:"name"`
	BaseURL  string `mapstructure:"base_url" yaml:"base_url"`
	Path     string `mapstructure:"path" yaml:"path"`
	APIKey   string `mapstructure:"api_key" yaml:"api_key"`
	Model    string `mapstructure:"model" yaml:"model"`
	Format   string `mapstructure:"format" yaml:"format"`
	LogoURL  string `mapstructure:"logo_url" yaml:"logo_url"`
	Sponsor  bool   `mapstructure:"sponsor" yaml:"sponsor"`
	ThinkTag string `mapstructure:"think_tag" yaml:"think_tag,omitempty"`
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
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 12345)

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Validate default_provider references an existing provider
	if cfg.DefaultProvider != "" {
		if _, ok := cfg.Providers[cfg.DefaultProvider]; !ok {
			return nil, fmt.Errorf("default_provider %q not found in providers", cfg.DefaultProvider)
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
		// Strip common API path prefix to prevent double /v1/v1 in upstream URL.
		// Users often copy base_url from provider docs which include /v1.
		p.BaseURL = strings.TrimRight(strings.TrimSuffix(p.BaseURL, "/v1"), "/")
		cfg.Providers[k] = p
	}

	return &cfg, nil
}

// DefaultProviderConfig returns the default provider config, or nil if not set.
func (c *Config) DefaultProviderConfig() *ProviderConfig {
	if c.DefaultProvider == "" {
		return nil
	}
	p, ok := c.Providers[c.DefaultProvider]
	if !ok {
		return nil
	}
	return &p
}

// DataDir returns the path to the data directory (~/.llm-gateway/).
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".llm-gateway"), nil
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
// 2. ./config.yaml in current directory
// 3. ~/.llm-gateway/config.yaml
func DefaultConfigPath(flagPath string) string {
	if flagPath != "" && flagPath != "config.yaml" {
		return flagPath
	}
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	dir, err := DataDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(dir, "config.yaml")
}

func expandEnv(s string) string {
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		envKey := s[2 : len(s)-1]
		return os.Getenv(envKey)
	}
	return s
}
