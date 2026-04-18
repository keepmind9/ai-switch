package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig              `mapstructure:"server"`
	Upstream  UpstreamConfig            `mapstructure:"upstream"`
	Providers map[string]ProviderConfig `mapstructure:"providers"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

type UpstreamConfig struct {
	BaseURL  string            `mapstructure:"base_url"`
	Path     string            `mapstructure:"path"`
	APIKey   string            `mapstructure:"api_key"`
	Model    string            `mapstructure:"model"`
	Format   string            `mapstructure:"format"`
	ModelMap map[string]string `mapstructure:"model_map"`
}

type ProviderConfig struct {
	Name     string            `mapstructure:"name"`
	BaseURL  string            `mapstructure:"base_url"`
	APIKey   string            `mapstructure:"api_key"`
	Model    string            `mapstructure:"model"`
	Format   string            `mapstructure:"format"`
	ModelMap map[string]string `mapstructure:"model_map"`
	LogoURL  string            `mapstructure:"logo_url"`
	Sponsor  bool              `mapstructure:"sponsor"`
}

var validFormats = map[string]bool{
	"chat":      true,
	"responses": true,
	"anthropic": true,
}

func Load(path string) (*Config, error) {
	viper.SetConfigFile(path)
	viper.SetConfigType("yaml")

	if err := viper.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Set default format
	if cfg.Upstream.Format == "" {
		cfg.Upstream.Format = "chat"
	}

	// Validate format
	if !validFormats[cfg.Upstream.Format] {
		return nil, fmt.Errorf("invalid upstream format %q: must be one of chat, responses, anthropic", cfg.Upstream.Format)
	}

	// Expand environment variables
	cfg.Upstream.APIKey = expandEnv(cfg.Upstream.APIKey)
	for k, p := range cfg.Providers {
		p.APIKey = expandEnv(p.APIKey)
		if p.Format == "" {
			p.Format = "chat"
		}
		cfg.Providers[k] = p
	}

	return &cfg, nil
}

// ResolveModel maps a client model name to the upstream model name via model_map.
// If no mapping exists, returns the original model name.
func (u *UpstreamConfig) ResolveModel(model string) string {
	if u.ModelMap != nil {
		if mapped, ok := u.ModelMap[model]; ok {
			return mapped
		}
	}
	return model
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
