package config

import (
	"fmt"
	"sync"
)

// Provider holds the current configuration with thread-safe access.
type Provider struct {
	mu   sync.RWMutex
	cfg  *Config
	path string
}

// NewProvider creates a new config provider with the initial config.
func NewProvider(cfg *Config, path string) *Provider {
	return &Provider{cfg: cfg, path: path}
}

// Path returns the config file path.
func (p *Provider) Path() string {
	return p.path
}

// Get returns a snapshot of the current config. The returned pointer
// must not be modified by callers.
func (p *Provider) Get() *Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

// Reload re-reads the config file and swaps the configuration.
// In-flight requests continue using the old config.
func (p *Provider) Reload() error {
	newCfg, err := Load(p.path)
	if err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.cfg = newCfg
	return nil
}
