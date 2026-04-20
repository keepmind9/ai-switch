package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProvider(t *testing.T) (*Provider, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	content := `
server:
  host: "0.0.0.0"
  port: 12345
default_route: "gw-default"
providers:
  default:
    name: "Default"
    base_url: "https://api.example.com/v1"
    api_key: "initial-key"
routes:
  "gw-default":
    provider: "default"
    default_model: "initial-model"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	return NewProvider(cfg, cfgPath), cfgPath
}

func TestProvider_Get(t *testing.T) {
	p, _ := newTestProvider(t)

	cfg := p.Get()
	require.NotNil(t, cfg)
	assert.Equal(t, "gw-default", cfg.DefaultRoute)

	dr := cfg.DefaultRouteConfig()
	require.NotNil(t, dr)
	assert.Equal(t, "default", dr.Provider)
	assert.Equal(t, "initial-model", dr.DefaultModel)
}

func TestProvider_Reload(t *testing.T) {
	p, cfgPath := newTestProvider(t)

	// Update config file
	newContent := `
server:
  host: "0.0.0.0"
  port: 9999
default_route: "gw-default"
providers:
  default:
    name: "Default"
    base_url: "https://api.new.com/v1"
    api_key: "new-key"
routes:
  "gw-default":
    provider: "default"
    default_model: "new-model"
`
	err := os.WriteFile(cfgPath, []byte(newContent), 0644)
	require.NoError(t, err)

	err = p.Reload()
	require.NoError(t, err)

	cfg := p.Get()
	assert.Equal(t, 9999, cfg.Server.Port)

	dr := cfg.DefaultRouteConfig()
	require.NotNil(t, dr)
	assert.Equal(t, "default", dr.Provider)
	assert.Equal(t, "new-model", dr.DefaultModel)
}

func TestProvider_Reload_InvalidFile(t *testing.T) {
	p, cfgPath := newTestProvider(t)

	// Corrupt config file
	err := os.WriteFile(cfgPath, []byte("{{invalid yaml"), 0644)
	require.NoError(t, err)

	err = p.Reload()
	assert.Error(t, err)

	// Old config should still be accessible
	cfg := p.Get()
	dr := cfg.DefaultRouteConfig()
	require.NotNil(t, dr)
	assert.Equal(t, "default", dr.Provider)
}

func TestProvider_Reload_MissingFile(t *testing.T) {
	p, cfgPath := newTestProvider(t)

	os.Remove(cfgPath)

	err := p.Reload()
	assert.Error(t, err)

	cfg := p.Get()
	dr := cfg.DefaultRouteConfig()
	require.NotNil(t, dr)
	assert.Equal(t, "default", dr.Provider)
}

func TestProvider_ConcurrentAccess(t *testing.T) {
	p, _ := newTestProvider(t)

	var wg sync.WaitGroup
	const goroutines = 50

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := p.Get()
			assert.NotNil(t, cfg)
		}()
	}

	// Concurrent reloads (with valid config)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Reload()
		}()
	}

	wg.Wait()

	// Verify provider is still functional
	cfg := p.Get()
	assert.NotNil(t, cfg)
}

func TestProvider_Reload_UpdatesProviders(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")

	initialContent := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  default:
    name: "Default"
    base_url: "https://api.example.com/v1"
    api_key: "key"
  test:
    name: "Test"
    base_url: "https://test.com/v1"
    api_key: "key1"
`
	err := os.WriteFile(cfgPath, []byte(initialContent), 0644)
	require.NoError(t, err)

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	p := NewProvider(cfg, cfgPath)

	// Verify initial state
	assert.Equal(t, "key1", p.Get().Providers["test"].APIKey)

	// Update config with new provider key
	updatedContent := `
server:
  host: "0.0.0.0"
  port: 12345
providers:
  default:
    name: "Default"
    base_url: "https://api.example.com/v1"
    api_key: "key"
  test:
    name: "Test"
    base_url: "https://test.com/v1"
    api_key: "key2"
`
	err = os.WriteFile(cfgPath, []byte(updatedContent), 0644)
	require.NoError(t, err)

	err = p.Reload()
	require.NoError(t, err)

	assert.Equal(t, "key2", p.Get().Providers["test"].APIKey)
}
