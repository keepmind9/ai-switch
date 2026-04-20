package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := &Config{
		Server:       ServerConfig{Host: "0.0.0.0", Port: 12345},
		DefaultRoute: "gw-test",
		Providers: map[string]ProviderConfig{
			"minimax": {
				Name:    "MiniMax",
				BaseURL: "https://api.minimaxi.com",
				APIKey:  "sk-test-key",
				Format:  "chat",
				Sponsor: true,
			},
		},
		Routes: map[string]RouteRule{
			"gw-test": {
				Provider:     "minimax",
				DefaultModel: "MiniMax-M2.7",
				SceneMap:     map[string]string{"default": "MiniMax-M2.7"},
				ModelMap:     map[string]string{"gpt-4o": "MiniMax-M2.7"},
			},
		},
	}

	err := WriteConfig(path, original)
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "minimax")
	assert.Contains(t, string(data), "gw-test")

	loaded, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, original.Server.Host, loaded.Server.Host)
	assert.Equal(t, original.Server.Port, loaded.Server.Port)
	assert.Equal(t, original.DefaultRoute, loaded.DefaultRoute)
	assert.Len(t, loaded.Providers, 1)
	assert.Equal(t, "MiniMax", loaded.Providers["minimax"].Name)
	assert.Len(t, loaded.Routes, 1)
	assert.Equal(t, "minimax", loaded.Routes["gw-test"].Provider)
}
